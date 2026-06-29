// Package middleware provides HTTP middleware for auth, rate limiting,
// and download concurrency control.
package middleware

import (
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"nakamaserver/internal/logger"
)

// AuthAdmin rejects requests whose X-API-Key header doesn't match adminKey.
func AuthAdmin(adminKey string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != adminKey {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// AuthDownload rejects requests whose X-API-Key header doesn't match downloadKey.
func AuthDownload(downloadKey string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != downloadKey {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// AuthEither accepts requests whose X-API-Key matches either adminKey or downloadKey.
func AuthEither(adminKey, downloadKey string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-API-Key")
		if key != adminKey && key != downloadKey {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- Per-IP rate limiter (token bucket) ---

type bucket struct {
	tokens   float64
	lastFill time.Time
	mu       sync.Mutex
}

type ipRateLimiter struct {
	mu          sync.Mutex
	buckets     map[string]*bucket
	ratePerMin  float64
	burstSize   float64
	retryAfter  string
}

func newIPRateLimiter(ratePerMin, burstSize float64, retryAfter string) *ipRateLimiter {
	rl := &ipRateLimiter{
		buckets:     make(map[string]*bucket),
		ratePerMin:  ratePerMin,
		burstSize:   burstSize,
		retryAfter:  retryAfter,
	}
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		for range ticker.C {
			rl.mu.Lock()
			now := time.Now()
			for ip, b := range rl.buckets {
				b.mu.Lock()
				idle := now.Sub(b.lastFill)
				b.mu.Unlock()
				if idle > 10*time.Minute {
					delete(rl.buckets, ip)
				}
			}
			rl.mu.Unlock()
		}
	}()
	return rl
}

func (rl *ipRateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	b, ok := rl.buckets[ip]
	if !ok {
		b = &bucket{tokens: rl.burstSize, lastFill: time.Now()}
		rl.buckets[ip] = b
	}
	rl.mu.Unlock()

	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.lastFill)
	b.tokens += elapsed.Seconds() * (rl.ratePerMin / 60.0)
	if b.tokens > rl.burstSize {
		b.tokens = rl.burstSize
	}
	b.lastFill = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// downloadLimiter: 10 req/min per IP, burst 10.
var downloadLimiter = newIPRateLimiter(10, 10, "6")

// adminLimiter: 120 req/min per IP, burst 60.
var adminLimiter = newIPRateLimiter(120, 60, "1")

// RateLimit applies the download (user) rate limit: 10 req/min per IP.
func RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		if !downloadLimiter.allow(ip) {
			logger.Warn("rate limit hit", map[string]any{"ip": ip, "path": r.URL.Path})
			w.Header().Set("Retry-After", "6")
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RateLimitAdmin applies the admin rate limit: 120 req/min per IP, burst 60.
func RateLimitAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		if !adminLimiter.allow(ip) {
			logger.Warn("admin rate limit hit", map[string]any{"ip": ip, "path": r.URL.Path})
			w.Header().Set("Retry-After", "1")
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- 1 active download per IP ---

type downloadTracker struct {
	mu      sync.Mutex
	active  map[string]*atomic.Int32
}

var dlTracker = &downloadTracker{active: make(map[string]*atomic.Int32)}

func (dt *downloadTracker) tryAcquire(ip string) bool {
	dt.mu.Lock()
	ctr, ok := dt.active[ip]
	if !ok {
		ctr = &atomic.Int32{}
		dt.active[ip] = ctr
	}
	dt.mu.Unlock()
	return ctr.CompareAndSwap(0, 1)
}

func (dt *downloadTracker) release(ip string) {
	dt.mu.Lock()
	ctr, ok := dt.active[ip]
	dt.mu.Unlock()
	if ok {
		ctr.Store(0)
	}
}

// OneDownloadPerIP allows only one concurrent download per source IP.
func OneDownloadPerIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		if !dlTracker.tryAcquire(ip) {
			logger.Warn("download slot occupied", map[string]any{"ip": ip})
			http.Error(w, `{"error":"another download is already active from your IP"}`, http.StatusTooManyRequests)
			return
		}
		defer dlTracker.release(ip)
		next.ServeHTTP(w, r)
	})
}

// Logger logs each incoming request with method, path, remote addr, and duration.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(lrw, r)
		logger.Info("request", map[string]any{
			"method":   r.Method,
			"path":     r.URL.Path,
			"remote":   r.RemoteAddr,
			"status":   lrw.statusCode,
			"duration": time.Since(start).String(),
		})
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

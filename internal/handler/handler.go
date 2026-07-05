// Package handler wires together the HTTP routes for the Nakama server.
// Routes:
//
//	GET  /query                                 - list all games + modpacks (download key)
//	POST /admin/upload/game                     - upload game zip (admin key)
//	POST /admin/upload/modpack                  - upload modpack zip (admin key)
//	GET  /download/game/{uuid}                  - stream game zip (download key, 1/IP)
//	GET  /download/modpack/{uuid}               - stream modpack zip (download key, 1/IP)
//	DELETE /admin/game/{uuid}                   - delete game (admin key)
//	DELETE /admin/modpack/{uuid}                - delete modpack (admin key)
//	PATCH /admin/game/{uuid}                    - update game fields (admin key)
//	PATCH /admin/modpack/{uuid}                 - update modpack fields (admin key)
//	GET  /admin/disk-quota                      - disk quota status (admin key)
package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"nakamaserver/internal/logger"
	"nakamaserver/internal/store"
)

// safeSegment sanitizes a path segment: lower-case, alphanumeric + dot + dash + underscore only.
var unsafeRe = regexp.MustCompile(`[^a-zA-Z0-9.\-_]`)

func sanitize(s string) string {
	return unsafeRe.ReplaceAllString(strings.TrimSpace(s), "_")
}

// clientIP extracts the real client IP from proxy headers or RemoteAddr.
func clientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func jsonErr(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	fmt.Fprintf(w, `{"error":%q}`, msg)
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		logger.Error("json encode", map[string]any{"err": err.Error()})
	}
}

// --- Query ---

// QueryHandler returns a handler for GET /query.
func QueryHandler(gdb *store.GamesDB, mdb *store.ModpacksDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if r.Method != http.MethodGet {
			jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		games, err := gdb.ListGames()
		if err != nil {
			logger.Error("list games", map[string]any{"err": err.Error(), "ip": ip})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}
		modpacks, err := mdb.ListModpacks()
		if err != nil {
			logger.Error("list modpacks", map[string]any{"err": err.Error(), "ip": ip})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}
		logger.Info("catalog queried", map[string]any{"ip": ip, "games": len(games), "modpacks": len(modpacks)})
		jsonOK(w, store.FullCatalog{Games: games, Modpacks: modpacks})
	}
}

// --- Upload ---

// UploadGameHandler returns a handler for POST /admin/upload/game.
// Expects multipart/form-data with fields: title, version, launch_exe, app_id (optional), notes (optional), title_notes (optional), file (the zip).
func UploadGameHandler(gdb *store.GamesDB, gamesDir string, maxBytes int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		if err := r.ParseMultipartForm(32 << 20); err != nil { // Max 32MB in memory, rest to disk
			jsonErr(w, "failed to parse form: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer func() {
			_ = r.MultipartForm.RemoveAll()
		}()
		title := strings.TrimSpace(r.FormValue("title"))
		version := strings.TrimSpace(r.FormValue("version"))
		launchExe := strings.TrimSpace(r.FormValue("launch_exe"))
		appID := strings.TrimSpace(r.FormValue("app_id"))
		notes := strings.TrimSpace(r.FormValue("notes"))
		titleNotes := strings.TrimSpace(r.FormValue("title_notes"))
		if title == "" || version == "" || launchExe == "" {
			jsonErr(w, "title, version, and launch_exe are required", http.StatusBadRequest)
			return
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			jsonErr(w, "missing file field", http.StatusBadRequest)
			return
		}
		defer file.Close()

		fileName := sanitize(title) + "_" + sanitize(version) + ".zip"
		destPath := filepath.Join(gamesDir, fileName)

		// Check for conflict in DB first.
		existing, err := gdb.GetGame(title, version)
		if err != nil {
			logger.Error("get game", map[string]any{"err": err.Error()})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}
		if existing != nil {
			jsonErr(w, "game already exists with this title+version", http.StatusConflict)
			return
		}

		// Write file to disk.
		if err := os.MkdirAll(gamesDir, 0o755); err != nil {
			logger.Error("mkdir games", map[string]any{"err": err.Error()})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}
		out, err := os.Create(destPath)
		if err != nil {
			logger.Error("create game file", map[string]any{"err": err.Error()})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}
		written, err := io.Copy(out, file)
		out.Close()
		if err != nil {
			os.Remove(destPath)
			logger.Error("write game file", map[string]any{"err": err.Error()})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}

		uuid, inserted, err := gdb.InsertGame(title, version, fileName, launchExe, appID, notes, titleNotes, written)
		if err != nil {
			os.Remove(destPath)
			logger.Error("insert game", map[string]any{"err": err.Error()})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !inserted {
			os.Remove(destPath)
			jsonErr(w, "game already exists with this title+version", http.StatusConflict)
			return
		}

		logger.Info("game uploaded", map[string]any{"title": title, "version": version, "uuid": uuid, "bytes": written})
		jsonOK(w, map[string]any{"ok": true, "uuid": uuid, "file": fileName, "size_bytes": written})
	}
}

// UploadModpackHandler returns a handler for POST /admin/upload/modpack.
// Expects multipart/form-data with fields: game_title, modpack_title, notes (optional), file (the zip).
func UploadModpackHandler(mdb *store.ModpacksDB, modpacksDir string, maxBytes int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		if err := r.ParseMultipartForm(32 << 20); err != nil { // Max 32MB in memory, rest to disk
			jsonErr(w, "failed to parse form: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer func() {
			_ = r.MultipartForm.RemoveAll()
		}()
		gameTitle := strings.TrimSpace(r.FormValue("game_title"))
		modpackTitle := strings.TrimSpace(r.FormValue("modpack_title"))
		notes := strings.TrimSpace(r.FormValue("notes"))
		if gameTitle == "" || modpackTitle == "" {
			jsonErr(w, "game_title and modpack_title are required", http.StatusBadRequest)
			return
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			jsonErr(w, "missing file field", http.StatusBadRequest)
			return
		}
		defer file.Close()

		fileName := sanitize(gameTitle) + "_" + sanitize(modpackTitle) + ".zip"
		destPath := filepath.Join(modpacksDir, fileName)

		existing, err := mdb.GetModpack(gameTitle, modpackTitle)
		if err != nil {
			logger.Error("get modpack", map[string]any{"err": err.Error()})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}
		if existing != nil {
			jsonErr(w, "modpack already exists with this game_title+modpack_title", http.StatusConflict)
			return
		}

		if err := os.MkdirAll(modpacksDir, 0o755); err != nil {
			logger.Error("mkdir modpacks", map[string]any{"err": err.Error()})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}
		out, err := os.Create(destPath)
		if err != nil {
			logger.Error("create modpack file", map[string]any{"err": err.Error()})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}
		written, err := io.Copy(out, file)
		out.Close()
		if err != nil {
			os.Remove(destPath)
			logger.Error("write modpack file", map[string]any{"err": err.Error()})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}

		uuid, inserted, err := mdb.InsertModpack(gameTitle, modpackTitle, fileName, notes, written)
		if err != nil {
			os.Remove(destPath)
			logger.Error("insert modpack", map[string]any{"err": err.Error()})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !inserted {
			os.Remove(destPath)
			jsonErr(w, "modpack already exists with this game_title+modpack_title", http.StatusConflict)
			return
		}

		logger.Info("modpack uploaded", map[string]any{"game": gameTitle, "modpack": modpackTitle, "uuid": uuid, "bytes": written})
		jsonOK(w, map[string]any{"ok": true, "uuid": uuid, "file": fileName, "size_bytes": written})
	}
}

// --- Download ---

// DownloadGameHandler returns a handler for GET /download/game/{uuid}.
func DownloadGameHandler(gdb *store.GamesDB, gamesDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if r.Method != http.MethodGet {
			jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		uuid := strings.TrimPrefix(r.URL.Path, "/download/game/")
		if uuid == "" || strings.Contains(uuid, "/") {
			jsonErr(w, "usage: /download/game/{uuid}", http.StatusBadRequest)
			return
		}

		game, err := gdb.GetGameByUUID(uuid)
		if err != nil {
			logger.Error("get game for download", map[string]any{"err": err.Error(), "ip": ip, "uuid": uuid})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}
		if game == nil {
			logger.Warn("game not found", map[string]any{"ip": ip, "uuid": uuid})
			jsonErr(w, "game not found", http.StatusNotFound)
			return
		}

		path := filepath.Join(gamesDir, game.FileName)
		f, err := os.Open(path)
		if err != nil {
			logger.Error("open game file", map[string]any{"err": err.Error(), "ip": ip, "path": path})
			jsonErr(w, "file not found on disk", http.StatusNotFound)
			return
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil {
			logger.Error("stat game file", map[string]any{"err": err.Error(), "ip": ip, "path": path})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, game.FileName))
		start := time.Now()
		logger.Info("download game start", map[string]any{
			"ip":      ip,
			"uuid":    uuid,
			"title":   game.Title,
			"version": game.Version,
			"size":    fmt.Sprintf("%.1f MB", float64(stat.Size())/1e6),
		})
		http.ServeContent(&typeWriter{w, "application/zip"}, r, game.FileName, stat.ModTime(), f)
		logger.Info("download game done", map[string]any{
			"ip":       ip,
			"uuid":     uuid,
			"title":    game.Title,
			"version":  game.Version,
			"duration": time.Since(start).Round(time.Second).String(),
		})
		if err := gdb.IncrementGameDownloads(uuid); err != nil {
			logger.Error("increment game downloads count", map[string]any{"err": err.Error(), "uuid": uuid})
		}
	}
}

// typeWriter locks Content-Type so ServeContent doesn't overwrite it.
type typeWriter struct {
	http.ResponseWriter
	ctype string
}

func (w *typeWriter) WriteHeader(code int) {
	w.Header().Set("Content-Type", w.ctype)
	w.ResponseWriter.WriteHeader(code)
}

// DownloadModpackHandler returns a handler for GET /download/modpack/{uuid}.
func DownloadModpackHandler(mdb *store.ModpacksDB, modpacksDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if r.Method != http.MethodGet {
			jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		uuid := strings.TrimPrefix(r.URL.Path, "/download/modpack/")
		if uuid == "" || strings.Contains(uuid, "/") {
			jsonErr(w, "usage: /download/modpack/{uuid}", http.StatusBadRequest)
			return
		}

		mp, err := mdb.GetModpackByUUID(uuid)
		if err != nil {
			logger.Error("get modpack for download", map[string]any{"err": err.Error(), "ip": ip, "uuid": uuid})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}
		if mp == nil {
			logger.Warn("modpack not found", map[string]any{"ip": ip, "uuid": uuid})
			jsonErr(w, "modpack not found", http.StatusNotFound)
			return
		}

		path := filepath.Join(modpacksDir, mp.FileName)
		f, err := os.Open(path)
		if err != nil {
			logger.Error("open modpack file", map[string]any{"err": err.Error(), "ip": ip, "path": path})
			jsonErr(w, "file not found on disk", http.StatusNotFound)
			return
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil {
			logger.Error("stat modpack file", map[string]any{"err": err.Error(), "ip": ip, "path": path})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, mp.FileName))
		start := time.Now()
		logger.Info("download modpack start", map[string]any{
			"ip":      ip,
			"uuid":    uuid,
			"game":    mp.GameTitle,
			"modpack": mp.ModpackTitle,
			"size":    fmt.Sprintf("%.1f MB", float64(stat.Size())/1e6),
		})
		http.ServeContent(&typeWriter{w, "application/zip"}, r, mp.FileName, stat.ModTime(), f)
		logger.Info("download modpack done", map[string]any{
			"ip":       ip,
			"uuid":     uuid,
			"game":     mp.GameTitle,
			"modpack":  mp.ModpackTitle,
			"duration": time.Since(start).Round(time.Second).String(),
		})
		if err := mdb.IncrementModpackDownloads(uuid); err != nil {
			logger.Error("increment modpack downloads count", map[string]any{"err": err.Error(), "uuid": uuid})
		}
	}
}

// --- Delete ---

// DeleteGameHandler returns a handler for DELETE /admin/game/{uuid}.
func DeleteGameHandler(gdb *store.GamesDB, gamesDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		uuid := strings.TrimPrefix(r.URL.Path, "/admin/game/")
		if uuid == "" || strings.Contains(uuid, "/") {
			jsonErr(w, "usage: /admin/game/{uuid}", http.StatusBadRequest)
			return
		}

		fileName, found, err := gdb.DeleteGameByUUID(uuid)
		if err != nil {
			logger.Error("delete game db", map[string]any{"err": err.Error()})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !found {
			jsonErr(w, "game not found", http.StatusNotFound)
			return
		}

		diskPath := filepath.Join(gamesDir, fileName)
		if err := os.Remove(diskPath); err != nil && !os.IsNotExist(err) {
			logger.Error("delete game file", map[string]any{"err": err.Error(), "path": diskPath})
		}
		logger.Info("game deleted", map[string]any{"uuid": uuid, "file": fileName})
		jsonOK(w, map[string]any{"ok": true})
	}
}

// DeleteModpackHandler returns a handler for DELETE /admin/modpack/{uuid}.
func DeleteModpackHandler(mdb *store.ModpacksDB, modpacksDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		uuid := strings.TrimPrefix(r.URL.Path, "/admin/modpack/")
		if uuid == "" || strings.Contains(uuid, "/") {
			jsonErr(w, "usage: /admin/modpack/{uuid}", http.StatusBadRequest)
			return
		}

		fileName, found, err := mdb.DeleteModpackByUUID(uuid)
		if err != nil {
			logger.Error("delete modpack db", map[string]any{"err": err.Error()})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !found {
			jsonErr(w, "modpack not found", http.StatusNotFound)
			return
		}

		diskPath := filepath.Join(modpacksDir, fileName)
		if err := os.Remove(diskPath); err != nil && !os.IsNotExist(err) {
			logger.Error("delete modpack file", map[string]any{"err": err.Error(), "path": diskPath})
		}
		logger.Info("modpack deleted", map[string]any{"uuid": uuid, "file": fileName})
		jsonOK(w, map[string]any{"ok": true})
	}
}

// --- Admin dispatch (DELETE + PATCH share /admin/game/ and /admin/modpack/ prefix) ---

// GameAdminHandler dispatches DELETE and PATCH for /admin/game/{uuid}.
func GameAdminHandler(gdb *store.GamesDB, gamesDir string) http.HandlerFunc {
	deleteFn := DeleteGameHandler(gdb, gamesDir)
	patchFn := PatchGameHandler(gdb)
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodDelete:
			deleteFn(w, r)
		case http.MethodPatch:
			patchFn(w, r)
		default:
			jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// ModpackAdminHandler dispatches DELETE and PATCH for /admin/modpack/{uuid}.
func ModpackAdminHandler(mdb *store.ModpacksDB, modpacksDir string) http.HandlerFunc {
	deleteFn := DeleteModpackHandler(mdb, modpacksDir)
	patchFn := PatchModpackHandler(mdb)
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodDelete:
			deleteFn(w, r)
		case http.MethodPatch:
			patchFn(w, r)
		default:
			jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// --- Patch ---

// PatchGameHandler returns a handler for PATCH /admin/game/{uuid}.
// Accepts JSON body with optional fields: title, version, app_id, notes, title_notes, launch_exe.
func PatchGameHandler(gdb *store.GamesDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		uuid := strings.TrimPrefix(r.URL.Path, "/admin/game/")
		if uuid == "" || strings.Contains(uuid, "/") {
			jsonErr(w, "usage: /admin/game/{uuid}", http.StatusBadRequest)
			return
		}

		var fields map[string]string
		if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
			jsonErr(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		if len(fields) == 0 {
			jsonErr(w, "no fields to update", http.StatusBadRequest)
			return
		}

		// Validate game exists.
		game, err := gdb.GetGameByUUID(uuid)
		if err != nil {
			logger.Error("get game for patch", map[string]any{"err": err.Error()})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}
		if game == nil {
			jsonErr(w, "game not found", http.StatusNotFound)
			return
		}

		if err := gdb.UpdateGame(uuid, fields); err != nil {
			logger.Error("update game", map[string]any{"err": err.Error(), "uuid": uuid})
			jsonErr(w, err.Error(), http.StatusBadRequest)
			return
		}

		logger.Info("game patched", map[string]any{"uuid": uuid, "fields": fields})
		jsonOK(w, map[string]any{"ok": true})
	}
}

// PatchModpackHandler returns a handler for PATCH /admin/modpack/{uuid}.
// Accepts JSON body with optional fields: game_title, modpack_title, notes.
func PatchModpackHandler(mdb *store.ModpacksDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		uuid := strings.TrimPrefix(r.URL.Path, "/admin/modpack/")
		if uuid == "" || strings.Contains(uuid, "/") {
			jsonErr(w, "usage: /admin/modpack/{uuid}", http.StatusBadRequest)
			return
		}

		var fields map[string]string
		if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
			jsonErr(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		if len(fields) == 0 {
			jsonErr(w, "no fields to update", http.StatusBadRequest)
			return
		}

		mp, err := mdb.GetModpackByUUID(uuid)
		if err != nil {
			logger.Error("get modpack for patch", map[string]any{"err": err.Error()})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}
		if mp == nil {
			jsonErr(w, "modpack not found", http.StatusNotFound)
			return
		}

		if err := mdb.UpdateModpack(uuid, fields); err != nil {
			logger.Error("update modpack", map[string]any{"err": err.Error(), "uuid": uuid})
			jsonErr(w, err.Error(), http.StatusBadRequest)
			return
		}

		logger.Info("modpack patched", map[string]any{"uuid": uuid, "fields": fields})
		jsonOK(w, map[string]any{"ok": true})
	}
}

// --- Disk Quota ---

// DiskQuotaHandler returns a handler for GET /admin/disk-quota.
// It reports the total and used bytes of the filesystem that contains gamesDir.
func DiskQuotaHandler(gamesDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		totalBytes, freeBytes, err := diskUsage(gamesDir)
		if err != nil {
			logger.Error("disk usage", map[string]any{"err": err.Error(), "dir": gamesDir})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}

		jsonOK(w, map[string]any{
			"total_bytes": totalBytes,
			"used_bytes":  totalBytes - freeBytes,
		})
	}
}

// Package handler wires together the HTTP routes for the Nakama server.
// Routes:
//
//	GET  /query                                 - list all games + modpacks (download key)
//	POST /admin/upload/game                     - upload game zip (admin key)
//	POST /admin/upload/modpack                  - upload modpack zip (admin key)
//	GET  /download/game/{title}/{version}       - stream game zip (download key, 1/IP)
//	GET  /download/modpack/{gameTitle}/{modpackTitle} - stream modpack zip (download key, 1/IP)
//	DELETE /admin/game/{title}/{version}        - delete game (admin key)
//	DELETE /admin/modpack/{gameTitle}/{modpackTitle}  - delete modpack (admin key)
package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"nakamaserver/internal/logger"
	"nakamaserver/internal/store"
)

// safeSegment sanitizes a path segment: lower-case, alphanumeric + dot + dash + underscore only.
var unsafeRe = regexp.MustCompile(`[^a-zA-Z0-9.\-_]`)

func sanitize(s string) string {
	return unsafeRe.ReplaceAllString(strings.TrimSpace(s), "_")
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
		if r.Method != http.MethodGet {
			jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		games, err := gdb.ListGames()
		if err != nil {
			logger.Error("list games", map[string]any{"err": err.Error()})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}
		modpacks, err := mdb.ListModpacks()
		if err != nil {
			logger.Error("list modpacks", map[string]any{"err": err.Error()})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}
		jsonOK(w, store.FullCatalog{Games: games, Modpacks: modpacks})
	}
}

// --- Upload ---

// UploadGameHandler returns a handler for POST /admin/upload/game.
// Expects multipart/form-data with fields: title, version, launch_exe, file (the zip).
func UploadGameHandler(gdb *store.GamesDB, gamesDir string, maxBytes int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseMultipartForm(maxBytes); err != nil {
			jsonErr(w, "failed to parse form: "+err.Error(), http.StatusBadRequest)
			return
		}
		title := strings.TrimSpace(r.FormValue("title"))
		version := strings.TrimSpace(r.FormValue("version"))
		launchExe := strings.TrimSpace(r.FormValue("launch_exe"))
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

		inserted, err := gdb.InsertGame(title, version, fileName, launchExe, written)
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

		logger.Info("game uploaded", map[string]any{"title": title, "version": version, "bytes": written})
		jsonOK(w, map[string]any{"ok": true, "file": fileName, "size_bytes": written})
	}
}

// UploadModpackHandler returns a handler for POST /admin/upload/modpack.
// Expects multipart/form-data with fields: game_title, modpack_title, file (the zip).
func UploadModpackHandler(mdb *store.ModpacksDB, modpacksDir string, maxBytes int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseMultipartForm(maxBytes); err != nil {
			jsonErr(w, "failed to parse form: "+err.Error(), http.StatusBadRequest)
			return
		}
		gameTitle := strings.TrimSpace(r.FormValue("game_title"))
		modpackTitle := strings.TrimSpace(r.FormValue("modpack_title"))
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

		inserted, err := mdb.InsertModpack(gameTitle, modpackTitle, fileName, written)
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

		logger.Info("modpack uploaded", map[string]any{"game": gameTitle, "modpack": modpackTitle, "bytes": written})
		jsonOK(w, map[string]any{"ok": true, "file": fileName, "size_bytes": written})
	}
}

// --- Download ---

// DownloadGameHandler returns a handler for GET /download/game/{title}/{version}.
// URL path: /download/game/<title>/<version>
func DownloadGameHandler(gdb *store.GamesDB, gamesDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Strip prefix: /download/game/
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/download/game/"), "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			jsonErr(w, "usage: /download/game/{title}/{version}", http.StatusBadRequest)
			return
		}
		title, version := parts[0], parts[1]

		game, err := gdb.GetGame(title, version)
		if err != nil {
			logger.Error("get game for download", map[string]any{"err": err.Error()})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}
		if game == nil {
			jsonErr(w, "game not found", http.StatusNotFound)
			return
		}

		path := filepath.Join(gamesDir, game.FileName)
		f, err := os.Open(path)
		if err != nil {
			logger.Error("open game file", map[string]any{"err": err.Error(), "path": path})
			jsonErr(w, "file not found on disk", http.StatusNotFound)
			return
		}
		defer f.Close()

		stat, _ := f.Stat()
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, game.FileName))
		if stat != nil {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
		}
		logger.Info("download game start", map[string]any{"title": title, "version": version, "remote": r.RemoteAddr})
		io.Copy(w, f) //nolint:errcheck
	}
}

// DownloadModpackHandler returns a handler for GET /download/modpack/{gameTitle}/{modpackTitle}.
func DownloadModpackHandler(mdb *store.ModpacksDB, modpacksDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/download/modpack/"), "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			jsonErr(w, "usage: /download/modpack/{gameTitle}/{modpackTitle}", http.StatusBadRequest)
			return
		}
		gameTitle, modpackTitle := parts[0], parts[1]

		mp, err := mdb.GetModpack(gameTitle, modpackTitle)
		if err != nil {
			logger.Error("get modpack for download", map[string]any{"err": err.Error()})
			jsonErr(w, "internal error", http.StatusInternalServerError)
			return
		}
		if mp == nil {
			jsonErr(w, "modpack not found", http.StatusNotFound)
			return
		}

		path := filepath.Join(modpacksDir, mp.FileName)
		f, err := os.Open(path)
		if err != nil {
			logger.Error("open modpack file", map[string]any{"err": err.Error(), "path": path})
			jsonErr(w, "file not found on disk", http.StatusNotFound)
			return
		}
		defer f.Close()

		stat, _ := f.Stat()
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, mp.FileName))
		if stat != nil {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
		}
		logger.Info("download modpack start", map[string]any{"game": gameTitle, "modpack": modpackTitle, "remote": r.RemoteAddr})
		io.Copy(w, f) //nolint:errcheck
	}
}

// --- Delete ---

// DeleteGameHandler returns a handler for DELETE /admin/game/{title}/{version}.
func DeleteGameHandler(gdb *store.GamesDB, gamesDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/admin/game/"), "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			jsonErr(w, "usage: /admin/game/{title}/{version}", http.StatusBadRequest)
			return
		}
		title, version := parts[0], parts[1]

		fileName, found, err := gdb.DeleteGame(title, version)
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
			// DB record already deleted; log but return success to caller.
		}
		logger.Info("game deleted", map[string]any{"title": title, "version": version})
		jsonOK(w, map[string]any{"ok": true})
	}
}

// DeleteModpackHandler returns a handler for DELETE /admin/modpack/{gameTitle}/{modpackTitle}.
func DeleteModpackHandler(mdb *store.ModpacksDB, modpacksDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/admin/modpack/"), "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			jsonErr(w, "usage: /admin/modpack/{gameTitle}/{modpackTitle}", http.StatusBadRequest)
			return
		}
		gameTitle, modpackTitle := parts[0], parts[1]

		fileName, found, err := mdb.DeleteModpack(gameTitle, modpackTitle)
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
		logger.Info("modpack deleted", map[string]any{"game": gameTitle, "modpack": modpackTitle})
		jsonOK(w, map[string]any{"ok": true})
	}
}

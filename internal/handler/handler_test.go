package handler_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nakamaserver/internal/handler"
	"nakamaserver/internal/store"
)

func TestQueryHandler(t *testing.T) {
	tmpDir := t.TempDir()
	gamesDB, err := store.OpenGamesDB(tmpDir)
	if err != nil {
		t.Fatalf("OpenGamesDB: %v", err)
	}
	defer gamesDB.Close()
	modpacksDB, err := store.OpenModpacksDB(tmpDir)
	if err != nil {
		t.Fatalf("OpenModpacksDB: %v", err)
	}
	defer modpacksDB.Close()

	// Insert test game & modpack
	_, _, err = gamesDB.InsertGame("Minecraft", "1.20", "Minecraft_1.20.zip", "minecraft.exe", "", "", 1000)
	if err != nil {
		t.Fatalf("InsertGame: %v", err)
	}
	_, _, err = modpacksDB.InsertModpack("Minecraft", "RLCraft", "Minecraft_RLCraft.zip", "", 500)
	if err != nil {
		t.Fatalf("InsertModpack: %v", err)
	}

	handlerFunc := handler.QueryHandler(gamesDB, modpacksDB)
	req := httptest.NewRequest("GET", "/query", nil)
	rr := httptest.NewRecorder()

	handlerFunc.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rr.Code)
	}

	var catalog store.FullCatalog
	if err := json.Unmarshal(rr.Body.Bytes(), &catalog); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(catalog.Games) != 1 || catalog.Games[0].Title != "Minecraft" {
		t.Errorf("unexpected games list: %+v", catalog.Games)
	}
	if len(catalog.Modpacks) != 1 || catalog.Modpacks[0].ModpackTitle != "RLCraft" {
		t.Errorf("unexpected modpacks list: %+v", catalog.Modpacks)
	}
}

func TestUploadDownloadDeleteGame(t *testing.T) {
	tmpDir := t.TempDir()
	gamesDB, err := store.OpenGamesDB(tmpDir)
	if err != nil {
		t.Fatalf("OpenGamesDB: %v", err)
	}
	defer gamesDB.Close()

	gamesDir := filepath.Join(tmpDir, "games")
	uploadHandler := handler.UploadGameHandler(gamesDB, gamesDir, 10*1024*1024)

	// Prepare multipart form upload
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("title", "Super Mario")
	_ = writer.WriteField("version", "1.0")
	_ = writer.WriteField("launch_exe", "mario.exe")
	_ = writer.WriteField("app_id", "123456")
	part, err := writer.CreateFormFile("file", "mario.zip")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	_, _ = part.Write([]byte("fake zip contents"))
	_ = writer.Close()

	req := httptest.NewRequest("POST", "/admin/upload/game", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()

	uploadHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected upload to succeed, got %d: %s", rr.Code, rr.Body.String())
	}

	// Parse UUID from upload response
	var uploadResp struct {
		OK        bool   `json:"ok"`
		UUID      string `json:"uuid"`
		File      string `json:"file"`
		SizeBytes int64  `json:"size_bytes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &uploadResp); err != nil {
		t.Fatalf("failed to decode upload response: %v", err)
	}
	if uploadResp.UUID == "" {
		t.Fatal("expected non-empty uuid in upload response")
	}

	// Verify game file on disk
	expectedPath := filepath.Join(gamesDir, "Super_Mario_1.0.zip")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("expected file at %s, but does not exist", expectedPath)
	}

	// Test GET /download/game/{uuid}
	downloadHandler := handler.DownloadGameHandler(gamesDB, gamesDir)
	reqDl := httptest.NewRequest("GET", "/download/game/"+uploadResp.UUID, nil)
	rrDl := httptest.NewRecorder()

	downloadHandler.ServeHTTP(rrDl, reqDl)

	if rrDl.Code != http.StatusOK {
		t.Errorf("expected download status 200, got %d: %s", rrDl.Code, rrDl.Body.String())
	}
	if rrDl.Header().Get("Content-Type") != "application/zip" {
		t.Errorf("unexpected content type: %s", rrDl.Header().Get("Content-Type"))
	}
	if rrDl.Body.String() != "fake zip contents" {
		t.Errorf("unexpected download body: %s", rrDl.Body.String())
	}

	// Verify download count is incremented in DB
	game, err := gamesDB.GetGameByUUID(uploadResp.UUID)
	if err != nil {
		t.Fatalf("GetGameByUUID: %v", err)
	}
	if game == nil {
		t.Fatal("expected game to exist")
	}
	if game.Downloads != 1 {
		t.Errorf("expected game downloads count to be 1, got %d", game.Downloads)
	}
	if game.AppID != "123456" {
		t.Errorf("expected app_id=123456, got %s", game.AppID)
	}

	// Test DELETE /admin/game/{uuid}
	deleteHandler := handler.DeleteGameHandler(gamesDB, gamesDir)
	reqDel := httptest.NewRequest("DELETE", "/admin/game/"+uploadResp.UUID, nil)
	rrDel := httptest.NewRecorder()

	deleteHandler.ServeHTTP(rrDel, reqDel)

	if rrDel.Code != http.StatusOK {
		t.Errorf("expected delete status 200, got %d: %s", rrDel.Code, rrDel.Body.String())
	}

	// Verify file is removed from disk
	if _, err := os.Stat(expectedPath); !os.IsNotExist(err) {
		t.Errorf("expected file %s to be deleted, but it still exists", expectedPath)
	}
}

func TestUploadDownloadDeleteModpack(t *testing.T) {
	tmpDir := t.TempDir()
	modpacksDB, err := store.OpenModpacksDB(tmpDir)
	if err != nil {
		t.Fatalf("OpenModpacksDB: %v", err)
	}
	defer modpacksDB.Close()

	modpacksDir := filepath.Join(tmpDir, "modpacks")
	uploadHandler := handler.UploadModpackHandler(modpacksDB, modpacksDir, 10*1024*1024)

	// Prepare multipart form upload
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("game_title", "Doom")
	_ = writer.WriteField("modpack_title", "Brutal Doom")
	part, err := writer.CreateFormFile("file", "brutal.zip")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	_, _ = part.Write([]byte("brutal modpack contents"))
	_ = writer.Close()

	req := httptest.NewRequest("POST", "/admin/upload/modpack", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()

	uploadHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected upload to succeed, got %d: %s", rr.Code, rr.Body.String())
	}

	var uploadResp struct {
		OK        bool   `json:"ok"`
		UUID      string `json:"uuid"`
		File      string `json:"file"`
		SizeBytes int64  `json:"size_bytes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &uploadResp); err != nil {
		t.Fatalf("failed to decode upload response: %v", err)
	}
	if uploadResp.UUID == "" {
		t.Fatal("expected non-empty uuid in upload response")
	}

	// Verify modpack file on disk
	expectedPath := filepath.Join(modpacksDir, "Doom_Brutal_Doom.zip")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("expected file at %s, but does not exist", expectedPath)
	}

	// Test GET /download/modpack/{uuid}
	downloadHandler := handler.DownloadModpackHandler(modpacksDB, modpacksDir)
	reqDl := httptest.NewRequest("GET", "/download/modpack/"+uploadResp.UUID, nil)
	rrDl := httptest.NewRecorder()

	downloadHandler.ServeHTTP(rrDl, reqDl)

	if rrDl.Code != http.StatusOK {
		t.Errorf("expected download status 200, got %d: %s", rrDl.Code, rrDl.Body.String())
	}
	if rrDl.Body.String() != "brutal modpack contents" {
		t.Errorf("unexpected download body: %s", rrDl.Body.String())
	}

	// Verify download count is incremented in DB
	mp, err := modpacksDB.GetModpackByUUID(uploadResp.UUID)
	if err != nil {
		t.Fatalf("GetModpackByUUID: %v", err)
	}
	if mp == nil {
		t.Fatal("expected modpack to exist")
	}
	if mp.Downloads != 1 {
		t.Errorf("expected modpack downloads count to be 1, got %d", mp.Downloads)
	}

	// Test DELETE /admin/modpack/{uuid}
	deleteHandler := handler.DeleteModpackHandler(modpacksDB, modpacksDir)
	reqDel := httptest.NewRequest("DELETE", "/admin/modpack/"+uploadResp.UUID, nil)
	rrDel := httptest.NewRecorder()

	deleteHandler.ServeHTTP(rrDel, reqDel)

	if rrDel.Code != http.StatusOK {
		t.Errorf("expected delete status 200, got %d: %s", rrDel.Code, rrDel.Body.String())
	}

	// Verify file is removed from disk
	if _, err := os.Stat(expectedPath); !os.IsNotExist(err) {
		t.Errorf("expected file %s to be deleted, but it still exists", expectedPath)
	}
}

func TestPatchGame(t *testing.T) {
	tmpDir := t.TempDir()
	gamesDB, err := store.OpenGamesDB(tmpDir)
	if err != nil {
		t.Fatalf("OpenGamesDB: %v", err)
	}
	defer gamesDB.Close()

	uuid, _, err := gamesDB.InsertGame("Patch Game", "1.0", "patch.zip", "patch.exe", "old_steam", "", 100)
	if err != nil {
		t.Fatalf("InsertGame: %v", err)
	}

	patchHandler := handler.PatchGameHandler(gamesDB)

	// PATCH app_id and launch_exe
	patchBody := `{"app_id": "new_steam", "launch_exe": "new_exe.exe"}`
	req := httptest.NewRequest("PATCH", "/admin/game/"+uuid, strings.NewReader(patchBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	patchHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	game, err := gamesDB.GetGameByUUID(uuid)
	if err != nil {
		t.Fatalf("GetGameByUUID: %v", err)
	}
	if game.AppID != "new_steam" {
		t.Errorf("expected app_id=new_steam, got %s", game.AppID)
	}
	if game.LaunchExe != "new_exe.exe" {
		t.Errorf("expected launch_exe=new_exe.exe, got %s", game.LaunchExe)
	}
}

func TestPatchModpack(t *testing.T) {
	tmpDir := t.TempDir()
	modpacksDB, err := store.OpenModpacksDB(tmpDir)
	if err != nil {
		t.Fatalf("OpenModpacksDB: %v", err)
	}
	defer modpacksDB.Close()

	uuid, _, err := modpacksDB.InsertModpack("Old Game", "Old Mod", "old.zip", "", 200)
	if err != nil {
		t.Fatalf("InsertModpack: %v", err)
	}

	patchHandler := handler.PatchModpackHandler(modpacksDB)

	patchBody := `{"game_title": "New Game", "modpack_title": "New Mod"}`
	req := httptest.NewRequest("PATCH", "/admin/modpack/"+uuid, strings.NewReader(patchBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	patchHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	mp, err := modpacksDB.GetModpackByUUID(uuid)
	if err != nil {
		t.Fatalf("GetModpackByUUID: %v", err)
	}
	if mp.GameTitle != "New Game" {
		t.Errorf("expected game_title=New Game, got %s", mp.GameTitle)
	}
	if mp.ModpackTitle != "New Mod" {
		t.Errorf("expected modpack_title=New Mod, got %s", mp.ModpackTitle)
	}
}

func TestDiskQuotaHandler(t *testing.T) {
	tmpDir := t.TempDir()

	handlerFunc := handler.DiskQuotaHandler(tmpDir)
	req := httptest.NewRequest("GET", "/admin/disk-quota", nil)
	rr := httptest.NewRecorder()

	handlerFunc.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		TotalBytes int64 `json:"total_bytes"`
		UsedBytes  int64 `json:"used_bytes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.TotalBytes <= 0 {
		t.Errorf("expected total_bytes > 0, got %d", resp.TotalBytes)
	}
	if resp.UsedBytes < 0 {
		t.Errorf("expected used_bytes >= 0, got %d", resp.UsedBytes)
	}
	if resp.TotalBytes < resp.UsedBytes {
		t.Errorf("expected total_bytes (%d) >= used_bytes (%d)", resp.TotalBytes, resp.UsedBytes)
	}
	if freeBytes := resp.TotalBytes - resp.UsedBytes; freeBytes < 0 {
		t.Errorf("expected free_bytes >= 0, got %d", freeBytes)
	}
}

package handler_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	_, err = gamesDB.InsertGame("Minecraft", "1.20", "Minecraft_1.20.zip", "minecraft.exe", 1000)
	if err != nil {
		t.Fatalf("InsertGame: %v", err)
	}
	_, err = modpacksDB.InsertModpack("Minecraft", "RLCraft", "Minecraft_RLCraft.zip", 500)
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

	// Verify game file on disk
	expectedPath := filepath.Join(gamesDir, "Super_Mario_1.0.zip")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("expected file at %s, but does not exist", expectedPath)
	}

	// Test GET /download/game/Super%20Mario/1.0
	downloadHandler := handler.DownloadGameHandler(gamesDB, gamesDir)
	reqDl := httptest.NewRequest("GET", "/download/game/Super%20Mario/1.0", nil)
	rrDl := httptest.NewRecorder()

	downloadHandler.ServeHTTP(rrDl, reqDl)

	if rrDl.Code != http.StatusOK {
		t.Errorf("expected download status 200, got %d", rrDl.Code)
	}
	if rrDl.Header().Get("Content-Type") != "application/zip" {
		t.Errorf("unexpected content type: %s", rrDl.Header().Get("Content-Type"))
	}
	if rrDl.Body.String() != "fake zip contents" {
		t.Errorf("unexpected download body: %s", rrDl.Body.String())
	}

	// Verify download count is incremented in DB
	game, err := gamesDB.GetGame("Super Mario", "1.0")
	if err != nil {
		t.Fatalf("GetGame: %v", err)
	}
	if game == nil {
		t.Fatal("expected game to exist")
	}
	if game.Downloads != 1 {
		t.Errorf("expected game downloads count to be 1, got %d", game.Downloads)
	}

	// Test DELETE /admin/game/Super%20Mario/1.0
	deleteHandler := handler.DeleteGameHandler(gamesDB, gamesDir)
	reqDel := httptest.NewRequest("DELETE", "/admin/game/Super%20Mario/1.0", nil)
	rrDel := httptest.NewRecorder()

	deleteHandler.ServeHTTP(rrDel, reqDel)

	if rrDel.Code != http.StatusOK {
		t.Errorf("expected delete status 200, got %d", rrDel.Code)
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

	// Verify modpack file on disk
	expectedPath := filepath.Join(modpacksDir, "Doom_Brutal_Doom.zip")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("expected file at %s, but does not exist", expectedPath)
	}

	// Test GET /download/modpack/Doom/Brutal%20Doom
	downloadHandler := handler.DownloadModpackHandler(modpacksDB, modpacksDir)
	reqDl := httptest.NewRequest("GET", "/download/modpack/Doom/Brutal%20Doom", nil)
	rrDl := httptest.NewRecorder()

	downloadHandler.ServeHTTP(rrDl, reqDl)

	if rrDl.Code != http.StatusOK {
		t.Errorf("expected download status 200, got %d", rrDl.Code)
	}
	if rrDl.Body.String() != "brutal modpack contents" {
		t.Errorf("unexpected download body: %s", rrDl.Body.String())
	}

	// Verify download count is incremented in DB
	mp, err := modpacksDB.GetModpack("Doom", "Brutal Doom")
	if err != nil {
		t.Fatalf("GetModpack: %v", err)
	}
	if mp == nil {
		t.Fatal("expected modpack to exist")
	}
	if mp.Downloads != 1 {
		t.Errorf("expected modpack downloads count to be 1, got %d", mp.Downloads)
	}

	// Test DELETE /admin/modpack/Doom/Brutal%20Doom
	deleteHandler := handler.DeleteModpackHandler(modpacksDB, modpacksDir)
	reqDel := httptest.NewRequest("DELETE", "/admin/modpack/Doom/Brutal%20Doom", nil)
	rrDel := httptest.NewRecorder()

	deleteHandler.ServeHTTP(rrDel, reqDel)

	if rrDel.Code != http.StatusOK {
		t.Errorf("expected delete status 200, got %d", rrDel.Code)
	}

	// Verify file is removed from disk
	if _, err := os.Stat(expectedPath); !os.IsNotExist(err) {
		t.Errorf("expected file %s to be deleted, but it still exists", expectedPath)
	}
}

func TestDiskQuotaHandler(t *testing.T) {
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

	// Insert test data
	_, err = gamesDB.InsertGame("Game1", "1.0", "Game1_1.0.zip", "g1.exe", 1000)
	if err != nil {
		t.Fatalf("InsertGame: %v", err)
	}
	_, err = gamesDB.InsertGame("Game2", "2.0", "Game2_2.0.zip", "g2.exe", 2000)
	if err != nil {
		t.Fatalf("InsertGame: %v", err)
	}
	_, err = modpacksDB.InsertModpack("Game1", "Mod1", "Game1_Mod1.zip", 500)
	if err != nil {
		t.Fatalf("InsertModpack: %v", err)
	}

	const quotaBytes int64 = 100 * 1024 * 1024 * 1024 // 100 GB
	handlerFunc := handler.DiskQuotaHandler(gamesDB, modpacksDB, quotaBytes)
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

	if resp.TotalBytes != quotaBytes {
		t.Errorf("expected total_bytes %d, got %d", quotaBytes, resp.TotalBytes)
	}
	if resp.UsedBytes != 3500 {
		t.Errorf("expected used_bytes 3500, got %d", resp.UsedBytes)
	}
}

package store_test

import (
	"testing"

	"nakamaserver/internal/store"
)

func TestGamesDB(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := store.OpenGamesDB(tmpDir)
	if err != nil {
		t.Fatalf("OpenGamesDB failed: %v", err)
	}
	defer db.Close()

	// List initial games
	games, err := db.ListGames()
	if err != nil {
		t.Fatalf("ListGames failed: %v", err)
	}
	if len(games) != 0 {
		t.Errorf("expected 0 games, got %d", len(games))
	}

	// Insert game
	uuid, ok, err := db.InsertGame("Among Us", "1.13", "among_us_1.13.zip", "among_us.exe", "", "", "", 12345)
	if err != nil {
		t.Fatalf("InsertGame failed: %v", err)
	}
	if !ok {
		t.Error("expected insert to succeed")
	}
	if uuid == "" {
		t.Error("expected non-empty uuid")
	}

	// Insert duplicate game
	_, ok, err = db.InsertGame("Among Us", "1.13", "among_us_1.13_dup.zip", "among_us.exe", "", "", "", 12345)
	if err != nil {
		t.Fatalf("InsertGame duplicate failed: %v", err)
	}
	if ok {
		t.Error("expected duplicate insert to be ignored")
	}

	// Get game by UUID
	game, err := db.GetGameByUUID(uuid)
	if err != nil {
		t.Fatalf("GetGameByUUID failed: %v", err)
	}
	if game == nil {
		t.Fatal("expected to find game")
	}
	if game.Title != "Among Us" || game.Downloads != 0 {
		t.Errorf("unexpected game fields: %+v", game)
	}

	// Increment downloads by UUID
	err = db.IncrementGameDownloads(uuid)
	if err != nil {
		t.Fatalf("IncrementGameDownloads failed: %v", err)
	}

	// Get game again to check downloads
	game, err = db.GetGameByUUID(uuid)
	if err != nil {
		t.Fatalf("GetGameByUUID failed: %v", err)
	}
	if game.Downloads != 1 {
		t.Errorf("expected 1 download, got %d", game.Downloads)
	}

	// List games
	games, err = db.ListGames()
	if err != nil {
		t.Fatalf("ListGames failed: %v", err)
	}
	if len(games) != 1 || games[0].Downloads != 1 {
		t.Errorf("expected 1 game with 1 download, got %+v", games)
	}

	// Delete game by UUID
	fileName, found, err := db.DeleteGameByUUID(uuid)
	if err != nil {
		t.Fatalf("DeleteGameByUUID failed: %v", err)
	}
	if !found || fileName != "among_us_1.13.zip" {
		t.Errorf("unexpected delete response: found=%v, fileName=%s", found, fileName)
	}

	// Verify deleted
	game, err = db.GetGameByUUID(uuid)
	if err != nil {
		t.Fatalf("GetGameByUUID after delete failed: %v", err)
	}
	if game != nil {
		t.Error("expected game to be deleted")
	}

	// Test UpdateGame
	uuid2, ok, err := db.InsertGame("Minecraft", "1.20", "mc.zip", "mc.exe", "12345", "", "", 5000)
	if err != nil {
		t.Fatalf("InsertGame failed: %v", err)
	}
	if !ok {
		t.Fatal("expected insert to succeed")
	}

	err = db.UpdateGame(uuid2, map[string]string{"app_id": "99999", "launch_exe": "new_exe.exe"})
	if err != nil {
		t.Fatalf("UpdateGame failed: %v", err)
	}

	game, err = db.GetGameByUUID(uuid2)
	if err != nil {
		t.Fatalf("GetGameByUUID failed: %v", err)
	}
	if game.AppID != "99999" || game.LaunchExe != "new_exe.exe" {
		t.Errorf("unexpected updated fields: app_id=%s, launch_exe=%s", game.AppID, game.LaunchExe)
	}
}

func TestModpacksDB(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := store.OpenModpacksDB(tmpDir)
	if err != nil {
		t.Fatalf("OpenModpacksDB failed: %v", err)
	}
	defer db.Close()

	// List initial modpacks
	modpacks, err := db.ListModpacks()
	if err != nil {
		t.Fatalf("ListModpacks failed: %v", err)
	}
	if len(modpacks) != 0 {
		t.Errorf("expected 0 modpacks, got %d", len(modpacks))
	}

	// Insert modpack
	uuid, ok, err := db.InsertModpack("Among Us", "Town of Us", "Among_Us_Town_of_Us.zip", "", 54321)
	if err != nil {
		t.Fatalf("InsertModpack failed: %v", err)
	}
	if !ok {
		t.Error("expected insert to succeed")
	}
	if uuid == "" {
		t.Error("expected non-empty uuid")
	}

	// Get modpack by UUID
	mp, err := db.GetModpackByUUID(uuid)
	if err != nil {
		t.Fatalf("GetModpackByUUID failed: %v", err)
	}
	if mp == nil {
		t.Fatal("expected to find modpack")
	}
	if mp.ModpackTitle != "Town of Us" || mp.Downloads != 0 {
		t.Errorf("unexpected modpack fields: %+v", mp)
	}

	// Increment downloads
	err = db.IncrementModpackDownloads(uuid)
	if err != nil {
		t.Fatalf("IncrementModpackDownloads failed: %v", err)
	}

	// Get modpack again to check downloads
	mp, err = db.GetModpackByUUID(uuid)
	if err != nil {
		t.Fatalf("GetModpackByUUID failed: %v", err)
	}
	if mp.Downloads != 1 {
		t.Errorf("expected 1 download, got %d", mp.Downloads)
	}

	// Test UpdateModpack
	err = db.UpdateModpack(uuid, map[string]string{"game_title": "New Game", "modpack_title": "Updated Mod"})
	if err != nil {
		t.Fatalf("UpdateModpack failed: %v", err)
	}

	mp, err = db.GetModpackByUUID(uuid)
	if err != nil {
		t.Fatalf("GetModpackByUUID failed: %v", err)
	}
	if mp.GameTitle != "New Game" || mp.ModpackTitle != "Updated Mod" {
		t.Errorf("unexpected updated fields: game_title=%s, modpack_title=%s", mp.GameTitle, mp.ModpackTitle)
	}
}

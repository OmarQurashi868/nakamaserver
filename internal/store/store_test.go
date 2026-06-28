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
	ok, err := db.InsertGame("Among Us", "1.13", "among_us_1.13.zip", "among_us.exe", 12345)
	if err != nil {
		t.Fatalf("InsertGame failed: %v", err)
	}
	if !ok {
		t.Error("expected insert to succeed")
	}

	// Insert duplicate game
	ok, err = db.InsertGame("Among Us", "1.13", "among_us_1.13_dup.zip", "among_us.exe", 12345)
	if err != nil {
		t.Fatalf("InsertGame duplicate failed: %v", err)
	}
	if ok {
		t.Error("expected duplicate insert to be ignored")
	}

	// Get game
	game, err := db.GetGame("Among Us", "1.13")
	if err != nil {
		t.Fatalf("GetGame failed: %v", err)
	}
	if game == nil {
		t.Fatal("expected to find game")
	}
	if game.Title != "Among Us" || game.Downloads != 0 {
		t.Errorf("unexpected game fields: %+v", game)
	}

	// Increment downloads
	err = db.IncrementGameDownloads("Among Us", "1.13")
	if err != nil {
		t.Fatalf("IncrementGameDownloads failed: %v", err)
	}

	// Get game again to check downloads
	game, err = db.GetGame("Among Us", "1.13")
	if err != nil {
		t.Fatalf("GetGame failed: %v", err)
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

	// Delete game
	fileName, found, err := db.DeleteGame("Among Us", "1.13")
	if err != nil {
		t.Fatalf("DeleteGame failed: %v", err)
	}
	if !found || fileName != "among_us_1.13.zip" {
		t.Errorf("unexpected delete response: found=%v, fileName=%s", found, fileName)
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
	ok, err := db.InsertModpack("Among Us", "Town of Us", "Among_Us_Town_of_Us.zip", 54321)
	if err != nil {
		t.Fatalf("InsertModpack failed: %v", err)
	}
	if !ok {
		t.Error("expected insert to succeed")
	}

	// Get modpack
	mp, err := db.GetModpack("Among Us", "Town of Us")
	if err != nil {
		t.Fatalf("GetModpack failed: %v", err)
	}
	if mp == nil {
		t.Fatal("expected to find modpack")
	}
	if mp.ModpackTitle != "Town of Us" || mp.Downloads != 0 {
		t.Errorf("unexpected modpack fields: %+v", mp)
	}

	// Increment downloads
	err = db.IncrementModpackDownloads("Among Us", "Town of Us")
	if err != nil {
		t.Fatalf("IncrementModpackDownloads failed: %v", err)
	}

	// Get modpack again to check downloads
	mp, err = db.GetModpack("Among Us", "Town of Us")
	if err != nil {
		t.Fatalf("GetModpack failed: %v", err)
	}
	if mp.Downloads != 1 {
		t.Errorf("expected 1 download, got %d", mp.Downloads)
	}
}

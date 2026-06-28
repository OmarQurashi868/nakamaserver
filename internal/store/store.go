// Package store manages the two SQLite databases (games and modpacks)
// and the associated file operations.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// GamesDB wraps the games SQLite database.
type GamesDB struct {
	db *sql.DB
}

// ModpacksDB wraps the modpacks SQLite database.
type ModpacksDB struct {
	db *sql.DB
}

// Game represents one row in the games table.
type Game struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Version     string `json:"version"`
	FileName    string `json:"file_name"`
	FileSizeB   int64  `json:"file_size_bytes"`
	LaunchExe   string `json:"launch_exe"`
	UploadedAt  string `json:"uploaded_at"`
}

// Modpack represents one row in the modpacks table.
type Modpack struct {
	ID           int64  `json:"id"`
	GameTitle    string `json:"game_title"`
	ModpackTitle string `json:"modpack_title"`
	FileName     string `json:"file_name"`
	FileSizeB    int64  `json:"file_size_bytes"`
	UploadedAt   string `json:"uploaded_at"`
}

// FullCatalog is the payload returned by GET /query.
type FullCatalog struct {
	Games    []Game    `json:"games"`
	Modpacks []Modpack `json:"modpacks"`
}

// OpenGamesDB opens (or creates) the games SQLite database.
func OpenGamesDB(dir string) (*GamesDB, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create games dir: %w", err)
	}
	path := filepath.Join(dir, "games.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open games db: %w", err)
	}
	if err := migrateGames(db); err != nil {
		return nil, err
	}
	return &GamesDB{db: db}, nil
}

// OpenModpacksDB opens (or creates) the modpacks SQLite database.
func OpenModpacksDB(dir string) (*ModpacksDB, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create modpacks dir: %w", err)
	}
	path := filepath.Join(dir, "modpacks.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open modpacks db: %w", err)
	}
	if err := migrateModpacks(db); err != nil {
		return nil, err
	}
	return &ModpacksDB{db: db}, nil
}

func migrateGames(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS games (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			title       TEXT NOT NULL,
			version     TEXT NOT NULL,
			file_name   TEXT NOT NULL,
			file_size_b INTEGER NOT NULL,
			launch_exe  TEXT NOT NULL,
			uploaded_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
			UNIQUE(title, version)
		)
	`)
	return err
}

func migrateModpacks(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS modpacks (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			game_title    TEXT NOT NULL,
			modpack_title TEXT NOT NULL,
			file_name     TEXT NOT NULL,
			file_size_b   INTEGER NOT NULL,
			uploaded_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
			UNIQUE(game_title, modpack_title)
		)
	`)
	return err
}

// --- Games methods ---

// InsertGame inserts a game record. Returns false if already exists.
func (g *GamesDB) InsertGame(title, version, fileName, launchExe string, sizeB int64) (bool, error) {
	res, err := g.db.Exec(
		`INSERT OR IGNORE INTO games (title, version, file_name, file_size_b, launch_exe) VALUES (?,?,?,?,?)`,
		title, version, fileName, sizeB, launchExe,
	)
	if err != nil {
		return false, fmt.Errorf("insert game: %w", err)
	}
	rows, _ := res.RowsAffected()
	return rows > 0, nil
}

// GetGame looks up a game by title and version.
func (g *GamesDB) GetGame(title, version string) (*Game, error) {
	row := g.db.QueryRow(
		`SELECT id, title, version, file_name, file_size_b, launch_exe, uploaded_at FROM games WHERE title=? AND version=?`,
		title, version,
	)
	var game Game
	err := row.Scan(&game.ID, &game.Title, &game.Version, &game.FileName, &game.FileSizeB, &game.LaunchExe, &game.UploadedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &game, nil
}

// DeleteGame removes a game record. Returns false if not found.
func (g *GamesDB) DeleteGame(title, version string) (string, bool, error) {
	row := g.db.QueryRow(`SELECT file_name FROM games WHERE title=? AND version=?`, title, version)
	var fileName string
	if err := row.Scan(&fileName); err == sql.ErrNoRows {
		return "", false, nil
	} else if err != nil {
		return "", false, err
	}
	_, err := g.db.Exec(`DELETE FROM games WHERE title=? AND version=?`, title, version)
	return fileName, true, err
}

// ListGames returns all game records.
func (g *GamesDB) ListGames() ([]Game, error) {
	rows, err := g.db.Query(`SELECT id, title, version, file_name, file_size_b, launch_exe, uploaded_at FROM games ORDER BY title, version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var games []Game
	for rows.Next() {
		var game Game
		if err := rows.Scan(&game.ID, &game.Title, &game.Version, &game.FileName, &game.FileSizeB, &game.LaunchExe, &game.UploadedAt); err != nil {
			return nil, err
		}
		games = append(games, game)
	}
	if games == nil {
		games = []Game{}
	}
	return games, rows.Err()
}

// --- Modpacks methods ---

// InsertModpack inserts a modpack record. Returns false if already exists.
func (m *ModpacksDB) InsertModpack(gameTitle, modpackTitle, fileName string, sizeB int64) (bool, error) {
	res, err := m.db.Exec(
		`INSERT OR IGNORE INTO modpacks (game_title, modpack_title, file_name, file_size_b) VALUES (?,?,?,?)`,
		gameTitle, modpackTitle, fileName, sizeB,
	)
	if err != nil {
		return false, fmt.Errorf("insert modpack: %w", err)
	}
	rows, _ := res.RowsAffected()
	return rows > 0, nil
}

// GetModpack looks up a modpack by game title and modpack title.
func (m *ModpacksDB) GetModpack(gameTitle, modpackTitle string) (*Modpack, error) {
	row := m.db.QueryRow(
		`SELECT id, game_title, modpack_title, file_name, file_size_b, uploaded_at FROM modpacks WHERE game_title=? AND modpack_title=?`,
		gameTitle, modpackTitle,
	)
	var mp Modpack
	err := row.Scan(&mp.ID, &mp.GameTitle, &mp.ModpackTitle, &mp.FileName, &mp.FileSizeB, &mp.UploadedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &mp, nil
}

// DeleteModpack removes a modpack record. Returns false if not found.
func (m *ModpacksDB) DeleteModpack(gameTitle, modpackTitle string) (string, bool, error) {
	row := m.db.QueryRow(`SELECT file_name FROM modpacks WHERE game_title=? AND modpack_title=?`, gameTitle, modpackTitle)
	var fileName string
	if err := row.Scan(&fileName); err == sql.ErrNoRows {
		return "", false, nil
	} else if err != nil {
		return "", false, err
	}
	_, err := m.db.Exec(`DELETE FROM modpacks WHERE game_title=? AND modpack_title=?`, gameTitle, modpackTitle)
	return fileName, true, err
}

// ListModpacks returns all modpack records.
func (m *ModpacksDB) ListModpacks() ([]Modpack, error) {
	rows, err := m.db.Query(`SELECT id, game_title, modpack_title, file_name, file_size_b, uploaded_at FROM modpacks ORDER BY game_title, modpack_title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var modpacks []Modpack
	for rows.Next() {
		var mp Modpack
		if err := rows.Scan(&mp.ID, &mp.GameTitle, &mp.ModpackTitle, &mp.FileName, &mp.FileSizeB, &mp.UploadedAt); err != nil {
			return nil, err
		}
		modpacks = append(modpacks, mp)
	}
	if modpacks == nil {
		modpacks = []Modpack{}
	}
	return modpacks, rows.Err()
}

// Package store manages the two SQLite databases (games and modpacks)
// and the associated file operations.
package store

import (
	"crypto/rand"
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
	ID         int64  `json:"id"`
	UUID       string `json:"uuid"`
	Title      string `json:"title"`
	Version    string `json:"version"`
	FileName   string `json:"file_name"`
	FileSizeB  int64  `json:"file_size_bytes"`
	LaunchExe  string `json:"launch_exe"`
	AppID      string `json:"app_id"`
	Notes      string `json:"notes"`
	TitleNotes string `json:"title_notes"`
	UploadedAt string `json:"uploaded_at"`
	Downloads  int64  `json:"downloads"`
}

// Modpack represents one row in the modpacks table.
type Modpack struct {
	ID           int64  `json:"id"`
	UUID         string `json:"uuid"`
	GameTitle    string `json:"game_title"`
	ModpackTitle string `json:"modpack_title"`
	FileName     string `json:"file_name"`
	FileSizeB    int64  `json:"file_size_bytes"`
	Notes        string `json:"notes"`
	UploadedAt   string `json:"uploaded_at"`
	Downloads    int64  `json:"downloads"`
}

// FullCatalog is the payload returned by GET /query.
type FullCatalog struct {
	Games    []Game    `json:"games"`
	Modpacks []Modpack `json:"modpacks"`
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
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
			uuid        TEXT NOT NULL UNIQUE,
			title       TEXT NOT NULL,
			version     TEXT NOT NULL,
			file_name   TEXT NOT NULL,
			file_size_b INTEGER NOT NULL,
			launch_exe  TEXT NOT NULL,
			app_id      TEXT NOT NULL DEFAULT '',
			notes       TEXT NOT NULL DEFAULT '',
			title_notes TEXT NOT NULL DEFAULT '',
			uploaded_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
			downloads   INTEGER NOT NULL DEFAULT 0,
			UNIQUE(title, version)
		)
	`)
	if err != nil {
		return err
	}
	var count int
	err = db.QueryRow("SELECT count(*) FROM pragma_table_info('games') WHERE name='downloads'").Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		_, err = db.Exec("ALTER TABLE games ADD COLUMN downloads INTEGER NOT NULL DEFAULT 0")
		if err != nil {
			return err
		}
	}
	err = db.QueryRow("SELECT count(*) FROM pragma_table_info('games') WHERE name='app_id'").Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		_, err = db.Exec("ALTER TABLE games ADD COLUMN app_id TEXT NOT NULL DEFAULT ''")
		if err != nil {
			return err
		}
	}
	err = db.QueryRow("SELECT count(*) FROM pragma_table_info('games') WHERE name='notes'").Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		_, err = db.Exec("ALTER TABLE games ADD COLUMN notes TEXT NOT NULL DEFAULT ''")
		if err != nil {
			return err
		}
	}
	err = db.QueryRow("SELECT count(*) FROM pragma_table_info('games') WHERE name='title_notes'").Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		_, err = db.Exec("ALTER TABLE games ADD COLUMN title_notes TEXT NOT NULL DEFAULT ''")
		if err != nil {
			return err
		}
	}
	err = db.QueryRow("SELECT count(*) FROM pragma_table_info('games') WHERE name='uuid'").Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		_, err = db.Exec("ALTER TABLE games ADD COLUMN uuid TEXT NOT NULL DEFAULT ''")
		if err != nil {
			return err
		}
		// Backfill UUIDs for existing rows.
		rows, err := db.Query("SELECT id FROM games WHERE uuid = ''")
		if err != nil {
			return err
		}
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		rows.Close()
		for _, id := range ids {
			_, err := db.Exec("UPDATE games SET uuid = ? WHERE id = ?", newUUID(), id)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func migrateModpacks(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS modpacks (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid          TEXT NOT NULL UNIQUE,
			game_title    TEXT NOT NULL,
			modpack_title TEXT NOT NULL,
			file_name     TEXT NOT NULL,
			file_size_b   INTEGER NOT NULL,
			notes         TEXT NOT NULL DEFAULT '',
			uploaded_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
			downloads     INTEGER NOT NULL DEFAULT 0,
			UNIQUE(game_title, modpack_title)
		)
	`)
	if err != nil {
		return err
	}
	var count int
	err = db.QueryRow("SELECT count(*) FROM pragma_table_info('modpacks') WHERE name='downloads'").Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		_, err = db.Exec("ALTER TABLE modpacks ADD COLUMN downloads INTEGER NOT NULL DEFAULT 0")
		if err != nil {
			return err
		}
	}
	err = db.QueryRow("SELECT count(*) FROM pragma_table_info('modpacks') WHERE name='notes'").Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		_, err = db.Exec("ALTER TABLE modpacks ADD COLUMN notes TEXT NOT NULL DEFAULT ''")
		if err != nil {
			return err
		}
	}
	err = db.QueryRow("SELECT count(*) FROM pragma_table_info('modpacks') WHERE name='uuid'").Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		_, err = db.Exec("ALTER TABLE modpacks ADD COLUMN uuid TEXT NOT NULL DEFAULT ''")
		if err != nil {
			return err
		}
		rows, err := db.Query("SELECT id FROM modpacks WHERE uuid = ''")
		if err != nil {
			return err
		}
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		rows.Close()
		for _, id := range ids {
			_, err := db.Exec("UPDATE modpacks SET uuid = ? WHERE id = ?", newUUID(), id)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// --- Games methods ---

// InsertGame inserts a game record. Returns the new UUID and whether it was inserted.
func (g *GamesDB) InsertGame(title, version, fileName, launchExe, appID, notes, titleNotes string, sizeB int64) (string, bool, error) {
	uuid := newUUID()
	res, err := g.db.Exec(
		`INSERT OR IGNORE INTO games (uuid, title, version, file_name, file_size_b, launch_exe, app_id, notes, title_notes) VALUES (?,?,?,?,?,?,?,?,?)`,
		uuid, title, version, fileName, sizeB, launchExe, appID, notes, titleNotes,
	)
	if err != nil {
		return "", false, fmt.Errorf("insert game: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return "", false, nil
	}
	// Sync app_id and title_notes across all versions of the same title.
	if appID != "" {
		_, _ = g.db.Exec(`UPDATE games SET app_id = ? WHERE title = ? AND app_id != ?`, appID, title, appID)
	}
	if titleNotes != "" {
		_, _ = g.db.Exec(`UPDATE games SET title_notes = ? WHERE title = ? AND title_notes != ?`, titleNotes, title, titleNotes)
	}
	return uuid, true, nil
}

// GetGame looks up a game by title and version.
func (g *GamesDB) GetGame(title, version string) (*Game, error) {
	row := g.db.QueryRow(
		`SELECT id, uuid, title, version, file_name, file_size_b, launch_exe, app_id, notes, title_notes, uploaded_at, downloads FROM games WHERE title=? AND version=?`,
		title, version,
	)
	var game Game
	err := row.Scan(&game.ID, &game.UUID, &game.Title, &game.Version, &game.FileName, &game.FileSizeB, &game.LaunchExe, &game.AppID, &game.Notes, &game.TitleNotes, &game.UploadedAt, &game.Downloads)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &game, nil
}

// GetGameByUUID looks up a game by its UUID.
func (g *GamesDB) GetGameByUUID(uuid string) (*Game, error) {
	row := g.db.QueryRow(
		`SELECT id, uuid, title, version, file_name, file_size_b, launch_exe, app_id, notes, title_notes, uploaded_at, downloads FROM games WHERE uuid=?`,
		uuid,
	)
	var game Game
	err := row.Scan(&game.ID, &game.UUID, &game.Title, &game.Version, &game.FileName, &game.FileSizeB, &game.LaunchExe, &game.AppID, &game.Notes, &game.TitleNotes, &game.UploadedAt, &game.Downloads)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &game, nil
}

// DeleteGameByUUID removes a game by UUID. Returns the file name and whether it was found.
func (g *GamesDB) DeleteGameByUUID(uuid string) (string, bool, error) {
	row := g.db.QueryRow(`SELECT file_name FROM games WHERE uuid=?`, uuid)
	var fileName string
	if err := row.Scan(&fileName); err == sql.ErrNoRows {
		return "", false, nil
	} else if err != nil {
		return "", false, err
	}
	_, err := g.db.Exec(`DELETE FROM games WHERE uuid=?`, uuid)
	return fileName, true, err
}

// UpdateGame updates fields on a game identified by UUID.
// Allowed keys: title, version, app_id, notes, title_notes, launch_exe.
// app_id and title_notes sync across all versions of the same title.
func (g *GamesDB) UpdateGame(uuid string, fields map[string]string) error {
	allowed := map[string]bool{"title": true, "version": true, "app_id": true, "notes": true, "title_notes": true, "launch_exe": true}
	for k := range fields {
		if !allowed[k] {
			return fmt.Errorf("unknown field: %s", k)
		}
	}
	// If app_id or title_notes updated, sync across all versions of same title.
	_, syncApp := fields["app_id"]
	_, syncTitleNotes := fields["title_notes"]
	if syncApp || syncTitleNotes {
		var title string
		if err := g.db.QueryRow(`SELECT title FROM games WHERE uuid = ?`, uuid).Scan(&title); err == nil {
			if syncApp && fields["app_id"] != "" {
				_, _ = g.db.Exec(`UPDATE games SET app_id = ? WHERE title = ?`, fields["app_id"], title)
			}
			if syncTitleNotes && fields["title_notes"] != "" {
				_, _ = g.db.Exec(`UPDATE games SET title_notes = ? WHERE title = ?`, fields["title_notes"], title)
			}
		}
	}
	for k, v := range fields {
		_, err := g.db.Exec(fmt.Sprintf("UPDATE games SET %s = ? WHERE uuid = ?", k), v, uuid)
		if err != nil {
			return fmt.Errorf("update %s: %w", k, err)
		}
	}
	return nil
}

// ListGames returns all game records.
func (g *GamesDB) ListGames() ([]Game, error) {
	rows, err := g.db.Query(`SELECT id, uuid, title, version, file_name, file_size_b, launch_exe, app_id, notes, title_notes, uploaded_at, downloads FROM games ORDER BY title, version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var games []Game
	for rows.Next() {
		var game Game
		if err := rows.Scan(&game.ID, &game.UUID, &game.Title, &game.Version, &game.FileName, &game.FileSizeB, &game.LaunchExe, &game.AppID, &game.Notes, &game.TitleNotes, &game.UploadedAt, &game.Downloads); err != nil {
			return nil, err
		}
		games = append(games, game)
	}
	if games == nil {
		games = []Game{}
	}
	return games, rows.Err()
}

// TotalSize returns the sum of file_size_b across all games.
func (g *GamesDB) TotalSize() (int64, error) {
	var total int64
	err := g.db.QueryRow(`SELECT COALESCE(SUM(file_size_b), 0) FROM games`).Scan(&total)
	return total, err
}

// IncrementGameDownloads increments the downloads count for a game by UUID.
func (g *GamesDB) IncrementGameDownloads(uuid string) error {
	_, err := g.db.Exec(`UPDATE games SET downloads = downloads + 1 WHERE uuid=?`, uuid)
	return err
}

// --- Modpacks methods ---

// InsertModpack inserts a modpack record. Returns the new UUID and whether it was inserted.
func (m *ModpacksDB) InsertModpack(gameTitle, modpackTitle, fileName, notes string, sizeB int64) (string, bool, error) {
	uuid := newUUID()
	res, err := m.db.Exec(
		`INSERT OR IGNORE INTO modpacks (uuid, game_title, modpack_title, file_name, file_size_b, notes) VALUES (?,?,?,?,?,?)`,
		uuid, gameTitle, modpackTitle, fileName, sizeB, notes,
	)
	if err != nil {
		return "", false, fmt.Errorf("insert modpack: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return "", false, nil
	}
	return uuid, true, nil
}

// GetModpack looks up a modpack by game title and modpack title.
func (m *ModpacksDB) GetModpack(gameTitle, modpackTitle string) (*Modpack, error) {
	row := m.db.QueryRow(
		`SELECT id, uuid, game_title, modpack_title, file_name, file_size_b, notes, uploaded_at, downloads FROM modpacks WHERE game_title=? AND modpack_title=?`,
		gameTitle, modpackTitle,
	)
	var mp Modpack
	err := row.Scan(&mp.ID, &mp.UUID, &mp.GameTitle, &mp.ModpackTitle, &mp.FileName, &mp.FileSizeB, &mp.Notes, &mp.UploadedAt, &mp.Downloads)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &mp, nil
}

// GetModpackByUUID looks up a modpack by its UUID.
func (m *ModpacksDB) GetModpackByUUID(uuid string) (*Modpack, error) {
	row := m.db.QueryRow(
		`SELECT id, uuid, game_title, modpack_title, file_name, file_size_b, notes, uploaded_at, downloads FROM modpacks WHERE uuid=?`,
		uuid,
	)
	var mp Modpack
	err := row.Scan(&mp.ID, &mp.UUID, &mp.GameTitle, &mp.ModpackTitle, &mp.FileName, &mp.FileSizeB, &mp.Notes, &mp.UploadedAt, &mp.Downloads)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &mp, nil
}

// DeleteModpackByUUID removes a modpack by UUID. Returns the file name and whether it was found.
func (m *ModpacksDB) DeleteModpackByUUID(uuid string) (string, bool, error) {
	row := m.db.QueryRow(`SELECT file_name FROM modpacks WHERE uuid=?`, uuid)
	var fileName string
	if err := row.Scan(&fileName); err == sql.ErrNoRows {
		return "", false, nil
	} else if err != nil {
		return "", false, err
	}
	_, err := m.db.Exec(`DELETE FROM modpacks WHERE uuid=?`, uuid)
	return fileName, true, err
}

// UpdateModpack updates fields on a modpack identified by UUID.
// Allowed keys: game_title, modpack_title.
func (m *ModpacksDB) UpdateModpack(uuid string, fields map[string]string) error {
	allowed := map[string]bool{"game_title": true, "modpack_title": true, "notes": true}
	for k := range fields {
		if !allowed[k] {
			return fmt.Errorf("unknown field: %s", k)
		}
	}
	for k, v := range fields {
		_, err := m.db.Exec(fmt.Sprintf("UPDATE modpacks SET %s = ? WHERE uuid = ?", k), v, uuid)
		if err != nil {
			return fmt.Errorf("update %s: %w", k, err)
		}
	}
	return nil
}

// ListModpacks returns all modpack records.
func (m *ModpacksDB) ListModpacks() ([]Modpack, error) {
	rows, err := m.db.Query(`SELECT id, uuid, game_title, modpack_title, file_name, file_size_b, notes, uploaded_at, downloads FROM modpacks ORDER BY game_title, modpack_title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var modpacks []Modpack
	for rows.Next() {
		var mp Modpack
		if err := rows.Scan(&mp.ID, &mp.UUID, &mp.GameTitle, &mp.ModpackTitle, &mp.FileName, &mp.FileSizeB, &mp.Notes, &mp.UploadedAt, &mp.Downloads); err != nil {
			return nil, err
		}
		modpacks = append(modpacks, mp)
	}
	if modpacks == nil {
		modpacks = []Modpack{}
	}
	return modpacks, rows.Err()
}

// TotalSize returns the sum of file_size_b across all modpacks.
func (m *ModpacksDB) TotalSize() (int64, error) {
	var total int64
	err := m.db.QueryRow(`SELECT COALESCE(SUM(file_size_b), 0) FROM modpacks`).Scan(&total)
	return total, err
}

// IncrementModpackDownloads increments the downloads count for a modpack by UUID.
func (m *ModpacksDB) IncrementModpackDownloads(uuid string) error {
	_, err := m.db.Exec(`UPDATE modpacks SET downloads = downloads + 1 WHERE uuid=?`, uuid)
	return err
}

// Close closes the games SQLite database connection.
func (g *GamesDB) Close() error {
	return g.db.Close()
}

// Close closes the modpacks SQLite database connection.
func (m *ModpacksDB) Close() error {
	return m.db.Close()
}

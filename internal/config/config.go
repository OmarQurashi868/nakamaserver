package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	AdminKey       string
	DownloadKey    string
	Port           string
	MaxUploadBytes int64
	DiskQuotaBytes int64
	GamesDir       string
	ModpacksDir    string
}

// Load reads config from environment variables and returns an error if required
// values are missing.
func Load() (*Config, error) {
	adminKey := os.Getenv("ADMIN_KEY")
	if adminKey == "" {
		return nil, fmt.Errorf("ADMIN_KEY env var is required")
	}

	downloadKey := os.Getenv("DOWNLOAD_KEY")
	if downloadKey == "" {
		return nil, fmt.Errorf("DOWNLOAD_KEY env var is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	maxUploadBytes := int64(10 * 1024 * 1024 * 1024) // default 10 GB
	if raw := os.Getenv("MAX_UPLOAD_BYTES"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid MAX_UPLOAD_BYTES: %w", err)
		}
		maxUploadBytes = v
	}

	diskQuotaBytes := int64(100 * 1024 * 1024 * 1024) // default 100 GB
	if raw := os.Getenv("DISK_QUOTA_BYTES"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid DISK_QUOTA_BYTES: %w", err)
		}
		diskQuotaBytes = v
	}

	gamesDir := os.Getenv("GAMES_DIR")
	if gamesDir == "" {
		gamesDir = "/data/nakama/games"
	}

	modpacksDir := os.Getenv("MODPACKS_DIR")
	if modpacksDir == "" {
		modpacksDir = "/data/nakama/modpacks"
	}

	return &Config{
		AdminKey:       adminKey,
		DownloadKey:    downloadKey,
		Port:           port,
		MaxUploadBytes: maxUploadBytes,
		DiskQuotaBytes: diskQuotaBytes,
		GamesDir:       gamesDir,
		ModpacksDir:    modpacksDir,
	}, nil
}

// Command nakamaserver is the entry point for the Nakama distribution server.
// It wires config, stores, middleware, and HTTP handlers together.
package main

import (
	"fmt"
	"net/http"
	"os"

	"nakamaserver/internal/config"
	"nakamaserver/internal/handler"
	"nakamaserver/internal/logger"
	"nakamaserver/internal/middleware"
	"nakamaserver/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}

	gamesDB, err := store.OpenGamesDB(cfg.GamesDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "games db:", err)
		os.Exit(1)
	}

	modpacksDB, err := store.OpenModpacksDB(cfg.ModpacksDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "modpacks db:", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()

	// GET /query — list all games + modpacks (admin or download key auth + rate limit)
	mux.Handle("/query",
		middleware.Logger(
			middleware.RateLimit(
				middleware.AuthEither(cfg.AdminKey, cfg.DownloadKey,
					handler.QueryHandler(gamesDB, modpacksDB),
				),
			),
		),
	)

	// POST /admin/upload/game (admin key auth + rate limit; uploads exempt from 1-per-IP)
	mux.Handle("/admin/upload/game",
		middleware.Logger(
			middleware.RateLimitAdmin(
				middleware.AuthAdmin(cfg.AdminKey,
					handler.UploadGameHandler(gamesDB, cfg.GamesDir, cfg.MaxUploadBytes),
				),
			),
		),
	)

	// POST /admin/upload/modpack
	mux.Handle("/admin/upload/modpack",
		middleware.Logger(
			middleware.RateLimitAdmin(
				middleware.AuthAdmin(cfg.AdminKey,
					handler.UploadModpackHandler(modpacksDB, cfg.ModpacksDir, cfg.MaxUploadBytes),
				),
			),
		),
	)

	// GET /download/game/{uuid} — 1 active download per IP
	mux.Handle("/download/game/",
		middleware.Logger(
			middleware.RateLimit(
				middleware.AuthEither(cfg.AdminKey, cfg.DownloadKey,
					middleware.OneDownloadPerIP(
						handler.DownloadGameHandler(gamesDB, cfg.GamesDir),
					),
				),
			),
		),
	)

	// GET /download/modpack/{uuid} — 1 active download per IP
	mux.Handle("/download/modpack/",
		middleware.Logger(
			middleware.RateLimit(
				middleware.AuthEither(cfg.AdminKey, cfg.DownloadKey,
					middleware.OneDownloadPerIP(
						handler.DownloadModpackHandler(modpacksDB, cfg.ModpacksDir),
					),
				),
			),
		),
	)

	// DELETE + PATCH /admin/game/{uuid}
	mux.Handle("/admin/game/",
		middleware.Logger(
			middleware.RateLimitAdmin(
				middleware.AuthAdmin(cfg.AdminKey,
					handler.GameAdminHandler(gamesDB, cfg.GamesDir),
				),
			),
		),
	)

	// DELETE + PATCH /admin/modpack/{uuid}
	mux.Handle("/admin/modpack/",
		middleware.Logger(
			middleware.RateLimitAdmin(
				middleware.AuthAdmin(cfg.AdminKey,
					handler.ModpackAdminHandler(modpacksDB, cfg.ModpacksDir),
				),
			),
		),
	)

	// GET /admin/disk-quota
	mux.Handle("/admin/disk-quota",
		middleware.Logger(
			middleware.RateLimitAdmin(
				middleware.AuthAdmin(cfg.AdminKey,
					handler.DiskQuotaHandler(cfg.GamesDir),
				),
			),
		),
	)

	addr := ":" + cfg.Port
	logger.Info("starting nakamaserver", map[string]any{"addr": addr})
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintln(os.Stderr, "server error:", err)
		os.Exit(1)
	}
}

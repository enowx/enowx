// Command enowx-cloud is the team cloud's server (docs/cloud.md), set up
// from the environment. It serves plain HTTP: a proxy in front ends TLS.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/enowx/enowx/cloud/blobstore"
	"github.com/enowx/enowx/cloud/server"
	"github.com/enowx/enowx/cloud/store"
	"github.com/enowx/enowx/version"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version.String)
		return
	}
	if err := run(log); err != nil {
		log.Error("enowx-cloud", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	env := func(name, def string) string {
		if v := os.Getenv(name); v != "" {
			return v
		}
		return def
	}
	cfg := server.Config{
		URL:                env("ENOWX_CLOUD_URL", ""),
		GitHubClientID:     env("ENOWX_CLOUD_GITHUB_CLIENT_ID", ""),
		GitHubClientSecret: env("ENOWX_CLOUD_GITHUB_CLIENT_SECRET", ""),
		SMTPURL:            env("ENOWX_CLOUD_SMTP_URL", ""),
		GitHubURL:          env("ENOWX_CLOUD_GITHUB_URL", ""),
		GitHubAPIURL:       env("ENOWX_CLOUD_GITHUB_API_URL", ""),
		DefaultTeam:        env("ENOWX_CLOUD_DEFAULT_TEAM", ""),
		DefaultRole:        env("ENOWX_CLOUD_DEFAULT_ROLE", ""),
		DefaultTeamRepo:    env("ENOWX_CLOUD_DEFAULT_TEAM_REPO", ""),
	}
	cfg.Contributor = server.ContributorConfig{
		DiscordPublicKey: env("ENOWX_CLOUD_DISCORD_PUBLIC_KEY", ""),
		DiscordBotToken:  env("ENOWX_CLOUD_DISCORD_BOT_TOKEN", ""),
		DiscordGuildID:   env("ENOWX_CLOUD_DISCORD_GUILD_ID", ""),
		DiscordRoleID:    env("ENOWX_CLOUD_DISCORD_ROLE_ID", ""),
		WebhookSecret:    env("ENOWX_CLOUD_GITHUB_WEBHOOK_SECRET", ""),
		LinkSecret:       env("ENOWX_CLOUD_CONTRIBUTOR_LINK_SECRET", ""),
		Repo:             env("ENOWX_CLOUD_CONTRIBUTOR_REPO", ""),
	}
	addr, data := env("ENOWX_CLOUD_ADDR", ":8080"), env("ENOWX_CLOUD_DATA", "data")
	if err := os.MkdirAll(data, 0o700); err != nil {
		return err
	}
	// Teams' files: in an R2 bucket when one is set, else in the data
	// folder.
	if bucket := env("ENOWX_CLOUD_R2_BUCKET", ""); bucket != "" {
		cfg.Blobs = blobstore.R2(env("ENOWX_CLOUD_R2_ACCOUNT_ID", ""), bucket, env("ENOWX_CLOUD_R2_ACCESS_KEY_ID", ""), env("ENOWX_CLOUD_R2_SECRET_ACCESS_KEY", ""))
	} else {
		cfg.Blobs = blobstore.Dir(filepath.Join(data, "blobs"))
	}
	// enowX UI: off unless asked for. It shares the store above, teams' files
	// untouched, under a prefix of its own, and over a bucket only on Postgres.
	if env("ENOWX_CLOUD_LIBRARY", "") == "on" {
		if os.Getenv("ENOWX_CLOUD_R2_BUCKET") != "" && os.Getenv("ENOWX_CLOUD_DATABASE_URL") == "" {
			return errors.New("ENOWX_CLOUD_LIBRARY needs ENOWX_CLOUD_DATABASE_URL: the library runs on Postgres")
		}
		cfg.Library = &server.LibraryConfig{Blobs: cfg.Blobs, Prefix: env("ENOWX_CLOUD_LIBRARY_PREFIX", ""), PublicURL: env("ENOWX_CLOUD_LIBRARY_PUBLIC_URL", ""),
			ReturnURL: env("ENOWX_CLOUD_LIBRARY_RETURN_URL", ""), WebSecret: env("ENOWX_CLOUD_LIBRARY_WEB_SECRET", ""), AdminTeam: env("ENOWX_CLOUD_LIBRARY_ADMIN_TEAM", "")}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	conns, err := strconv.Atoi(env("ENOWX_CLOUD_DATABASE_MAX_CONNS", "0"))
	if err != nil {
		return fmt.Errorf("ENOWX_CLOUD_DATABASE_MAX_CONNS is a number: %w", err)
	}
	db, err := store.Open(ctx, store.Config{Dir: data, Postgres: os.Getenv("ENOWX_CLOUD_DATABASE_URL"), Schema: os.Getenv("ENOWX_CLOUD_DATABASE_SCHEMA"), MaxConns: conns})
	if err != nil {
		return err
	}
	defer db.Close()
	if cfg.URL == "" {
		log.Warn("ENOWX_CLOUD_URL is unset: GitHub sign-in and invite links need it")
	}
	handler := server.New(cfg, db, log)
	go handler.RunLibrary(ctx)
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	served := make(chan error, 1)
	go func() { served <- srv.ListenAndServe() }()
	database := "sqlite in " + data
	if os.Getenv("ENOWX_CLOUD_DATABASE_URL") != "" {
		database = "postgres"
	}
	log.Info("enowx-cloud serving", "addr", addr, "version", version.String, "database", database)
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	// Requests under way finish; new ones are refused.
	shut, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shut); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

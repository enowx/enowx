// Command contributor-bot gives the Contributor role on a Discord server to a member
// whose linked GitHub account has a merged pull request in one repository. It is a
// small service of its own: nothing in the enowX app ships or imports it.
//
// It takes Discord's slash commands over HTTP (an interactions endpoint), so it holds no
// connection open, and keeps only pairs of IDs. See README.md.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// config is what the bot runs with, all of it from the environment.
type config struct {
	Port          string
	BaseURL       string // where this service is reached, no trailing slash
	AppID         string
	PublicKey     string // hex, Discord's to sign interactions with
	BotToken      string
	GuildID       string
	RoleID        string
	ClientID      string
	ClientSecret  string
	WebhookSecret string
	StateSecret   string
	Repo          string // owner/name: only a merged pull request here counts
	LinksFile     string
}

// readConfig reads the environment through get, naming all that's missing at once.
func readConfig(get func(string) string) (config, error) {
	var missing []string
	need := func(name string) string {
		v := strings.TrimSpace(get(name))
		if v == "" {
			missing = append(missing, name)
		}
		return v
	}
	c := config{
		BaseURL:       strings.TrimRight(need("BASE_URL"), "/"),
		AppID:         need("DISCORD_APP_ID"),
		PublicKey:     need("DISCORD_PUBLIC_KEY"),
		BotToken:      need("DISCORD_BOT_TOKEN"),
		GuildID:       need("DISCORD_GUILD_ID"),
		RoleID:        need("DISCORD_ROLE_ID"),
		ClientID:      need("GITHUB_CLIENT_ID"),
		ClientSecret:  need("GITHUB_CLIENT_SECRET"),
		WebhookSecret: need("GITHUB_WEBHOOK_SECRET"),
		StateSecret:   need("STATE_SECRET"),
		Repo:          need("GITHUB_REPO"),
		LinksFile:     need("LINKS_FILE"),
		Port:          get("PORT"),
	}
	if len(missing) > 0 {
		return c, fmt.Errorf("not set: %s", strings.Join(missing, ", "))
	}
	if c.Port == "" {
		c.Port = "8080"
	}
	return c, nil
}

func main() {
	if err := run(); err != nil {
		slog.Error("contributor-bot", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := readConfig(os.Getenv)
	if err != nil {
		return err
	}
	discord := newDiscord(cfg)
	if len(os.Args) > 1 {
		if os.Args[1] != "register" {
			return errors.New("usage: contributor-bot [register]")
		}
		// Once, by hand, and again if the commands change.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return discord.registerCommands(ctx)
	}
	links, err := openLinks(cfg.LinksFile)
	if err != nil {
		return err
	}
	b, err := newBot(cfg, links, discord, newGitHub(cfg))
	if err != nil {
		return err
	}
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: b.routes(), ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		done, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(done)
	}()
	slog.Info("contributor-bot listening", "port", cfg.Port, "repo", cfg.Repo)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

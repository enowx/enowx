package store

import (
	"context"
	"database/sql"
	"errors"
)

// ErrGitHubLinked is a GitHub account linked to another Discord account.
var ErrGitHubLinked = errors.New("store: that GitHub account is linked to another Discord account")

// LinkDiscord links Discord account discordID to GitHub account githubID, in place of
// any account it linked before.
func (s *DB) LinkDiscord(ctx context.Context, discordID string, githubID int64) error {
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM discord_links WHERE discord_id = $1`, discordID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO discord_links (discord_id, github_id) VALUES ($1, $2)`, discordID, githubID)
	if isUnique(err) {
		return ErrGitHubLinked
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// UnlinkDiscord removes discordID's link, if it has one.
func (s *DB) UnlinkDiscord(ctx context.Context, discordID string) error {
	_, err := s.ExecContext(ctx, `DELETE FROM discord_links WHERE discord_id = $1`, discordID)
	return err
}

// DiscordLinked says discordID has a link.
func (s *DB) DiscordLinked(ctx context.Context, discordID string) (bool, error) {
	var id int64
	err := s.QueryRowContext(ctx, `SELECT github_id FROM discord_links WHERE discord_id = $1`, discordID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// DiscordOfGitHub is the Discord account githubID is linked to, "" for none.
func (s *DB) DiscordOfGitHub(ctx context.Context, githubID int64) (string, error) {
	var id string
	err := s.QueryRowContext(ctx, `SELECT discord_id FROM discord_links WHERE github_id = $1`, githubID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

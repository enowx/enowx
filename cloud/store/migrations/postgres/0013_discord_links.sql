-- Who linked what, for the Contributor role on Discord: a Discord account's ID and
-- a GitHub account's, and nothing else. One GitHub account links to one Discord
-- account. They belong to neither table of accounts: linking makes no account here.
CREATE TABLE discord_links (
	discord_id TEXT PRIMARY KEY,
	github_id  BIGINT NOT NULL UNIQUE
);

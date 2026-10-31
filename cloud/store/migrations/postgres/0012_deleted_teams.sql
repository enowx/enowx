-- A deleted team's former members, for a while (store.deletedTeamsKept):
-- asking for the team, they are told it was deleted rather than that it
-- isn't there, so their hosts say why its space went. The team's row and
-- all it held are gone.
CREATE TABLE deleted_teams (
	team_id    TEXT NOT NULL,
	account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
	deleted    TEXT NOT NULL,
	PRIMARY KEY (team_id, account_id)
);
CREATE INDEX deleted_teams_deleted ON deleted_teams (deleted);

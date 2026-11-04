-- enowX UI's community: comments on designs, what happens to a creator, and how
-- many looked. Comment text is other people's data: plain text, never HTML. A
-- comment removed by a moderator keeps its text for them, and no answer but a
-- report's holds it; one its author deleted keeps none.
CREATE TABLE library_comments (
	id        TEXT PRIMARY KEY,
	item_id   TEXT NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
	author_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
	body      TEXT NOT NULL,
	status    TEXT NOT NULL DEFAULT 'live',
	created   TEXT NOT NULL
);
CREATE INDEX library_comments_item ON library_comments (item_id, created);
CREATE INDEX library_comments_author ON library_comments (author_id, created);

ALTER TABLE library_items ADD COLUMN comments_off INTEGER NOT NULL DEFAULT 0;

-- A report may be on a comment, and a copyright report says more.
ALTER TABLE library_reports ADD COLUMN comment_id TEXT NOT NULL DEFAULT '';
ALTER TABLE library_reports ADD COLUMN copyright TEXT NOT NULL DEFAULT '';

-- actor_id is who did it, for a comment; a moderator's decision names none.
CREATE TABLE library_notifications (
	id         TEXT PRIMARY KEY,
	account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
	kind       TEXT NOT NULL,
	item_id    TEXT NOT NULL DEFAULT '',
	actor_id   TEXT NOT NULL DEFAULT '',
	reason     TEXT NOT NULL DEFAULT '',
	read_at    TEXT NOT NULL DEFAULT '',
	created    TEXT NOT NULL
);
CREATE INDEX library_notifications_account ON library_notifications (account_id, created);

-- off is a JSON list of kinds turned off. Email is off until asked for.
CREATE TABLE library_notify_settings (
	account_id TEXT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
	off        TEXT NOT NULL DEFAULT '[]',
	email      INTEGER NOT NULL DEFAULT 0
);

-- Totals, a row an item, apart from library_items so that counting never holds
-- its row. Nothing here says who: no address, no account, no cookie.
CREATE TABLE library_stats (
	item_id TEXT PRIMARY KEY REFERENCES library_items(id) ON DELETE CASCADE,
	views   BIGINT NOT NULL DEFAULT 0,
	reuses  BIGINT NOT NULL DEFAULT 0
);

ALTER TABLE library_versions ADD COLUMN video_size BIGINT NOT NULL DEFAULT 0;

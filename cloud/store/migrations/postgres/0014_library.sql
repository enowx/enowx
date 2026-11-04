-- enowX UI, the community library of canvas designs (docs/cloud.md). A
-- profile is made as an account first uses it; what the library keeps of a
-- design's content is in its bucket, this says whose it is and whether it
-- shows. library_log only ever grows: the store has no way to change a row.
CREATE TABLE library_profiles (
	account_id    TEXT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
	handle        TEXT NOT NULL UNIQUE,
	bio           TEXT NOT NULL DEFAULT '',
	links         TEXT NOT NULL DEFAULT '[]',
	hidden        INTEGER NOT NULL DEFAULT 0,
	open_for_work INTEGER NOT NULL DEFAULT 0,
	work_kinds    TEXT NOT NULL DEFAULT '[]',
	skills        TEXT NOT NULL DEFAULT '[]',
	timezone      TEXT NOT NULL DEFAULT '',
	contact_url   TEXT NOT NULL DEFAULT '',
	trust         TEXT NOT NULL DEFAULT 'new',
	used_bytes    BIGINT NOT NULL DEFAULT 0,
	created       TEXT NOT NULL
);

CREATE TABLE library_items (
	id            TEXT PRIMARY KEY,
	owner_id      TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
	slug          TEXT NOT NULL,
	kind          TEXT NOT NULL,
	license       TEXT NOT NULL,
	title         TEXT NOT NULL,
	description   TEXT NOT NULL DEFAULT '',
	category      TEXT NOT NULL,
	style         TEXT NOT NULL DEFAULT '',
	theme         TEXT NOT NULL DEFAULT '',
	device        TEXT NOT NULL DEFAULT '',
	has_motion    INTEGER NOT NULL DEFAULT 0,
	made_with     TEXT NOT NULL DEFAULT 'hand',
	source_url    TEXT NOT NULL DEFAULT '',
	status        TEXT NOT NULL,
	live_version  INTEGER NOT NULL DEFAULT 0,
	reject_reason TEXT NOT NULL DEFAULT '',
	saves         INTEGER NOT NULL DEFAULT 0,
	featured_at   TEXT NOT NULL DEFAULT '',
	announced_at  TEXT NOT NULL DEFAULT '',
	purge_at      TEXT NOT NULL DEFAULT '',
	created       TEXT NOT NULL,
	updated       TEXT NOT NULL
);
CREATE INDEX library_items_browse ON library_items (status, created);
CREATE INDEX library_items_owner ON library_items (owner_id);

CREATE TABLE library_versions (
	item_id            TEXT NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
	version            INTEGER NOT NULL,
	status             TEXT NOT NULL,
	kind               TEXT NOT NULL,
	license            TEXT NOT NULL,
	source_size        BIGINT NOT NULL DEFAULT 0,
	doc_size           BIGINT NOT NULL DEFAULT 0,
	doc_phone_size     BIGINT NOT NULL DEFAULT 0,
	thumb_size         BIGINT NOT NULL DEFAULT 0,
	thumb_phone_size   BIGINT NOT NULL DEFAULT 0,
	width              INTEGER NOT NULL DEFAULT 0,
	height             INTEGER NOT NULL DEFAULT 0,
	format             TEXT NOT NULL,
	prompt_text        TEXT NOT NULL DEFAULT '',
	prompt_approved_at TEXT NOT NULL DEFAULT '',
	rights_at          TEXT NOT NULL,
	scan               TEXT NOT NULL DEFAULT '[]',
	meta               TEXT NOT NULL DEFAULT '{}',
	created            TEXT NOT NULL,
	PRIMARY KEY (item_id, version)
);

CREATE TABLE library_tags (
	item_id TEXT NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
	tag     TEXT NOT NULL,
	PRIMARY KEY (item_id, tag)
);
CREATE INDEX library_tags_tag ON library_tags (tag);

CREATE TABLE library_saves (
	account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
	item_id    TEXT NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
	created    TEXT NOT NULL,
	PRIMARY KEY (account_id, item_id)
);

-- A report's reporter is empty when the library itself raised it (a preview
-- that tried to leave its frame).
CREATE TABLE library_reports (
	id          TEXT PRIMARY KEY,
	item_id     TEXT NOT NULL DEFAULT '',
	account_id  TEXT NOT NULL DEFAULT '',
	reporter_id TEXT NOT NULL DEFAULT '',
	reason      TEXT NOT NULL,
	detail      TEXT NOT NULL DEFAULT '',
	status      TEXT NOT NULL DEFAULT 'open',
	decision_id BIGINT NOT NULL DEFAULT 0,
	created     TEXT NOT NULL
);
CREATE INDEX library_reports_open ON library_reports (status, created);

CREATE TABLE library_log (
	id         BIGSERIAL PRIMARY KEY,
	at         TEXT NOT NULL,
	actor_id   TEXT NOT NULL,
	action     TEXT NOT NULL,
	item_id    TEXT NOT NULL DEFAULT '',
	account_id TEXT NOT NULL DEFAULT '',
	version    INTEGER NOT NULL DEFAULT 0,
	reason     TEXT NOT NULL
);

CREATE TABLE library_moderators (
	account_id   TEXT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
	appointed_by TEXT NOT NULL,
	created      TEXT NOT NULL
);

-- Web sessions and sign-in codes, by their hash. They live apart from
-- devices' tokens so that one opens the library's API and nothing else.
CREATE TABLE library_sessions (
	hash       TEXT PRIMARY KEY,
	account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
	created    TEXT NOT NULL,
	expires    TEXT NOT NULL
);

CREATE TABLE library_auth_codes (
	hash       TEXT PRIMARY KEY,
	account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
	challenge  TEXT NOT NULL,
	expires    TEXT NOT NULL
);

CREATE TABLE library_usage (
	month         TEXT PRIMARY KEY,
	bytes_written BIGINT NOT NULL DEFAULT 0,
	bytes_stored  BIGINT NOT NULL DEFAULT 0,
	write_ops     BIGINT NOT NULL DEFAULT 0
);

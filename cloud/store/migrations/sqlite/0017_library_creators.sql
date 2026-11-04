-- enowX UI's creators: what a profile shows, who follows whom, collections, and
-- who has a real claim to the contributor badge. A follow and a private collection
-- are their owner's alone: the API never answers them to anyone else.
ALTER TABLE library_profiles ADD COLUMN show_github INTEGER NOT NULL DEFAULT 0;
ALTER TABLE library_profiles ADD COLUMN show_stats INTEGER NOT NULL DEFAULT 1;
ALTER TABLE library_profiles ADD COLUMN handle_changed TEXT NOT NULL DEFAULT '';

CREATE INDEX library_saves_item ON library_saves (item_id);

CREATE TABLE library_follows (
	follower_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
	creator_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
	created     TEXT NOT NULL,
	PRIMARY KEY (follower_id, creator_id)
);
CREATE INDEX library_follows_creator ON library_follows (creator_id);

CREATE TABLE library_collections (
	id          TEXT PRIMARY KEY,
	owner_id    TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
	title       TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	public      INTEGER NOT NULL DEFAULT 0,
	created     TEXT NOT NULL,
	updated     TEXT NOT NULL
);
CREATE INDEX library_collections_owner ON library_collections (owner_id);

CREATE TABLE library_collection_items (
	collection_id TEXT NOT NULL REFERENCES library_collections(id) ON DELETE CASCADE,
	item_id       TEXT NOT NULL REFERENCES library_items(id) ON DELETE CASCADE,
	created       TEXT NOT NULL,
	PRIMARY KEY (collection_id, item_id)
);
CREATE INDEX library_collection_items_item ON library_collection_items (item_id);

-- GitHub accounts the Cloud found to have contributed: a merged pull request (signed
-- webhook or the role's sign-in) or GitHub's list of the default team's repository's
-- contributors. By GitHub's ID, public data, as a login can change hands; no account
-- is made for one.
CREATE TABLE github_contributors (
	github_id BIGINT PRIMARY KEY,
	source    TEXT NOT NULL,
	since     TEXT NOT NULL
);

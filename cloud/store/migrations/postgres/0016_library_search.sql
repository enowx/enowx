-- enowX UI's search. Words: a weighted tsvector the database keeps itself, a
-- GIN index on it; tags are copied to the item, as a generated column can't
-- read another table. What the server reads of a version at submit: its dominant
-- colors, and the perceptual hash of its picture moderators see near matches by.
ALTER TABLE library_items ADD COLUMN tags_text TEXT NOT NULL DEFAULT '';
UPDATE library_items SET tags_text = COALESCE((SELECT string_agg(tag, ' ' ORDER BY tag) FROM library_tags WHERE item_id = library_items.id), '');
ALTER TABLE library_items ADD COLUMN search tsvector GENERATED ALWAYS AS (
	setweight(to_tsvector('english'::regconfig, title), 'A') ||
	setweight(to_tsvector('english'::regconfig, tags_text), 'B') ||
	setweight(to_tsvector('english'::regconfig, description), 'C')
) STORED;
CREATE INDEX library_items_search ON library_items USING gin (search);

-- What an item is made for comes from its size from now on.
UPDATE library_items SET device = CASE WHEN COALESCE((SELECT width FROM library_versions v WHERE v.item_id = library_items.id AND v.version = library_items.live_version), 1000) <= 600 THEN 'phone' ELSE 'desktop' END WHERE device = '';

ALTER TABLE library_versions ADD COLUMN phash TEXT NOT NULL DEFAULT '';

CREATE TABLE library_version_colors (
	item_id TEXT NOT NULL,
	version INTEGER NOT NULL,
	color   TEXT NOT NULL,
	share   INTEGER NOT NULL,
	PRIMARY KEY (item_id, version, color),
	FOREIGN KEY (item_id, version) REFERENCES library_versions (item_id, version) ON DELETE CASCADE
);
CREATE INDEX library_version_colors_color ON library_version_colors (color, item_id);

-- Meaning search, off unless an embedder is set. vec is the vector's bytes
-- whatever the database; the pgvector column is added by the server when it
-- finds that extension, as a cloud must start without it.
CREATE TABLE library_embeddings (
	item_id TEXT PRIMARY KEY REFERENCES library_items(id) ON DELETE CASCADE,
	model   TEXT NOT NULL,
	vec     BYTEA NOT NULL,
	created TEXT NOT NULL
);

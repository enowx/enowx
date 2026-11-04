-- enowX UI's search. SQLite stays with LIKE, tags copied to the item so it reads
-- them with the rest. What the server reads of a version at submit: its dominant
-- colors, and the perceptual hash of its picture moderators see near matches by.
ALTER TABLE library_items ADD COLUMN tags_text TEXT NOT NULL DEFAULT '';
UPDATE library_items SET tags_text = COALESCE((SELECT group_concat(tag, ' ') FROM library_tags WHERE item_id = library_items.id), '');

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

-- Meaning search, off unless an embedder is set: here the vectors are compared
-- by the server, for tests and small clouds.
CREATE TABLE library_embeddings (
	item_id TEXT PRIMARY KEY REFERENCES library_items(id) ON DELETE CASCADE,
	model   TEXT NOT NULL,
	vec     BLOB NOT NULL,
	created TEXT NOT NULL
);

package store

import (
	"context"
	"fmt"
	"strconv"
)

// LibraryHash is a version's picture's hash, and what a moderator is told of it.
type LibraryHash struct {
	ItemID, Owner, Handle, Slug, Title, Status, Phash string
	Version                                           int
}

// libraryHashesMax bounds what one look at the queue reads: far more versions
// than the library's limits leave room for soon, the newest first.
const libraryHashesMax = 20000

// LibraryHashes are the pictures' hashes of versions that were or are about to
// be public, taken-down ones too: a design put back up is what moderators want to
// see. Only moderators' queue reads this.
func (s *DB) LibraryHashes(ctx context.Context) ([]LibraryHash, error) {
	rows, err := s.QueryContext(ctx, `SELECT v.item_id, i.owner_id, COALESCE(p.handle, ''), i.slug, i.title, i.status, v.phash, v.version
		FROM library_versions v JOIN library_items i ON i.id = v.item_id LEFT JOIN library_profiles p ON p.account_id = i.owner_id
		WHERE v.phash <> '' AND v.status <> 'uploading' ORDER BY v.created DESC LIMIT `+strconv.Itoa(libraryHashesMax))
	if err != nil {
		return nil, fmt.Errorf("store: library hashes: %w", err)
	}
	defer rows.Close()
	out := []LibraryHash{}
	for rows.Next() {
		var h LibraryHash
		if err := rows.Scan(&h.ItemID, &h.Owner, &h.Handle, &h.Slug, &h.Title, &h.Status, &h.Phash, &h.Version); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

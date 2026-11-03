package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"

	"github.com/enowx/enowx/cloud/api"
)

// maxTeamItems is the most items a team keeps, deleted ones aside.
const maxTeamItems = 100_000

// ConflictError is a write of an item whose version was taken already:
// Item is the item as it is.
type ConflictError struct{ Item api.Item }

func (e *ConflictError) Error() string {
	return fmt.Sprintf("store: item %s is at version %d already", e.Item.ID, e.Item.Version)
}

// A page holds 200 items at most, and stops once past itemPageBytes of
// them (one item always fits).
const (
	itemPage      = 200
	itemPageBytes = 8 << 20
)

// itemIDPattern is an item's ID, chosen by the devices: opaque to the
// cloud.
var itemIDPattern = regexp.MustCompile(`^[0-9a-f]{16,64}$`)

var itemKinds = map[string]bool{api.ItemNote: true, api.ItemCanvas: true, api.ItemFile: true, api.ItemChat: true}

// Items are team teamID's items changed after seq, for member accID.
func (s *DB) Items(ctx context.Context, teamID, accID string, after int64) (api.Items, error) {
	if _, err := role(ctx, s, teamID, accID); err != nil {
		return api.Items{}, err
	}
	out := api.Items{Items: []api.Item{}, Seq: after}
	last, more, err := s.itemPageEnd(ctx, teamID, after)
	if err != nil || last == after {
		return out, err
	}
	out.More = more
	if s.itemReads != nil {
		select {
		case s.itemReads <- struct{}{}:
			defer func() { <-s.itemReads }()
		case <-ctx.Done():
			return out, ctx.Err()
		}
	}
	rows, err := s.QueryContext(ctx, `SELECT id, kind, version, epoch, sealed, deleted, seq, updated, updated_by FROM items
		WHERE team_id = $1 AND seq > $2 AND seq <= $3 ORDER BY seq`, teamID, after, last)
	if err != nil {
		return api.Items{}, err
	}
	defer rows.Close()
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return api.Items{}, err
		}
		out.Items = append(out.Items, it)
		out.Seq = it.Seq
	}
	if err := rows.Err(); err != nil {
		return api.Items{}, err
	}
	// Seq stays at the last item read, never the team's latest: an item
	// committed between this read and another would be skipped for good.
	return out, nil
}

// itemPageEnd is the seq of the last item of the page after seq after, and
// whether more follow, from the items' sizes: the bodies of a page's 200 items
// can be gigabytes, and those read only to be dropped hold a connection all the
// same while they cross the network.
func (s *DB) itemPageEnd(ctx context.Context, teamID string, after int64) (last int64, more bool, err error) {
	rows, err := s.QueryContext(ctx, `SELECT seq, COALESCE(length(sealed), 0) FROM items
		WHERE team_id = $1 AND seq > $2 ORDER BY seq LIMIT $3`, teamID, after, itemPage+1)
	if err != nil {
		return after, false, err
	}
	defer rows.Close()
	last = after
	count, size := 0, 0
	for rows.Next() {
		var seq int64
		var n int
		if err := rows.Scan(&seq, &n); err != nil {
			return after, false, err
		}
		if count == itemPage || count > 0 && size+n > itemPageBytes {
			return last, true, nil
		}
		last, count, size = seq, count+1, size+n
	}
	return last, false, rows.Err()
}

func scanItem(r interface{ Scan(...any) error }) (api.Item, error) {
	var it api.Item
	var deleted int
	var updated string
	if err := r.Scan(&it.ID, &it.Kind, &it.Version, &it.Epoch, &it.Sealed, &deleted, &it.Seq, &updated, &it.UpdatedBy); err != nil {
		return api.Item{}, err
	}
	it.Deleted, it.Updated = deleted != 0, parseStamp(updated)
	return it, nil
}

// PutItem writes item id of team teamID from device deviceID of member
// accID (a member or above; viewers read only): its version p.Version,
// one after the one kept, sealed with the team's current key.
func (s *DB) PutItem(ctx context.Context, teamID, accID, deviceID, id string, p api.PutItemParams) (api.Item, error) {
	switch {
	case !itemIDPattern.MatchString(id):
		return api.Item{}, fmt.Errorf("store: no item %q", id)
	case !itemKinds[p.Kind]:
		return api.Item{}, fmt.Errorf("store: no kind of item %q", p.Kind)
	case p.Version < 1:
		return api.Item{}, errors.New("store: an item's version starts at 1")
	case !p.Deleted && len(p.Sealed) == 0:
		return api.Item{}, errors.New("store: an item needs its content")
	case p.Deleted && len(p.Sealed) > api.MaxTombstone:
		return api.Item{}, fmt.Errorf("store: a deleted item keeps %d bytes at most, its sealed mark", api.MaxTombstone)
	case len(p.Sealed) > api.MaxItemSize:
		return api.Item{}, fmt.Errorf("store: an item is %d MB at most", api.MaxItemSize>>20)
	}

	var out api.Item
	err := s.asRole(ctx, teamID, accID, api.RoleMember, func(tx *sql.Tx, _ string) error {
		// The team's row first, locked for the transaction: writes to its
		// items take their turn, so two of the same version can't both
		// pass the check below.
		var epoch int
		var seq int64
		if err := tx.QueryRowContext(ctx, `UPDATE teams SET item_seq = item_seq + 1 WHERE id = $1 RETURNING item_seq, epoch`, teamID).Scan(&seq, &epoch); err != nil {
			return err
		}
		if p.Epoch != epoch {
			return fmt.Errorf("store: seal it with the team's key of epoch %d, not %d", epoch, p.Epoch)
		}
		cur, err := scanItem(tx.QueryRowContext(ctx, `SELECT id, kind, version, epoch, sealed, deleted, seq, updated, updated_by FROM items WHERE team_id = $1 AND id = $2`, teamID, id))
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if p.Version != 1 {
				return &ConflictError{Item: api.Item{ID: id}}
			}
			var n int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM items WHERE team_id = $1 AND deleted = 0`, teamID).Scan(&n); err != nil {
				return err
			}
			if n >= maxTeamItems {
				return fmt.Errorf("store: a team keeps %d items at most", maxTeamItems)
			}
		case err != nil:
			return err
		case cur.Version != p.Version-1:
			return &ConflictError{Item: cur}
		}
		out = api.Item{ID: id, Kind: p.Kind, Version: p.Version, Epoch: p.Epoch, Sealed: p.Sealed, Deleted: p.Deleted, Seq: seq, Updated: now(), UpdatedBy: deviceID}
		deleted := 0
		if p.Deleted {
			deleted = 1
		}
		// And the version checked again as it is written.
		res, err := tx.ExecContext(ctx, `INSERT INTO items (team_id, id, kind, version, epoch, sealed, deleted, updated, updated_by, seq) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (team_id, id) DO UPDATE SET kind = $3, version = $4, epoch = $5, sealed = $6, deleted = $7, updated = $8, updated_by = $9, seq = $10
			WHERE items.version = $4 - 1`,
			teamID, id, p.Kind, p.Version, p.Epoch, p.Sealed, deleted, stamp(out.Updated), deviceID, seq)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return &ConflictError{Item: api.Item{ID: id}}
		}
		return nil
	})
	return out, err
}

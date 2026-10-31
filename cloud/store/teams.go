package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/enowx/enowx/cloud/api"
)

// ErrForbidden is the error for what the asker's role doesn't allow.
var ErrForbidden = errors.New("store: your role in the team doesn't allow it")

// inviteLife is how long an invite works; inviteprefix marks its token.
const (
	inviteLife   = 7 * 24 * time.Hour
	invitePrefix = "enxi_"
)

var rank = map[string]int{api.RoleViewer: 1, api.RoleMember: 2, api.RoleAdmin: 3, api.RoleOwner: 4}

// role is accID's role in team teamID, ErrNotFound when not a member: a
// team is invisible to others. A former member of a team deleted is told
// so (ErrTeamDeleted).
func role(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, teamID, accID string) (string, error) {
	var r string
	err := q.QueryRowContext(ctx, `SELECT role FROM members WHERE team_id = $1 AND account_id = $2`, teamID, accID).Scan(&r)
	if errors.Is(err, sql.ErrNoRows) {
		if wasMember(ctx, q, teamID, accID) {
			return "", ErrTeamDeleted
		}
		return "", ErrNotFound
	}
	return r, err
}

// Role is accID's role in team teamID, ErrNotFound when not a member.
func (s *DB) Role(ctx context.Context, teamID, accID string) (string, error) {
	return role(ctx, s, teamID, accID)
}

// CreateTeam makes a team, accID its owner.
func (s *DB) CreateTeam(ctx context.Context, accID, name string) (api.Team, error) {
	name = cleanName(name, maxName)
	if name == "" {
		return api.Team{}, errors.New("store: a team needs a name")
	}
	t := api.Team{ID: newID(), Name: name, Epoch: 1, Created: now(), Role: api.RoleOwner}
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return api.Team{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO teams (id, name, created) VALUES ($1, $2, $3)`, t.ID, t.Name, stamp(t.Created)); err != nil {
		return api.Team{}, fmt.Errorf("store: create team: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO members (team_id, account_id, role, joined) VALUES ($1, $2, $3, $4)`, t.ID, accID, api.RoleOwner, stamp(t.Created)); err != nil {
		return api.Team{}, err
	}
	if err := audit(ctx, tx, t.ID, accID, "team.created", t.Name); err != nil {
		return api.Team{}, err
	}
	return t, tx.Commit()
}

// Teams are the teams accID is a member of.
func (s *DB) Teams(ctx context.Context, accID string) ([]api.Team, error) {
	rows, err := s.QueryContext(ctx, `SELECT t.id, t.name, t.copy_out, t.epoch, t.created, m.role
		FROM teams t JOIN members m ON m.team_id = t.id WHERE m.account_id = $1 ORDER BY t.name`, accID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []api.Team{}
	for rows.Next() {
		var t api.Team
		var created string
		var copyOut int
		if err := rows.Scan(&t.ID, &t.Name, &copyOut, &t.Epoch, &created, &t.Role); err != nil {
			return nil, err
		}
		t.Created, t.CopyOut = parseStamp(created), copyOut != 0
		out = append(out, t)
	}
	return out, rows.Err()
}

// Team is team teamID with its members, as member accID sees it.
func (s *DB) Team(ctx context.Context, teamID, accID string) (api.Team, error) {
	r, err := role(ctx, s, teamID, accID)
	if err != nil {
		return api.Team{}, err
	}
	t := api.Team{ID: teamID, Role: r}
	var created string
	var copyOut int
	if err := s.QueryRowContext(ctx, `SELECT name, copy_out, epoch, created FROM teams WHERE id = $1`, teamID).Scan(&t.Name, &copyOut, &t.Epoch, &created); err != nil {
		return api.Team{}, err
	}
	t.Created, t.CopyOut = parseStamp(created), copyOut != 0
	rows, err := s.QueryContext(ctx, `SELECT a.id, a.name, a.email, a.avatar, a.github_login, a.created, m.role, m.joined
		FROM members m JOIN accounts a ON a.id = m.account_id WHERE m.team_id = $1 ORDER BY m.joined`, teamID)
	if err != nil {
		return api.Team{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var m api.Member
		var accCreated, joined string
		if err := rows.Scan(&m.Account.ID, &m.Account.Name, &m.Account.Email, &m.Account.Avatar, &m.Account.GitHub, &accCreated, &m.Role, &joined); err != nil {
			return api.Team{}, err
		}
		if m.Account.ID != accID && rank[r] < rank[api.RoleAdmin] {
			m.Account.Email = "" // a member's address is theirs and the admins' to see
		}
		m.Account.Created, m.Joined = parseStamp(accCreated), parseStamp(joined)
		t.Members = append(t.Members, m)
	}
	return t, rows.Err()
}

// UpdateTeam changes team teamID's name or rules, for an admin.
func (s *DB) UpdateTeam(ctx context.Context, teamID, accID string, p api.TeamParams) error {
	return s.asRole(ctx, teamID, accID, api.RoleAdmin, func(tx *sql.Tx, _ string) error {
		if p.Name != nil {
			name := cleanName(*p.Name, maxName)
			if name == "" {
				return errors.New("store: a team needs a name")
			}
			if _, err := tx.ExecContext(ctx, `UPDATE teams SET name = $1 WHERE id = $2`, name, teamID); err != nil {
				return err
			}
			if err := audit(ctx, tx, teamID, accID, "team.renamed", name); err != nil {
				return err
			}
		}
		if p.CopyOut != nil {
			copyOut := 0
			if *p.CopyOut {
				copyOut = 1
			}
			if _, err := tx.ExecContext(ctx, `UPDATE teams SET copy_out = $1 WHERE id = $2`, copyOut, teamID); err != nil {
				return err
			}
			if err := audit(ctx, tx, teamID, accID, "team.copy_out", fmt.Sprint(*p.CopyOut)); err != nil {
				return err
			}
		}
		return nil
	})
}

// CreateInvite invites to team teamID with role, for an admin; email, when
// given, is whom it is for. Its token is in the invite returned only.
func (s *DB) CreateInvite(ctx context.Context, teamID, accID string, p api.InviteParams) (api.Invite, error) {
	if rank[p.Role] == 0 || p.Role == api.RoleOwner {
		return api.Invite{}, fmt.Errorf("store: invite as %s, %s or %s", api.RoleAdmin, api.RoleMember, api.RoleViewer)
	}
	var raw [24]byte
	rand.Read(raw[:])
	in := api.Invite{Role: p.Role, Email: cleanName(p.Email, 254), Created: now(), Token: invitePrefix + base64.RawURLEncoding.EncodeToString(raw[:])}
	in.Expires = in.Created.Add(inviteLife)
	in.ID = inviteID(hashToken(in.Token))
	err := s.asRole(ctx, teamID, accID, api.RoleAdmin, func(tx *sql.Tx, _ string) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO invites (hash, team_id, role, email, created_by, created, expires) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			hashToken(in.Token), teamID, in.Role, in.Email, accID, stamp(in.Created), stamp(in.Expires)); err != nil {
			return err
		}
		return audit(ctx, tx, teamID, accID, "invite.created", in.Role+" "+in.Email)
	})
	return in, err
}

// inviteID names an invite by its hash's start: enough to tell invites
// apart, nothing to accept one with.
func inviteID(hash []byte) string { return hex.EncodeToString(hash[:6]) }

// Invites are team teamID's invites still open, for an admin.
func (s *DB) Invites(ctx context.Context, teamID, accID string) ([]api.Invite, error) {
	out := []api.Invite{}
	err := s.asRole(ctx, teamID, accID, api.RoleAdmin, func(tx *sql.Tx, _ string) error {
		rows, err := tx.QueryContext(ctx, `SELECT hash, role, email, created, expires FROM invites WHERE team_id = $1 AND used IS NULL AND expires > $2 ORDER BY created`, teamID, stamp(now()))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var in api.Invite
			var hash []byte
			var created, expires string
			if err := rows.Scan(&hash, &in.Role, &in.Email, &created, &expires); err != nil {
				return err
			}
			in.ID, in.Created, in.Expires = inviteID(hash), parseStamp(created), parseStamp(expires)
			out = append(out, in)
		}
		return rows.Err()
	})
	return out, err
}

// DeleteInvite withdraws invite id of team teamID, for an admin.
func (s *DB) DeleteInvite(ctx context.Context, teamID, accID, id string) error {
	return s.asRole(ctx, teamID, accID, api.RoleAdmin, func(tx *sql.Tx, _ string) error {
		// Found by its ID, the hash's start, in Go: the same for both
		// databases.
		rows, err := tx.QueryContext(ctx, `SELECT hash FROM invites WHERE team_id = $1 AND used IS NULL`, teamID)
		if err != nil {
			return err
		}
		var hash []byte
		for rows.Next() {
			var h []byte
			if err := rows.Scan(&h); err != nil {
				rows.Close()
				return err
			}
			if inviteID(h) == strings.ToLower(id) {
				hash = h
			}
		}
		rows.Close()
		if hash == nil {
			return ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM invites WHERE hash = $1`, hash); err != nil {
			return err
		}
		return audit(ctx, tx, teamID, accID, "invite.deleted", id)
	})
}

// AcceptInvite makes accID a member of the invite's team, with its role;
// one already a member keeps theirs. It returns the team.
func (s *DB) AcceptInvite(ctx context.Context, token, accID string) (api.Team, error) {
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return api.Team{}, err
	}
	defer tx.Rollback()
	var teamID, r string
	err = tx.QueryRowContext(ctx, `SELECT team_id, role FROM invites WHERE hash = $1 AND used IS NULL AND expires > $2`, hashToken(token), stamp(now())).Scan(&teamID, &r)
	if errors.Is(err, sql.ErrNoRows) {
		return api.Team{}, ErrExpired
	}
	if err != nil {
		return api.Team{}, err
	}
	// Used once: of two accepting it at once, one finds it taken.
	res, err := tx.ExecContext(ctx, `UPDATE invites SET used = $1, used_by = $2 WHERE hash = $3 AND used IS NULL`, stamp(now()), accID, hashToken(token))
	if err != nil {
		return api.Team{}, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return api.Team{}, ErrExpired
	}
	if _, err := role(ctx, tx, teamID, accID); errors.Is(err, ErrNotFound) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO members (team_id, account_id, role, joined) VALUES ($1, $2, $3, $4)`, teamID, accID, r, stamp(now())); err != nil {
			return api.Team{}, err
		}
		if err := audit(ctx, tx, teamID, accID, "member.joined", r); err != nil {
			return api.Team{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return api.Team{}, err
	}
	return s.Team(ctx, teamID, accID)
}

// SetRole changes member target's role in team teamID, as by. Admins change
// roles below owner; the owner alone hands the team over, by making another
// member owner, and becomes an admin.
func (s *DB) SetRole(ctx context.Context, teamID, by, target, newRole string) error {
	if rank[newRole] == 0 {
		return fmt.Errorf("store: no role %q", newRole)
	}
	return s.asRole(ctx, teamID, by, api.RoleAdmin, func(tx *sql.Tx, byRole string) error {
		cur, err := role(ctx, tx, teamID, target)
		if err != nil {
			return err
		}
		switch {
		case cur == api.RoleOwner:
			return ErrForbidden // the owner hands the team over instead
		case newRole == api.RoleOwner && byRole != api.RoleOwner:
			return ErrForbidden
		case newRole == api.RoleOwner:
			if _, err := tx.ExecContext(ctx, `UPDATE members SET role = $1 WHERE team_id = $2 AND account_id = $3`, api.RoleAdmin, teamID, by); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE members SET role = $1 WHERE team_id = $2 AND account_id = $3`, newRole, teamID, target); err != nil {
			return err
		}
		return audit(ctx, tx, teamID, by, "member.role", target+" "+cur+"→"+newRole)
	})
}

// RemoveMember takes target out of team teamID: an admin removing someone
// below the owner, or a member leaving (the owner hands the team over
// first). The team's keys sealed to target's devices go, and the team's
// key moves to a new epoch they never get.
func (s *DB) RemoveMember(ctx context.Context, teamID, by, target string) error {
	need := api.RoleAdmin
	if by == target {
		need = api.RoleViewer
	}
	return s.asRole(ctx, teamID, by, need, func(tx *sql.Tx, _ string) error {
		cur, err := role(ctx, tx, teamID, target)
		if err != nil {
			return err
		}
		if cur == api.RoleOwner {
			return ErrForbidden
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM members WHERE team_id = $1 AND account_id = $2`, teamID, target); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM team_keys WHERE team_id = $1 AND device_id IN (SELECT id FROM devices WHERE account_id = $2)`, teamID, target); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE teams SET epoch = epoch + 1 WHERE id = $1`, teamID); err != nil {
			return err
		}
		action := "member.removed"
		if by == target {
			action = "member.left"
		}
		return audit(ctx, tx, teamID, by, action, target)
	})
}

// asRole runs do in a transaction when accID is a member of team teamID
// with at least role need: ErrNotFound for one not a member, ErrForbidden
// for one below it.
func (s *DB) asRole(ctx context.Context, teamID, accID, need string, do func(tx *sql.Tx, role string) error) error {
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	r, err := role(ctx, tx, teamID, accID)
	if err != nil {
		return err
	}
	if rank[r] < rank[need] {
		return ErrForbidden
	}
	if err := do(tx, r); err != nil {
		return err
	}
	return tx.Commit()
}

// PeekInvite is what invite token is for, while it is open; ErrExpired
// once used or past its time.
func (s *DB) PeekInvite(ctx context.Context, token string) (api.InvitePeek, error) {
	var p api.InvitePeek
	var expires string
	err := s.QueryRowContext(ctx, `SELECT t.name, i.role, a.name, a.github_login, i.expires FROM invites i JOIN teams t ON t.id = i.team_id JOIN accounts a ON a.id = i.created_by
		WHERE i.hash = $1 AND i.used IS NULL AND i.expires > $2`, hashToken(token), stamp(now())).Scan(&p.Team, &p.Role, &p.By, &p.ByLogin, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrExpired
	}
	p.Expires = parseStamp(expires)
	return p, err
}

// JoinDefaultTeam makes accID a member of team teamID with role, the team
// everyone joins as they sign in; not when they are one already, nor when
// they left it or were removed from it. It says whether they joined.
func (s *DB) JoinDefaultTeam(ctx context.Context, teamID, accID, r string) (bool, error) {
	if rank[r] == 0 || r == api.RoleOwner {
		return false, fmt.Errorf("store: a default team's role is %s, %s or %s", api.RoleAdmin, api.RoleMember, api.RoleViewer)
	}
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM teams WHERE id = $1`, teamID).Scan(&n); err != nil || n == 0 {
		return false, err
	}
	if _, err := role(ctx, tx, teamID, accID); !errors.Is(err, ErrNotFound) {
		return false, err // a member already
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit WHERE team_id = $1 AND action IN ('member.removed', 'member.left') AND detail = $2`, teamID, accID).Scan(&n); err != nil || n > 0 {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO members (team_id, account_id, role, joined) VALUES ($1, $2, $3, $4)`, teamID, accID, r, stamp(now())); err != nil {
		return false, err
	}
	if err := audit(ctx, tx, teamID, accID, "member.joined", r+" (default team)"); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// GitHubID is the GitHub account ID account accID signed in with, 0 for
// none.
func (s *DB) GitHubID(ctx context.Context, accID string) (int64, error) {
	var id sql.NullInt64
	err := s.QueryRowContext(ctx, `SELECT github_id FROM accounts WHERE id = $1`, accID).Scan(&id)
	return id.Int64, err
}

// RaiseDefaultViewer makes accID, a viewer of the default team teamID, a
// member: one who became a contributor of its repository. Not one an
// admin set the role of since: their choice stands.
func (s *DB) RaiseDefaultViewer(ctx context.Context, teamID, accID string) (bool, error) {
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	r, err := role(ctx, tx, teamID, accID)
	if err != nil || r != api.RoleViewer {
		return false, nil
	}
	var set int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit WHERE team_id = $1 AND action = 'member.role' AND detail LIKE $2`, teamID, accID+" %").Scan(&set); err != nil || set > 0 {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE members SET role = $1 WHERE team_id = $2 AND account_id = $3`, api.RoleMember, teamID, accID); err != nil {
		return false, err
	}
	if err := audit(ctx, tx, teamID, accID, "member.joined", api.RoleMember+" (a contributor of the default team's repository)"); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

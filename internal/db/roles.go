// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Ollornog/UnderDoc/internal/id"
)

// Roles (ADR-29), from the least to the most. Each includes those below it. RoleNone is no role at all.
const (
	RoleNone    = ""
	RoleRead    = "read"
	RoleComment = "comment"
	RoleEdit    = "edit"
	RoleManage  = "manage"
	RoleOwner   = "owner"
)

// Kinds of whom a role is granted to.
const (
	ToPerson = "person"
	ToGroup  = "group"
	ToTenant = "tenant"
)

// Sources of groups: local ones, made here, and those taken over from OIDC or the gate.
const (
	GroupLocal  = "local"
	GroupSynced = "synced"
)

// Errors of the rights.
var (
	// ErrRole is a role that does not exist, or one that cannot be granted (owner).
	ErrRole = errors.New("no such role")
	// ErrPrivate is a role or a link asked for on something private: private stays private (ADR-29).
	ErrPrivate = errors.New("private stays private")
	// ErrGroup is a group that cannot be changed here, such as one taken over from the sign-in.
	ErrGroup = errors.New("this group comes from the sign-in and can only be read")
)

// Rank orders the roles: 0 for none, then read, comment, edit, manage and owner.
func Rank(role string) int {
	switch role {
	case RoleRead:
		return 1
	case RoleComment:
		return 2
	case RoleEdit:
		return 3
	case RoleManage:
		return 4
	case RoleOwner:
		return 5
	}
	return 0
}

// roleOf is the role of a rank.
func roleOf(rank int) string {
	return [...]string{RoleNone, RoleRead, RoleComment, RoleEdit, RoleManage, RoleOwner}[max(0, min(rank, 5))]
}

// grantable tells whether a role may be granted: every role but the owner, which is the column of the object.
func grantable(role string) bool { return Rank(role) >= 1 && Rank(role) <= 4 }

// Actor is who changes rights: an account, and whether it administers the tenant. Administrators manage the rights of
// everything that is not private, but read content only where they hold a role (ADR-29).
type Actor struct {
	ID    string
	Admin bool
}

// visible is a condition on o: the viewer ($1) holds a role on it (ADR-29). The function object_role decides, the same
// for every query that lists, finds or opens objects.
const visible = `object_role(o.id, $1) > 0`

// Role returns the role of an account on an object it sees, RoleNone where it sees nothing.
func (db *DB) Role(ctx context.Context, objectID, viewer string) (string, error) {
	var rank int
	if err := db.pool.QueryRow(ctx, "SELECT object_role($1, $2)", objectID, viewer).Scan(&rank); err != nil {
		return RoleNone, err
	}
	return roleOf(rank), nil
}

// may tells whether the viewer holds at least the role on an object.
func (db *DB) may(ctx context.Context, o Object, viewer, role string) error {
	held, err := db.Role(ctx, o.ID, viewer)
	if err != nil {
		return err
	}
	if Rank(held) < Rank(role) {
		return ErrNotAllowed
	}
	return nil
}

// isPrivate tells whether an object or anything above it is private.
func isPrivate(ctx context.Context, q execer, objectID string) (bool, error) {
	var private bool
	err := q.QueryRow(ctx, `WITH RECURSIVE up AS (
			SELECT id, parent, private FROM objects WHERE id = $1
			UNION ALL SELECT p.id, p.parent, p.private FROM objects p JOIN up ON p.id = up.parent
		)
		SELECT coalesce(bool_or(private), false) FROM up`, objectID).Scan(&private)
	return private, err
}

// forRights returns an object whose rights the actor may manage: one they manage, or, for an administrator, any that is
// alive and not private. Something private, or something the actor does not see and does not administer, is not found;
// one they see but may not manage is not allowed.
func (db *DB) forRights(ctx context.Context, objectID string, actor Actor) (Object, error) {
	o, err := db.Object(ctx, objectID, actor.ID)
	if errors.Is(err, ErrNotFound) && actor.Admin {
		o, err = scan(db.pool.QueryRow(ctx, `SELECT `+columns+` FROM objects o WHERE id = $1 AND `+alive, objectID))
		if errors.Is(err, pgx.ErrNoRows) {
			err = ErrNotFound
		}
	}
	if err != nil {
		return Object{}, err
	}
	private, err := isPrivate(ctx, db.pool, o.ID)
	if err != nil {
		return Object{}, err
	}
	if private {
		if actor.Admin && o.Owner != actor.ID {
			return Object{}, ErrNotFound
		}
		return Object{}, ErrPrivate
	}
	if IsDivider(o.Kind) {
		return Object{}, ErrPlace
	}
	if actor.Admin {
		return o, nil
	}
	if err := db.may(ctx, o, actor.ID, RoleManage); err != nil {
		return Object{}, err
	}
	return o, nil
}

// Grant is a role granted on an object.
type Grant struct {
	Object string
	// Kind is ToPerson, ToGroup or ToTenant, and Who the account or the group; empty for the tenant.
	Kind, Who string
	// Name is how the interface names Who: the name of the account or of the group.
	Name string
	Role string
	By   string
	At   time.Time
}

// Rights are the roles that hold on an object: its owner, its own grants, and what it inherits from above.
type Rights struct {
	Owner, OwnerName string
	// Private tells that the object is private: its owner alone sees it, and no grant counts.
	Private bool
	// Inherit is false where the object breaks the inheritance.
	Inherit bool
	Own     []Grant
	// Inherited are the grants of the objects above, up to where the inheritance breaks, with the object each is on; the
	// owners of those objects manage and are listed as such.
	Inherited []Grant
}

const grantColumns = `g.object, g.kind, g.who, CASE g.kind WHEN 'person' THEN coalesce(a.name, g.who) WHEN 'group' THEN coalesce(gr.name, g.who)
	ELSE '' END, g.role, g.granted_by, g.granted_at`

const grantJoins = ` LEFT JOIN accounts a ON g.kind = 'person' AND a.id = g.who LEFT JOIN groups gr ON g.kind = 'group' AND gr.id = g.who`

func scanGrant(row pgx.CollectableRow) (Grant, error) {
	var g Grant
	err := row.Scan(&g.Object, &g.Kind, &g.Who, &g.Name, &g.Role, &g.By, &g.At)
	return g, err
}

// RightsOf returns the rights of an object. Whoever sees the object reads them (the facts panel names who sees it), and
// administrators read them on everything that is not private.
func (db *DB) RightsOf(ctx context.Context, objectID string, actor Actor) (Rights, error) {
	o, err := db.Object(ctx, objectID, actor.ID)
	if errors.Is(err, ErrNotFound) && actor.Admin {
		o, err = db.forRights(ctx, objectID, actor)
	}
	if err != nil {
		return Rights{}, err
	}
	r := Rights{Owner: o.Owner, OwnerName: db.AccountName(ctx, o.Owner), Inherit: !o.Breaks}
	if r.Private, err = isPrivate(ctx, db.pool, o.ID); err != nil || r.Private {
		return r, err
	}
	rows, err := db.pool.Query(ctx, `SELECT `+grantColumns+` FROM grants g`+grantJoins+` WHERE g.object = $1
		ORDER BY CASE g.kind WHEN 'tenant' THEN 0 WHEN 'group' THEN 1 ELSE 2 END, lower(coalesce(a.name, gr.name, '')), g.who`, objectID)
	if err != nil {
		return Rights{}, err
	}
	if r.Own, err = pgx.CollectRows(rows, scanGrant); err != nil {
		return Rights{}, err
	}
	if o.Breaks {
		return r, nil
	}
	for x := o; x.Parent != ""; {
		var parent Object
		parent, err = scan(db.pool.QueryRow(ctx, `SELECT `+columns+` FROM objects WHERE id = $1`, x.Parent))
		if err != nil {
			return Rights{}, err
		}
		r.Inherited = append(r.Inherited, Grant{Object: parent.ID, Kind: ToPerson, Who: parent.Owner, Name: db.AccountName(ctx, parent.Owner),
			Role: RoleManage})
		rows, err := db.pool.Query(ctx, `SELECT `+grantColumns+` FROM grants g`+grantJoins+` WHERE g.object = $1
			ORDER BY CASE g.kind WHEN 'tenant' THEN 0 WHEN 'group' THEN 1 ELSE 2 END, lower(coalesce(a.name, gr.name, '')), g.who`, parent.ID)
		if err != nil {
			return Rights{}, err
		}
		above, err := pgx.CollectRows(rows, scanGrant)
		if err != nil {
			return Rights{}, err
		}
		r.Inherited = append(r.Inherited, above...)
		if parent.Breaks {
			break
		}
		x = parent
	}
	return r, nil
}

// SetGrant grants a role on an object to a person, a group or the tenant, or takes it back with RoleNone. The actor
// must manage the object, or administer the tenant; nobody grants owner, and nothing private gets grants (ADR-29).
func (db *DB) SetGrant(ctx context.Context, objectID string, actor Actor, kind, who, role string) error {
	if role != RoleNone && !grantable(role) {
		return ErrRole
	}
	switch kind {
	case ToTenant:
		who = ""
	case ToPerson, ToGroup:
		if who == "" {
			return ErrNotFound
		}
	default:
		return ErrRole
	}
	if _, err := db.forRights(ctx, objectID, actor); err != nil {
		return err
	}
	if role == RoleNone {
		_, err := db.pool.Exec(ctx, "DELETE FROM grants WHERE object = $1 AND kind = $2 AND who = $3", objectID, kind, who)
		return err
	}
	var known bool
	var err error
	switch kind {
	case ToPerson:
		err = db.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM accounts WHERE id = $1)", who).Scan(&known)
	case ToGroup:
		err = db.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM groups WHERE id = $1)", who).Scan(&known)
	default:
		known = true
	}
	if err != nil {
		return err
	}
	if !known {
		return ErrNotFound
	}
	return setGrant(ctx, db.pool, objectID, kind, who, role, actor.ID)
}

// setGrant writes one grant, replacing what that one held before.
func setGrant(ctx context.Context, q execer, objectID, kind, who, role, by string) error {
	var done bool
	return q.QueryRow(ctx, `INSERT INTO grants (object, kind, who, role, granted_by) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (tenant, object, kind, who) DO UPDATE SET role = EXCLUDED.role, granted_by = EXCLUDED.granted_by, granted_at = now()
		RETURNING true`, objectID, kind, who, role, by).Scan(&done)
}

// SetInherit breaks the inheritance of an object, or restores it. Breaking copies what held from above onto the object,
// so nobody loses access by surprise; the manager then takes back what shall not hold (ADR-29). A collection at the top
// has nothing above it.
func (db *DB) SetInherit(ctx context.Context, objectID string, actor Actor, inherit bool) error {
	o, err := db.forRights(ctx, objectID, actor)
	if err != nil {
		return err
	}
	if o.Parent == "" {
		return ErrPlace
	}
	if o.Breaks != inherit {
		return nil
	}
	var copied []Grant
	if !inherit {
		r, err := db.RightsOf(ctx, objectID, actor)
		if err != nil {
			return err
		}
		copied = r.Inherited
	}
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		// The highest role of each one, as a copy may name someone twice.
		best := map[[2]string]string{}
		for _, g := range copied {
			key := [2]string{g.Kind, g.Who}
			if Rank(g.Role) > Rank(best[key]) {
				best[key] = g.Role
			}
		}
		own := map[[2]string]string{}
		rows, err := tx.Query(ctx, "SELECT kind, who, role FROM grants WHERE object = $1", objectID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var kind, who, role string
			if err := rows.Scan(&kind, &who, &role); err != nil {
				return err
			}
			own[[2]string{kind, who}] = role
		}
		if err := rows.Err(); err != nil {
			return err
		}
		keys := make([][2]string, 0, len(best))
		for key := range best {
			keys = append(keys, key)
		}
		slices.SortFunc(keys, func(a, b [2]string) int { return strings.Compare(a[0]+"\x00"+a[1], b[0]+"\x00"+b[1]) })
		for _, key := range keys {
			if key[0] == ToPerson && key[1] == o.Owner {
				continue
			}
			if Rank(own[key]) >= Rank(best[key]) {
				continue
			}
			if err := setGrant(ctx, tx, objectID, key[0], key[1], best[key], actor.ID); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, "UPDATE objects SET inherit = $2, changed = now() WHERE id = $1", objectID, inherit)
		return err
	})
}

// TransferOwner gives an object to another account of the tenant. Only the owner or an administrator does; the former
// owner keeps the role edit, which can be taken back like any other (ADR-29).
func (db *DB) TransferOwner(ctx context.Context, objectID string, actor Actor, to string) error {
	o, err := db.forRights(ctx, objectID, actor)
	if err != nil {
		return err
	}
	if o.Owner != actor.ID && !actor.Admin {
		return ErrNotAllowed
	}
	if to == o.Owner {
		return nil
	}
	var known bool
	if err := db.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM accounts WHERE id = $1)", to).Scan(&known); err != nil {
		return err
	}
	if !known {
		return ErrNotFound
	}
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "UPDATE objects SET owner = $2, changed = now() WHERE id = $1", objectID, to); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM grants WHERE object = $1 AND kind = 'person' AND who = $2", objectID, to); err != nil {
			return err
		}
		var held string
		err := tx.QueryRow(ctx, "SELECT role FROM grants WHERE object = $1 AND kind = 'person' AND who = $2", objectID, o.Owner).Scan(&held)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if Rank(held) >= Rank(RoleEdit) {
			return nil
		}
		return setGrant(ctx, tx, objectID, ToPerson, o.Owner, RoleEdit, actor.ID)
	})
}

// --- Groups

// Group is a group of accounts: local, or taken over from the sign-in.
type Group struct {
	ID          string
	Name        string
	Source      string
	Description string
	Members     int
	Created     time.Time
}

const groupColumns = `g.id, g.name, g.source, g.description,
	CASE g.source WHEN 'local' THEN (SELECT count(*) FROM group_members m WHERE m.grp = g.id)
		ELSE (SELECT count(*) FROM accounts a WHERE g.name = ANY (a.groups)) END, g.created`

func scanGroup(row pgx.Row) (Group, error) {
	var g Group
	err := row.Scan(&g.ID, &g.Name, &g.Source, &g.Description, &g.Members, &g.Created)
	return g, err
}

// Groups returns every group of the tenant, by name.
func (db *DB) Groups(ctx context.Context) ([]Group, error) {
	rows, err := db.pool.Query(ctx, `SELECT `+groupColumns+` FROM groups g ORDER BY lower(g.name), g.source, g.id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Group, error) { return scanGroup(row) })
}

// GroupOf returns one group.
func (db *DB) GroupOf(ctx context.Context, groupID string) (Group, error) {
	g, err := scanGroup(db.pool.QueryRow(ctx, `SELECT `+groupColumns+` FROM groups g WHERE g.id = $1`, groupID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Group{}, ErrNotFound
	}
	return g, err
}

// GroupsOf returns the IDs of the groups an account belongs to.
func (db *DB) GroupsOf(ctx context.Context, accountID string) ([]string, error) {
	var groups []string
	err := db.pool.QueryRow(ctx, "SELECT account_groups($1)", accountID).Scan(&groups)
	return nonNil(groups), err
}

// ErrGroupName is a name of a group that is empty, too long, or taken by another local group.
var ErrGroupName = errors.New("a group has a name of 1 to 200 characters that no other local group has")

// CreateGroup makes a local group. Only administrators do; the caller checks.
func (db *DB) CreateGroup(ctx context.Context, name, description string) (Group, error) {
	name, description = strings.TrimSpace(name), strings.TrimSpace(description)
	if n := len([]rune(name)); n < 1 || n > 200 || len([]rune(description)) > 1000 {
		return Group{}, ErrGroupName
	}
	for attempt := 0; ; attempt++ {
		groupID := id.New()
		_, err := db.pool.Exec(ctx, "INSERT INTO groups (id, name, source, description) VALUES ($1, $2, 'local', $3)", groupID, name, description)
		if isUnique(err, "groups_tenant_source_name_key") {
			return Group{}, ErrGroupName
		}
		if isUnique(err, "groups_pkey") && attempt < 5 {
			continue
		}
		if err != nil {
			return Group{}, err
		}
		return db.GroupOf(ctx, groupID)
	}
}

// RenameGroup gives a local group a new name and description.
func (db *DB) RenameGroup(ctx context.Context, groupID, name, description string) error {
	g, err := db.GroupOf(ctx, groupID)
	if err != nil {
		return err
	}
	if g.Source != GroupLocal {
		return ErrGroup
	}
	name, description = strings.TrimSpace(name), strings.TrimSpace(description)
	if n := len([]rune(name)); n < 1 || n > 200 || len([]rune(description)) > 1000 {
		return ErrGroupName
	}
	_, err = db.pool.Exec(ctx, "UPDATE groups SET name = $2, description = $3 WHERE id = $1", groupID, name, description)
	if isUnique(err, "groups_tenant_source_name_key") {
		return ErrGroupName
	}
	return err
}

// DeleteGroup deletes a local group with its members and every role granted to it.
func (db *DB) DeleteGroup(ctx context.Context, groupID string) error {
	g, err := db.GroupOf(ctx, groupID)
	if err != nil {
		return err
	}
	if g.Source != GroupLocal {
		return ErrGroup
	}
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "DELETE FROM grants WHERE kind = 'group' AND who = $1", groupID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "DELETE FROM groups WHERE id = $1", groupID)
		return err
	})
}

// Member is an account in a group.
type Member struct {
	ID, Login, Name string
}

// GroupMembers returns the members of a group, by name.
func (db *DB) GroupMembers(ctx context.Context, groupID string) ([]Member, error) {
	g, err := db.GroupOf(ctx, groupID)
	if err != nil {
		return nil, err
	}
	query := `SELECT a.id, a.login, a.name FROM group_members m JOIN accounts a ON a.id = m.account WHERE m.grp = $1 ORDER BY lower(a.name), a.login`
	arg := groupID
	if g.Source == GroupSynced {
		query = `SELECT a.id, a.login, a.name FROM accounts a WHERE $1 = ANY (a.groups) ORDER BY lower(a.name), a.login`
		arg = g.Name
	}
	rows, err := db.pool.Query(ctx, query, arg)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Member, error) {
		var m Member
		err := row.Scan(&m.ID, &m.Login, &m.Name)
		return m, err
	})
}

// SetMember adds an account to a local group, or takes it out.
func (db *DB) SetMember(ctx context.Context, groupID, accountID string, member bool) error {
	g, err := db.GroupOf(ctx, groupID)
	if err != nil {
		return err
	}
	if g.Source != GroupLocal {
		return ErrGroup
	}
	if !member {
		_, err := db.pool.Exec(ctx, "DELETE FROM group_members WHERE grp = $1 AND account = $2", groupID, accountID)
		return err
	}
	var known bool
	if err := db.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM accounts WHERE id = $1)", accountID).Scan(&known); err != nil {
		return err
	}
	if !known {
		return ErrNotFound
	}
	_, err = db.pool.Exec(ctx, "INSERT INTO group_members (grp, account) VALUES ($1, $2) ON CONFLICT DO NOTHING", groupID, accountID)
	return err
}

// syncGroups takes over the names of groups a sign-in brought along as groups of the tenant.
func syncGroups(ctx context.Context, tx pgx.Tx, names []string) error {
	if len(names) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `INSERT INTO groups (id, name, source)
		SELECT new_id(), n, 'synced' FROM (SELECT DISTINCT btrim(n) AS n FROM unnest($1::text[]) AS n) x
		WHERE char_length(n) BETWEEN 1 AND 200 AND NOT EXISTS (SELECT 1 FROM groups g WHERE g.source = 'synced' AND g.name = x.n)
		ON CONFLICT DO NOTHING`, names)
	return err
}

// groupNamed returns the group of a name: the local one where there is one, else the one taken over, made if it is not
// there yet.
func groupNamed(ctx context.Context, tx pgx.Tx, name string) (string, error) {
	var groupID string
	err := tx.QueryRow(ctx, `SELECT id FROM groups WHERE name = $1 ORDER BY source = 'local' DESC LIMIT 1`, name).Scan(&groupID)
	if !errors.Is(err, pgx.ErrNoRows) {
		return groupID, err
	}
	if err := syncGroups(ctx, tx, []string{name}); err != nil {
		return "", err
	}
	err = tx.QueryRow(ctx, `SELECT id FROM groups WHERE name = $1 AND source = 'synced'`, name).Scan(&groupID)
	return groupID, err
}

// isUnique tells whether an error is the breach of a unique constraint of that name.
func isUnique(err error, constraint string) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505" && pg.ConstraintName == constraint
}

// People returns every account of the tenant by name, to grant roles to: IDs and names, nothing else.
func (db *DB) People(ctx context.Context) ([]Member, error) {
	rows, err := db.pool.Query(ctx, `SELECT id, login, name FROM accounts ORDER BY lower(name), login`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Member, error) {
		var m Member
		err := row.Scan(&m.ID, &m.Login, &m.Name)
		return m, err
	})
}

// Reason is one way an account holds a role on an object: a grant to it, to one of its groups or to the tenant, a link
// it opened, or the owner of the object or of something above it.
type Reason struct {
	// Object is where the role comes from, and Title its name.
	Object, Title string
	// Via is "owner", "owner above", "person", "group", "tenant" or "link"; Name names the group where it is one.
	Via, Name string
	Role      string
}

// Explain tells the role an account holds on an object and why, for the access check of the tab Rights (ADR-29).
// Whoever manages the object asks it, and administrators; anyone may ask it for themselves.
func (db *DB) Explain(ctx context.Context, objectID string, actor Actor, accountID string) (string, []Reason, error) {
	if accountID != actor.ID {
		if _, err := db.forRights(ctx, objectID, actor); err != nil {
			return "", nil, err
		}
	} else if _, err := db.Object(ctx, objectID, actor.ID); err != nil {
		return RoleNone, nil, nil
	}
	held, err := db.Role(ctx, objectID, accountID)
	if err != nil || held == RoleNone {
		return held, nil, err
	}
	var private bool
	if private, err = isPrivate(ctx, db.pool, objectID); err != nil || private {
		return held, []Reason{{Object: objectID, Via: "owner", Role: RoleOwner}}, err
	}
	groups, err := db.GroupsOf(ctx, accountID)
	if err != nil {
		return "", nil, err
	}
	var reasons []Reason
	for cur, self := objectID, true; cur != ""; self = false {
		o, err := db.objectForRights(ctx, cur)
		if err != nil {
			return "", nil, err
		}
		if o.Owner == accountID {
			via, role := "owner above", RoleManage
			if self {
				via, role = "owner", RoleOwner
			}
			reasons = append(reasons, Reason{Object: o.ID, Title: o.Title, Via: via, Role: role})
		}
		rows, err := db.pool.Query(ctx, `SELECT g.kind, g.role, coalesce(gr.name, '') FROM grants g LEFT JOIN groups gr ON g.kind = 'group' AND gr.id = g.who
			WHERE g.object = $1 AND (g.kind = 'tenant' OR (g.kind = 'person' AND g.who = $2) OR (g.kind = 'group' AND g.who = ANY ($3)))
			ORDER BY g.kind, gr.name`, o.ID, accountID, groups)
		if err != nil {
			return "", nil, err
		}
		found, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Reason, error) {
			r := Reason{Object: o.ID, Title: o.Title}
			err := row.Scan(&r.Via, &r.Role, &r.Name)
			return r, err
		})
		if err != nil {
			return "", nil, err
		}
		reasons = append(reasons, found...)
		links, err := db.pool.Query(ctx, `SELECT l.mode FROM share_links l JOIN share_link_users u ON u.link = l.id
			WHERE l.object = $1 AND u.account = $2 AND l.mode IN ('comment', 'edit') AND (l.expires IS NULL OR l.expires > now())
			  AND object_role(l.object, l.created_by, false) >= 4`, o.ID, accountID)
		if err != nil {
			return "", nil, err
		}
		modes, err := pgx.CollectRows(links, pgx.RowTo[string])
		if err != nil {
			return "", nil, err
		}
		for _, m := range modes {
			reasons = append(reasons, Reason{Object: o.ID, Title: o.Title, Via: "link", Role: m})
		}
		if o.Breaks {
			break
		}
		cur = o.Parent
	}
	return held, reasons, nil
}

// Held is a role an account holds directly: on which object, and through what.
type Held struct {
	Object, Title, Kind, Area string
	Via, Name, Role           string
}

// HeldBy lists where an account holds a role of its own, through a group or as the whole tenant: the start of "what may
// X do" in the tab Rights. Administrators ask it for anyone, everyone else for themselves. Objects in the trash are left
// out, and nothing private is ever there.
func (db *DB) HeldBy(ctx context.Context, actor Actor, accountID string) ([]Held, error) {
	if accountID != actor.ID && !actor.Admin {
		return nil, ErrNotAllowed
	}
	groups, err := db.GroupsOf(ctx, accountID)
	if err != nil {
		return nil, err
	}
	rows, err := db.pool.Query(ctx, `
		SELECT o.id, o.title, o.kind, o.area, 'owner', '', 'owner' FROM objects o
		WHERE o.owner = $1 AND o.deleted_at IS NULL AND NOT o.private AND (o.parent IS NULL OR (SELECT p.owner FROM objects p WHERE p.id = o.parent) <> $1)
		UNION ALL
		SELECT o.id, o.title, o.kind, o.area, g.kind, coalesce(gr.name, ''), g.role FROM grants g JOIN objects o ON o.id = g.object
			LEFT JOIN groups gr ON g.kind = 'group' AND gr.id = g.who
		WHERE o.deleted_at IS NULL AND (g.kind = 'tenant' OR (g.kind = 'person' AND g.who = $1) OR (g.kind = 'group' AND g.who = ANY ($2)))
		ORDER BY 2, 1, 5`, accountID, groups)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Held, error) {
		var h Held
		err := row.Scan(&h.Object, &h.Title, &h.Kind, &h.Area, &h.Via, &h.Name, &h.Role)
		return h, err
	})
}

// ErrLastAdmin is taking away one's own administration.
var ErrLastAdmin = errors.New("nobody takes their own administration away, so a tenant keeps an administrator")

// SetAdmin makes a local account an administrator of the tenant, or takes that back (ADR-29). Accounts of the gate or of
// OIDC are administrators through their group, which their sign-in decides. Only administrators do; the caller checks.
func (db *DB) SetAdmin(ctx context.Context, actor Actor, accountID string, admin bool) error {
	if !actor.Admin {
		return ErrNotAllowed
	}
	a, err := db.AccountByID(ctx, accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if a.Source != SourceLocal {
		return ErrSource
	}
	if admin {
		_, err := db.pool.Exec(ctx, "UPDATE accounts SET admin = true WHERE id = $1", accountID)
		return err
	}
	// Whoever takes it back administers the tenant and is someone else, so the tenant keeps an administrator.
	if accountID == actor.ID {
		return ErrLastAdmin
	}
	_, err = db.pool.Exec(ctx, "UPDATE accounts SET admin = false WHERE id = $1", accountID)
	return err
}

// User is an account with how it signs in and whether it administers the tenant as a local account.
type User struct {
	Member
	Source string
	Admin  bool
	Groups []string
}

// Everyone returns every account of the tenant with its source, its local administration and its groups taken over,
// for the administration of users (ADR-16, ADR-29).
func (db *DB) Everyone(ctx context.Context) ([]User, error) {
	rows, err := db.pool.Query(ctx, `SELECT id, login, name, source, admin, groups FROM accounts ORDER BY lower(name), login`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (User, error) {
		var p User
		err := row.Scan(&p.ID, &p.Login, &p.Name, &p.Source, &p.Admin, &p.Groups)
		p.Groups = nonNil(p.Groups)
		return p, err
	})
}

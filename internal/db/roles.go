// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"
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

// RoleMembers is the preset Members (T-34): every action but delete. Whoever holds it manages members and rights, but
// deletes nothing and archives nothing. It stands beside the order of the roles, since it is no edit: Rank gives it none.
const RoleMembers = "members"

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
func grantable(role string) bool { return Rank(role) >= 1 && Rank(role) <= 4 || role == RoleMembers }

// Actor is who changes rights: an account, and whether it administers the tenant. Administrators manage the rights of
// everything that is not private, but read content only where they hold a role (ADR-29).
type Actor struct {
	ID    string
	Admin bool
	// Role tells that the actor administers through the role of the app in the gate, not only through the mark of the
	// command line.
	Role bool
	// Delegate tells that the actor holds the role of the delegated administrator in the gate (T-33, ADR-35): accounts of
	// every group, archiving and restoring groups, nothing else of an administrator.
	Delegate bool
}

// visible is a condition on o: the viewer ($1) may read it (ADR-36). The function object_may decides, the same for
// every query that lists, finds or opens objects, and a deny on read holds through every one of them.
const visible = `object_may(o.id, $1, 'read')`

// Role returns the role of ADR-29 an account holds on an object it sees, RoleNone where it sees nothing: owner, or the
// widest preset all of whose actions it may do. It names what the interface shows; the actions decide.
func (db *DB) Role(ctx context.Context, objectID, viewer string) (string, error) {
	var rank int
	if err := db.pool.QueryRow(ctx, "SELECT object_role($1, $2)", objectID, viewer).Scan(&rank); err != nil {
		return RoleNone, err
	}
	return roleOf(rank), nil
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

// forRights returns an object whose rights the actor may manage: one they may manage, or, for an administrator, any that
// is alive and not private, whatever a deny says (administrators manage rights, not content, ADR-36). Something private, or something the actor does not see and does not administer, is not found;
// one they see but may not manage is not allowed.
func (db *DB) forRights(ctx context.Context, objectID string, actor Actor) (Object, error) {
	// Agents never manage rights, administrators' agents included (ADR-36).
	if err := noAgent(ctx); err != nil {
		return Object{}, err
	}
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
	if err := db.mayDo(ctx, o, actor.ID, ActManage); err != nil {
		return Object{}, err
	}
	return o, nil
}

// Grant is what one holder is granted on an object: its entries per action (ADR-36), and the preset they make.
type Grant struct {
	Object string
	// Kind is ToPerson, ToGroup, ToTenant or ToAgents, and Who the account or the group; empty for the tenant and Agents.
	Kind, Who string
	// Name is how the interface names Who: the name of the account or of the group.
	Name string
	// Role is the preset the entries make: a role of ADR-29, PresetCustom, or RoleNone.
	Role string
	// Actions maps each action with an entry to Allow or Deny; an action without one inherits.
	Actions map[string]string
	// Subgroups is whether a grant to a group holds for the groups inside it too (the default).
	Subgroups bool
	By        string
	At        time.Time
}

// Rights are the entries that hold on an object: its owner, its own entries, and what it inherits from above.
type Rights struct {
	Owner, OwnerName string
	// Private tells that the object is private: its owner alone sees it, and no entry counts.
	Private bool
	// Inherit is false where the object breaks the inheritance.
	Inherit bool
	Own     []Grant
	// Inherited are the entries of the objects above, up to where the inheritance breaks, nearest first, with the object
	// each is on; the owners of those objects hold every action there and are listed as such.
	Inherited []Grant
	// Agents are the denies on Agents above, which hold as a ceiling whatever breaks the inheritance.
	Agents []Grant
}

// grantsQuery reads the entries of one object, a row per holder.
const grantsQuery = `SELECT g.object, g.kind, g.who,
		CASE g.kind WHEN 'person' THEN coalesce(min(a.name), g.who) WHEN 'group' THEN coalesce(min(gr.name), g.who) WHEN 'agents' THEN 'Agents' ELSE '' END,
		array_agg(g.action ORDER BY g.action), array_agg(g.value ORDER BY g.action), bool_and(g.subgroups),
		(array_agg(g.granted_by ORDER BY g.granted_at DESC))[1], max(g.granted_at)
	FROM grants g LEFT JOIN accounts a ON g.kind = 'person' AND a.id = g.who LEFT JOIN groups gr ON g.kind = 'group' AND gr.id = g.who
	WHERE g.object = $1 AND ($2 OR g.kind = 'agents')
	GROUP BY g.object, g.kind, g.who
	ORDER BY CASE g.kind WHEN 'agents' THEN 0 WHEN 'tenant' THEN 1 WHEN 'group' THEN 2 ELSE 3 END,
		lower(coalesce(min(a.name), min(gr.name), '')), g.who`

func scanGrant(row pgx.CollectableRow) (Grant, error) {
	var g Grant
	var actions, values []string
	err := row.Scan(&g.Object, &g.Kind, &g.Who, &g.Name, &actions, &values, &g.Subgroups, &g.By, &g.At)
	g.Actions = map[string]string{}
	for i, action := range actions {
		if i < len(values) {
			g.Actions[action] = values[i]
		}
	}
	g.Role = PresetOf(g.Actions)
	return g, err
}

func (db *DB) grantsOn(ctx context.Context, objectID string, all bool) ([]Grant, error) {
	rows, err := db.pool.Query(ctx, grantsQuery, objectID, all)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanGrant)
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
	if r.Own, err = db.grantsOn(ctx, objectID, true); err != nil {
		return Rights{}, err
	}
	inheriting := !o.Breaks
	for x := o; x.Parent != ""; {
		var parent Object
		parent, err = scan(db.pool.QueryRow(ctx, `SELECT `+columns+` FROM objects WHERE id = $1`, x.Parent))
		if err != nil {
			return Rights{}, err
		}
		above, err := db.grantsOn(ctx, parent.ID, inheriting)
		if err != nil {
			return Rights{}, err
		}
		if inheriting {
			// The owner of something above holds every action there, as an allow on that level.
			r.Inherited = append(r.Inherited, Grant{Object: parent.ID, Kind: ToPerson, Who: parent.Owner, Name: db.AccountName(ctx, parent.Owner),
				Role: RoleManage, Actions: allowAll(), Subgroups: true})
		}
		for _, g := range above {
			if g.Kind == ToAgents {
				r.Agents = append(r.Agents, g)
			} else {
				r.Inherited = append(r.Inherited, g)
			}
		}
		if parent.Breaks {
			inheriting = false
		}
		x = parent
	}
	return r, nil
}

func allowAll() map[string]string {
	m := map[string]string{}
	for _, a := range Actions {
		m[a] = Allow
	}
	return m
}

// SetGrant grants a preset (a role of ADR-29) on an object to a person, a group or the tenant, or takes every entry of
// that holder back with RoleNone: SetEntries with the actions of the preset, all allowed, with subgroups.
func (db *DB) SetGrant(ctx context.Context, objectID string, actor Actor, kind, who, role string) error {
	if role != RoleNone && !grantable(role) {
		return ErrRole
	}
	actions := map[string]string{}
	for _, a := range PresetActions(role) {
		actions[a] = Allow
	}
	return db.SetEntries(ctx, objectID, actor, kind, who, actions, true)
}

// ErrEntry is an entry that cannot be: an unknown action or value, an allow for Agents (Agents only narrow), or an
// entry for Agents on something that is not a place of its own.
var ErrEntry = errors.New("no such entry: an action is read, comment, create, write, delete, share or manage, each allow or deny, and Agents can only be denied")

// SetEntries sets every entry of one holder on an object at once (ADR-36): each action maps to Allow or Deny, and an
// action left out inherits. No action at all takes the holder away. The actor must manage the object, or administer
// the tenant; nothing private gets entries (ADR-29). Subgroups says whether a grant to a group holds for the groups
// inside it too.
func (db *DB) SetEntries(ctx context.Context, objectID string, actor Actor, kind, who string, actions map[string]string, subgroups bool) error {
	switch kind {
	case ToTenant, ToAgents:
		who = ""
	case ToPerson, ToGroup:
		if who == "" {
			return ErrNotFound
		}
	default:
		return ErrRole
	}
	if kind != ToGroup {
		subgroups = true
	}
	keys := make([]string, 0, len(actions))
	for action, value := range actions {
		if !IsAction(action) || (value != Allow && value != Deny && value != Inherit) {
			return ErrEntry
		}
		if kind == ToAgents && value == Allow {
			return ErrEntry
		}
		if value != Inherit {
			keys = append(keys, action)
		}
	}
	slices.SortFunc(keys, func(a, b string) int { return slices.Index(Actions, a) - slices.Index(Actions, b) })
	o, err := db.forRights(ctx, objectID, actor)
	if err != nil {
		return err
	}
	// In a library shared over Samba a deny on a single file would hide it in the web and show it on the share, which
	// knows rights per folder only (ADR-29, ADR-36): it is refused, as breaking the inheritance of a file is.
	if o.Kind == KindFile && kind != ToAgents && slices.ContainsFunc(keys, func(a string) bool { return actions[a] == Deny }) {
		if root, err := db.sharedRootOf(ctx, db.pool, objectID); err != nil || root != "" {
			if err == nil {
				err = ErrPerFolder
			}
			return err
		}
	}
	var known bool
	switch kind {
	case ToPerson:
		err = db.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM accounts WHERE id = $1)", who).Scan(&known)
	case ToGroup:
		// An archived group gets no new entries; what it holds can still be taken away.
		err = db.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM groups WHERE id = $1 AND archived IS NULL)", who).Scan(&known)
	default:
		known = true
	}
	if err != nil {
		return err
	}
	if !known && len(keys) > 0 {
		return ErrNotFound
	}
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		// One change of the entries of an object at a time: what it reads to decide is what it replaces (B-6).
		if _, err := tx.Exec(ctx, "SELECT 1 FROM objects WHERE id = $1 FOR UPDATE", objectID); err != nil {
			return err
		}
		// Whoever manages gives and opens up only what they hold themselves (T-34): the preset Members gives no delete.
		old, err := entriesOf(ctx, tx, objectID, kind, who)
		if err != nil {
			return err
		}
		if err := db.mayOpenUp(ctx, tx, o, actor, openedBy(old, actions, subgroups)); err != nil {
			return err
		}
		return setEntries(ctx, tx, objectID, kind, who, keys, actions, subgroups, actor.ID)
	})
}

// setEntries replaces the entries of one holder on an object with those named by keys, in one statement.
func setEntries(ctx context.Context, q execer, objectID, kind, who string, keys []string, actions map[string]string, subgroups bool, by string) error {
	values := make([]string, len(keys))
	for i, k := range keys {
		values[i] = actions[k]
	}
	var n int
	return q.QueryRow(ctx, `WITH gone AS (
			DELETE FROM grants WHERE object = $1 AND kind = $2 AND who = $3 AND NOT (action = ANY ($4::text[]))
		), made AS (
			INSERT INTO grants (object, kind, who, action, value, subgroups, granted_by)
			SELECT $1, $2, $3, a, v, $6, $7 FROM unnest($4::text[], $5::text[]) AS x(a, v)
			ON CONFLICT (tenant, object, kind, who, action) DO UPDATE
				SET value = EXCLUDED.value, subgroups = EXCLUDED.subgroups, granted_by = EXCLUDED.granted_by, granted_at = now()
			RETURNING 1
		) SELECT count(*) FROM made`, objectID, kind, who, nonNil(keys), nonNil(values), subgroups, by).Scan(&n)
}

// setGrant writes the preset of a role for one holder, replacing what that one held before.
func setGrant(ctx context.Context, q execer, objectID, kind, who, role, by string) error {
	actions := map[string]string{}
	keys := PresetActions(role)
	for _, a := range keys {
		actions[a] = Allow
	}
	return setEntries(ctx, q, objectID, kind, who, keys, actions, true, by)
}

// SetInherit breaks the inheritance of an object, or restores it. Breaking copies what held from above onto the object,
// for each holder and action the nearest entry, so nobody gains anything and nobody loses access by surprise where the
// entries above do not overlap; the manager then takes back what shall not hold (ADR-29, ADR-36). Where entries above
// overlap (an allow for someone nearer, a deny for a group of theirs farther up), deny on the one level now wins. The
// denies on Agents are not copied: they hold as a ceiling anyway. A collection at the top has nothing above it.
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
	// Restoring takes every entry from above again, which may open up any action: only whoever holds all of them here
	// does it (T-34). Breaking copies what holds and never opens up anything.
	if inherit {
		if err := db.mayOpenUp(ctx, db.pool, o, actor, Actions); err != nil {
			return err
		}
	}
	// In a library shared over Samba, rights hold per folder (ADR-29, ADR-30): a single file that broke the inheritance
	// would be hidden in the web and shown on the share.
	if !inherit && o.Kind == KindFile {
		if root, err := db.sharedRootOf(ctx, db.pool, objectID); err != nil || root != "" {
			if err == nil {
				err = ErrPerFolder
			}
			return err
		}
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
		type holder struct{ kind, who string }
		own := map[holder]map[string]bool{}
		rows, err := tx.Query(ctx, "SELECT kind, who, action FROM grants WHERE object = $1", objectID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var kind, who, action string
			if err := rows.Scan(&kind, &who, &action); err != nil {
				return err
			}
			h := holder{kind, who}
			if own[h] == nil {
				own[h] = map[string]bool{}
			}
			own[h][action] = true
		}
		if err := rows.Err(); err != nil {
			return err
		}
		// Nearest first: the first entry of a holder for an action is the one that held.
		type entry struct {
			value     string
			subgroups bool
		}
		taken := map[holder]map[string]entry{}
		var order []holder
		for _, g := range copied {
			h := holder{g.Kind, g.Who}
			if h.kind == ToPerson && h.who == o.Owner {
				continue
			}
			if taken[h] == nil {
				taken[h] = map[string]entry{}
				order = append(order, h)
			}
			for _, a := range Actions {
				v, ok := g.Actions[a]
				if !ok {
					continue
				}
				if _, done := taken[h][a]; done || own[h][a] {
					continue
				}
				taken[h][a] = entry{v, g.Subgroups}
			}
		}
		for _, h := range order {
			for _, a := range Actions {
				e, ok := taken[h][a]
				if !ok {
					continue
				}
				if _, err := tx.Exec(ctx, `INSERT INTO grants (object, kind, who, action, value, subgroups, granted_by) VALUES ($1, $2, $3, $4, $5, $6, $7)
					ON CONFLICT DO NOTHING`, objectID, h.kind, h.who, a, e.value, e.subgroups || h.kind != ToGroup, actor.ID); err != nil {
					return err
				}
			}
		}
		_, err = tx.Exec(ctx, "UPDATE objects SET inherit = $2, changed = now() WHERE id = $1", objectID, inherit)
		return err
	})
}

// TransferOwner gives an object to another account of the tenant. Only the owner or an administrator does, in a browser
// session; the former owner keeps the preset edit for every action of it they hold no entry for, which can be taken back like any other
// (ADR-29).
func (db *DB) TransferOwner(ctx context.Context, objectID string, actor Actor, to string) error {
	// A person in the browser only, never an agent, a service account, a key or an app password (T-34), as for archiving
	// and deleting for good.
	if err := browserSession(ctx, actor.ID); err != nil {
		return err
	}
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
		_, err := tx.Exec(ctx, `INSERT INTO grants (object, kind, who, action, value, granted_by)
			SELECT $1, 'person', $2, a, 'allow', $3 FROM unnest(preset_actions('edit')) AS a ON CONFLICT DO NOTHING`, objectID, o.Owner, actor.ID)
		return err
	})
}

// --- Groups

// Group is a group of accounts: local, or taken over from the sign-in. A local group contains persons, other local
// groups and groups taken over; membership is passed down (ADR-36).
type Group struct {
	ID          string
	Name        string
	Source      string
	Description string
	// Members counts the persons in it directly, Groups the groups in it directly.
	Members int
	Groups  int
	Created time.Time
	// Archived tells that the group is archived (B-3): kept with its members and entries, its allow entries holding for
	// nobody until it is restored, its deny entries still holding.
	Archived bool
}

const groupColumns = `g.id, g.name, g.source, g.description,
	CASE g.source WHEN 'local' THEN (SELECT count(*) FROM group_members m WHERE m.grp = g.id)
		ELSE (SELECT count(*) FROM accounts a WHERE g.name = ANY (a.groups)) END,
	(SELECT count(*) FROM group_groups gg WHERE gg.grp = g.id), g.created, g.archived IS NOT NULL`

func scanGroup(row pgx.Row) (Group, error) {
	var g Group
	err := row.Scan(&g.ID, &g.Name, &g.Source, &g.Description, &g.Members, &g.Groups, &g.Created, &g.Archived)
	return g, err
}

func collectGroups(rows pgx.Rows, err error) ([]Group, error) {
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Group, error) { return scanGroup(row) })
}

// Groups returns every group of the tenant, by name.
func (db *DB) Groups(ctx context.Context) ([]Group, error) {
	return collectGroups(db.pool.Query(ctx, `SELECT `+groupColumns+` FROM groups g ORDER BY lower(g.name), g.source, g.id`))
}

// GroupOf returns one group.
func (db *DB) GroupOf(ctx context.Context, groupID string) (Group, error) {
	g, err := scanGroup(db.pool.QueryRow(ctx, `SELECT `+groupColumns+` FROM groups g WHERE g.id = $1`, groupID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Group{}, ErrNotFound
	}
	return g, err
}

// GroupsOf returns the IDs of the groups an account belongs to, directly and through the groups that contain those.
func (db *DB) GroupsOf(ctx context.Context, accountID string) ([]string, error) {
	var groups []string
	err := db.pool.QueryRow(ctx, "SELECT account_groups($1)", accountID).Scan(&groups)
	return nonNil(groups), err
}

// DirectGroupsOf returns the IDs of the groups an account belongs to directly: a grant to a group without its
// subgroups holds for these only.
func (db *DB) DirectGroupsOf(ctx context.Context, accountID string) ([]string, error) {
	var groups []string
	err := db.pool.QueryRow(ctx, "SELECT account_direct_groups($1)", accountID).Scan(&groups)
	return nonNil(groups), err
}

// AllowGroupsOf returns the IDs of the groups whose allow entries hold for an account: those of GroupsOf that are not
// archived and not reached through an archived one, and for a service account no group taken over (B-3).
func (db *DB) AllowGroupsOf(ctx context.Context, accountID string) ([]string, error) {
	var groups []string
	err := db.pool.QueryRow(ctx, "SELECT account_allow_groups($1)", accountID).Scan(&groups)
	return nonNil(groups), err
}

// AllowDirectGroupsOf is AllowGroupsOf for the groups an account is in directly.
func (db *DB) AllowDirectGroupsOf(ctx context.Context, accountID string) ([]string, error) {
	var groups []string
	err := db.pool.QueryRow(ctx, "SELECT account_allow_direct_groups($1)", accountID).Scan(&groups)
	return nonNil(groups), err
}

// SubGroups returns the groups directly inside a group, by name.
func (db *DB) SubGroups(ctx context.Context, groupID string) ([]Group, error) {
	return collectGroups(db.pool.Query(ctx, `SELECT `+groupColumns+` FROM group_groups x JOIN groups g ON g.id = x.member WHERE x.grp = $1
		ORDER BY lower(g.name), g.source, g.id`, groupID))
}

// ParentGroups returns the groups a group is directly inside, by name.
func (db *DB) ParentGroups(ctx context.Context, groupID string) ([]Group, error) {
	return collectGroups(db.pool.Query(ctx, `SELECT `+groupColumns+` FROM group_groups x JOIN groups g ON g.id = x.grp WHERE x.member = $1
		ORDER BY lower(g.name), g.source, g.id`, groupID))
}

// GroupAdmins returns the admins of a group, by name.
func (db *DB) GroupAdmins(ctx context.Context, groupID string) ([]Member, error) {
	rows, err := db.pool.Query(ctx, `SELECT a.id, a.login, a.name FROM group_admins x JOIN accounts a ON a.id = x.account WHERE x.grp = $1
		ORDER BY lower(a.name), a.login`, groupID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanMember)
}

func scanMember(row pgx.CollectableRow) (Member, error) {
	var m Member
	err := row.Scan(&m.ID, &m.Login, &m.Name)
	return m, err
}

// AdministeredGroups returns the IDs of the groups an account administers as a group admin: those it is an admin of,
// and every group inside them at any depth (ADR-36), as far as they are not archived.
func (db *DB) AdministeredGroups(ctx context.Context, accountID string) ([]string, error) {
	var groups []string
	// An archived group gives its admins nothing, and nothing passes down through it (B-3).
	err := db.pool.QueryRow(ctx, `WITH RECURSIVE down(id) AS (
			SELECT a.grp FROM group_admins a JOIN groups g ON g.id = a.grp WHERE a.account = $1 AND g.archived IS NULL
			UNION SELECT gg.member FROM group_groups gg JOIN down ON gg.grp = down.id
				JOIN groups g ON g.id = down.id WHERE g.archived IS NULL
		) SELECT coalesce(array_agg(id ORDER BY id), '{}') FROM down`, accountID).Scan(&groups)
	return nonNil(groups), err
}

// ErrGroupAdmin is a change of a group by someone who administers neither the tenant nor the group or a group above it.
var ErrGroupAdmin = fmt.Errorf("%w: only administrators and the admins of this group or of a group above it change it", ErrNotAllowed)

// mayAdministerGroup refuses whoever may not manage a group: administrators of the tenant manage every group, the admins
// of a group manage it and the groups inside it (ADR-36). Agents manage no group.
func (db *DB) mayAdministerGroup(ctx context.Context, actor Actor, groupID string) error {
	if err := noAgent(ctx); err != nil {
		return err
	}
	if actor.Admin {
		return nil
	}
	mine, err := db.AdministeredGroups(ctx, actor.ID)
	if err != nil {
		return err
	}
	if !slices.Contains(mine, groupID) {
		return ErrGroupAdmin
	}
	return nil
}

// MayAdministerGroup tells whether the actor manages a group.
func (db *DB) MayAdministerGroup(ctx context.Context, actor Actor, groupID string) (bool, error) {
	err := db.mayAdministerGroup(ctx, actor, groupID)
	if isNotAllowed(err) {
		return false, nil
	}
	return err == nil, err
}

// ErrGroupName is a name of a group that is empty, too long, or taken by another local group.
var ErrGroupName = errors.New("a group has a name of 1 to 200 characters that no other local group has")

// CreateGroup makes a local group. Administrators make one anywhere; the admin of a group makes one inside it, which then
// is inside it from the start (parent).
func (db *DB) CreateGroup(ctx context.Context, actor Actor, name, description, parent string) (Group, error) {
	name, description = strings.TrimSpace(name), strings.TrimSpace(description)
	if n := len([]rune(name)); n < 1 || n > 200 || len([]rune(description)) > 1000 {
		return Group{}, ErrGroupName
	}
	if parent == "" {
		if err := noAgent(ctx); err != nil {
			return Group{}, err
		}
		if !actor.Admin {
			return Group{}, ErrGroupAdmin
		}
	} else {
		p, err := db.GroupOf(ctx, parent)
		if err != nil {
			return Group{}, err
		}
		if p.Source != GroupLocal {
			return Group{}, ErrGroup
		}
		if p.Archived {
			return Group{}, ErrGroupArchived
		}
		if err := db.mayAdministerGroup(ctx, actor, parent); err != nil {
			return Group{}, err
		}
	}
	for attempt := 0; ; attempt++ {
		groupID := id.New()
		err := pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, "INSERT INTO groups (id, name, source, description) VALUES ($1, $2, 'local', $3)", groupID, name, description); err != nil {
				return err
			}
			if parent == "" {
				return nil
			}
			_, err := tx.Exec(ctx, "INSERT INTO group_groups (grp, member) VALUES ($1, $2)", parent, groupID)
			return err
		})
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

// localGroup returns a local group the actor manages. An archived group stays as it was until it is restored.
func (db *DB) localGroup(ctx context.Context, actor Actor, groupID string) (Group, error) {
	g, err := db.GroupOf(ctx, groupID)
	if err != nil {
		return Group{}, err
	}
	if g.Source != GroupLocal {
		return Group{}, ErrGroup
	}
	if err := db.mayAdministerGroup(ctx, actor, groupID); err != nil {
		return Group{}, err
	}
	if g.Archived {
		return Group{}, ErrGroupArchived
	}
	return g, nil
}

// RenameGroup gives a local group a new name and description.
func (db *DB) RenameGroup(ctx context.Context, actor Actor, groupID, name, description string) error {
	if _, err := db.localGroup(ctx, actor, groupID); err != nil {
		return err
	}
	name, description = strings.TrimSpace(name), strings.TrimSpace(description)
	if n := len([]rune(name)); n < 1 || n > 200 || len([]rune(description)) > 1000 {
		return ErrGroupName
	}
	_, err := db.pool.Exec(ctx, "UPDATE groups SET name = $2, description = $3 WHERE id = $1", groupID, name, description)
	if isUnique(err, "groups_tenant_source_name_key") {
		return ErrGroupName
	}
	return err
}

// Errors of archiving groups (B-3).
var (
	// ErrGroupArchived is a change of an archived group, or of a group inside or around one, that has to wait until it is
	// restored.
	ErrGroupArchived = errors.New("this group is archived: restore it first, or the group it is inside")
	// ErrGroupHasGroups is archiving a group with groups inside it that are not archived.
	ErrGroupHasGroups = errors.New("this group has groups inside it: take them out or archive them first")
	// ErrGroupArchiver is archiving or restoring a group by someone who is no administrator (B-5) or delegated
	// administrator (T-33).
	ErrGroupArchiver = fmt.Errorf("%w: only administrators archive or restore a group", ErrNotAllowed)
)

// lockGroups takes the lock of the tenant's groups that the check of group_groups takes (migration 030), so a group
// put inside another and an archiving cannot pass each other.
func lockGroups(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('underdoc groups ' || current_setting('underdoc.tenant', true)))")
	return err
}

// mayArchiveGroup refuses whoever may not archive or restore a group: administrators of the tenant only (B-5). Group
// admins never do, also not those of a group it is inside: through nesting (put it inside, take oneself out as its admin,
// archive, restore, take oneself in again) every such rule could be walked around. The delegated administrator does as
// well (T-33). Agents never do.
func (db *DB) mayArchiveGroup(ctx context.Context, _ execer, actor Actor, _ string) error {
	if err := noAgent(ctx); err != nil {
		return err
	}
	if !actor.Admin && !actor.Delegate {
		return ErrGroupArchiver
	}
	return nil
}

// MayArchiveGroup tells whether the actor may archive or restore a group.
func (db *DB) MayArchiveGroup(ctx context.Context, actor Actor, groupID string) (bool, error) {
	err := db.mayArchiveGroup(ctx, db.pool, actor, groupID)
	if isNotAllowed(err) {
		return false, nil
	}
	return err == nil, err
}

// ArchiveGroup archives a local group instead of deleting it (B-3): it keeps its members, the groups inside it, its
// admins and every entry granted to it, so RestoreGroup brings everything back. While it is archived, its allow entries
// hold for nobody and nothing passes up through it; its deny entries keep holding for whoever is in it, so archiving
// never gives anyone more. Only administrators archive it (B-5), and only while every group inside it is archived
// already. Nothing deletes a group for good.
func (db *DB) ArchiveGroup(ctx context.Context, actor Actor, groupID string) error {
	g, err := db.GroupOf(ctx, groupID)
	if err != nil {
		return err
	}
	if g.Source != GroupLocal {
		return ErrGroup
	}
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if err := lockGroups(ctx, tx); err != nil {
			return err
		}
		if err := db.mayArchiveGroup(ctx, tx, actor, groupID); err != nil {
			return err
		}
		var archived, inner bool
		if err := tx.QueryRow(ctx, `SELECT archived IS NOT NULL,
				EXISTS (SELECT 1 FROM group_groups gg JOIN groups m ON m.id = gg.member WHERE gg.grp = $1 AND m.archived IS NULL)
			FROM groups WHERE id = $1`, groupID).Scan(&archived, &inner); err != nil {
			return err
		}
		if archived {
			return ErrGroupArchived
		}
		if inner {
			return ErrGroupHasGroups
		}
		_, err := tx.Exec(ctx, "UPDATE groups SET archived = now(), archived_by = $2 WHERE id = $1", groupID, actor.ID)
		return err
	})
}

// RestoreGroup brings an archived group back with everything it kept. Only administrators do, as for archiving it, and
// only while no group it is inside is archived.
func (db *DB) RestoreGroup(ctx context.Context, actor Actor, groupID string) error {
	g, err := db.GroupOf(ctx, groupID)
	if err != nil {
		return err
	}
	if g.Source != GroupLocal {
		return ErrGroup
	}
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if err := lockGroups(ctx, tx); err != nil {
			return err
		}
		if err := db.mayArchiveGroup(ctx, tx, actor, groupID); err != nil {
			return err
		}
		var archived, outer bool
		if err := tx.QueryRow(ctx, `SELECT archived IS NOT NULL,
				EXISTS (SELECT 1 FROM group_groups gg JOIN groups p ON p.id = gg.grp WHERE gg.member = $1 AND p.archived IS NOT NULL)
			FROM groups WHERE id = $1`, groupID).Scan(&archived, &outer); err != nil {
			return err
		}
		if !archived {
			return ErrNotFound
		}
		if outer {
			return ErrGroupArchived
		}
		_, err := tx.Exec(ctx, "UPDATE groups SET archived = NULL, archived_by = '' WHERE id = $1", groupID)
		return err
	})
}

// Member is an account in a group.
type Member struct {
	ID, Login, Name string
}

// GroupMembers returns the persons directly in a group, by name.
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
	return pgx.CollectRows(rows, scanMember)
}

// SetMember adds an account to a local group, or takes it out.
func (db *DB) SetMember(ctx context.Context, actor Actor, groupID, accountID string, member bool) error {
	if _, err := db.localGroup(ctx, actor, groupID); err != nil {
		return err
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
	_, err := db.pool.Exec(ctx, "INSERT INTO group_members (grp, account) VALUES ($1, $2) ON CONFLICT DO NOTHING", groupID, accountID)
	return err
}

// InviteMember puts an account a person was just invited as into a local group (T-33): who may change the members of
// the group, and the delegated administrator into any local group. The group must not be archived.
func (db *DB) InviteMember(ctx context.Context, actor Actor, groupID, accountID string) error {
	if err := noAgent(ctx); err != nil {
		return err
	}
	g, err := db.GroupOf(ctx, groupID)
	if err != nil {
		return err
	}
	if g.Source != GroupLocal {
		return ErrGroup
	}
	if g.Archived {
		return ErrGroupArchived
	}
	if !actor.Delegate {
		if err := db.mayAdministerGroup(ctx, actor, groupID); err != nil {
			return err
		}
	}
	_, err = db.pool.Exec(ctx, "INSERT INTO group_members (grp, account) VALUES ($1, $2) ON CONFLICT DO NOTHING", groupID, accountID)
	return err
}

// MayInvite tells whether the actor may invite a person into a group: InviteMember would take them.
func (db *DB) MayInvite(ctx context.Context, actor Actor, groupID string) (bool, error) {
	if actor.Delegate || actor.Admin {
		return !requestAgent(ctx), nil
	}
	return db.MayAdministerGroup(ctx, actor, groupID)
}

// ErrGroupCycle is a group put inside itself, directly or through the groups inside it.
var ErrGroupCycle = errors.New("a group cannot be inside itself, also not through the groups inside it")

// SetSubgroup puts a group inside a local group, or takes it out (ADR-36). The inner group may be local or taken over
// from the sign-in; a group taken over contains nothing. A cycle is refused, checked by the database under a lock.
func (db *DB) SetSubgroup(ctx context.Context, actor Actor, groupID, memberID string, on bool) error {
	if _, err := db.localGroup(ctx, actor, groupID); err != nil {
		return err
	}
	if !on {
		_, err := db.pool.Exec(ctx, "DELETE FROM group_groups WHERE grp = $1 AND member = $2", groupID, memberID)
		return err
	}
	inner, err := db.GroupOf(ctx, memberID)
	if err != nil {
		return err
	}
	if groupID == memberID {
		return ErrGroupCycle
	}
	if inner.Archived {
		return ErrGroupArchived
	}
	// A group admin puts only a local group inside theirs that they manage already: inside it, they would manage it.
	if inner.Source == GroupLocal {
		if err := db.mayAdministerGroup(ctx, actor, memberID); err != nil {
			return err
		}
	}
	_, err = db.pool.Exec(ctx, "INSERT INTO group_groups (grp, member) VALUES ($1, $2) ON CONFLICT DO NOTHING", groupID, memberID)
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23514" && (pg.ConstraintName == "group_groups_cycle" || pg.ConstraintName == "group_groups_check") {
		return ErrGroupCycle
	}
	if errors.As(err, &pg) && pg.Code == "23514" && pg.ConstraintName == "group_groups_local" {
		return ErrGroup
	}
	if errors.As(err, &pg) && pg.Code == "23514" && pg.ConstraintName == "group_groups_archived" {
		return ErrGroupArchived
	}
	return err
}

// SetGroupAdmin makes an account an admin of a local group, or takes that back.
func (db *DB) SetGroupAdmin(ctx context.Context, actor Actor, groupID, accountID string, admin bool) error {
	if _, err := db.localGroup(ctx, actor, groupID); err != nil {
		return err
	}
	if !admin {
		_, err := db.pool.Exec(ctx, "DELETE FROM group_admins WHERE grp = $1 AND account = $2", groupID, accountID)
		return err
	}
	var known bool
	if err := db.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM accounts WHERE id = $1)", accountID).Scan(&known); err != nil {
		return err
	}
	if !known {
		return ErrNotFound
	}
	_, err := db.pool.Exec(ctx, "INSERT INTO group_admins (grp, account) VALUES ($1, $2) ON CONFLICT DO NOTHING", groupID, accountID)
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

// Explain tells the role an account holds on an object and, for each action, whether it is allowed and the deciding
// entry with where it comes from (ADR-36), for the access check of the tab Rights. Agent asks it as for a request of an
// agent of that account. Whoever manages the object asks it, and administrators; anyone may ask it for themselves.
func (db *DB) Explain(ctx context.Context, objectID string, actor Actor, accountID string, agent bool) (string, []Decision, error) {
	if accountID != actor.ID {
		if _, err := db.forRights(ctx, objectID, actor); err != nil {
			return "", nil, err
		}
	} else if _, err := db.Object(ctx, objectID, actor.ID); err != nil {
		return RoleNone, nil, nil
	}
	var decisions []Decision
	held := map[string]bool{}
	for _, action := range Actions {
		d, err := db.Decide(ctx, objectID, accountID, action, agent)
		if err != nil {
			return "", nil, err
		}
		held[action] = d.Allowed
		decisions = append(decisions, d)
	}
	role := RoleNone
	for _, r := range []string{RoleRead, RoleComment, RoleEdit, RoleManage} {
		all := true
		for _, a := range PresetActions(r) {
			all = all && held[a]
		}
		if all {
			role = r
		}
	}
	if len(decisions) > 0 && decisions[0].Allowed && (decisions[0].Via == ViaOwner || decisions[0].Via == ViaPrivate) && !agent {
		role = RoleOwner
	}
	return role, decisions, nil
}

// Held is what an account holds directly: on which object, and through what.
type Held struct {
	Object, Title, Kind, Area string
	// Via is owner, person, group or tenant; Name names the group.
	Via, Name string
	// Role is the preset of the entries, PresetCustom where they are none; Actions the entries.
	Role    string
	Actions map[string]string
}

// HeldBy lists where an account holds entries of its own, through a group or as the whole tenant, allow or deny: the
// start of "what may X do" in the tab Rights. Administrators ask it for anyone, everyone else for themselves. Objects in
// the trash are left out, and nothing private is ever there. An allow that does not hold for the account is left out: one
// through an archived group, and for a service account one for the whole tenant or a role of the gate (B-4); its denies
// stay, as they hold.
func (db *DB) HeldBy(ctx context.Context, actor Actor, accountID string) ([]Held, error) {
	if accountID != actor.ID && !actor.Admin {
		return nil, ErrNotAllowed
	}
	groups, err := db.GroupsOf(ctx, accountID)
	if err != nil {
		return nil, err
	}
	direct, err := db.DirectGroupsOf(ctx, accountID)
	if err != nil {
		return nil, err
	}
	allow, err := db.AllowGroupsOf(ctx, accountID)
	if err != nil {
		return nil, err
	}
	var service bool
	if err := db.pool.QueryRow(ctx, "SELECT account_service($1)", accountID).Scan(&service); err != nil {
		return nil, err
	}
	rows, err := db.pool.Query(ctx, `
		SELECT o.id, o.title, o.kind, o.area, 'owner', '', '', '{}'::text[], '{}'::text[] FROM objects o
		WHERE o.owner = $1 AND o.deleted_at IS NULL AND NOT object_private(o.id)
		  AND (o.parent IS NULL OR (SELECT p.owner FROM objects p WHERE p.id = o.parent) <> $1)
		UNION ALL
		SELECT o.id, o.title, o.kind, o.area, g.kind, coalesce(min(gr.name), ''), coalesce(g.who, ''), array_agg(g.action ORDER BY g.action), array_agg(g.value ORDER BY g.action)
		FROM grants g JOIN objects o ON o.id = g.object LEFT JOIN groups gr ON g.kind = 'group' AND gr.id = g.who
		WHERE o.deleted_at IS NULL AND NOT object_private(o.id)
		  AND (g.kind = 'tenant' OR (g.kind = 'person' AND g.who = $1)
		       OR (g.kind = 'group' AND g.who = ANY (CASE WHEN g.subgroups THEN $2::text[] ELSE $3::text[] END)))
		GROUP BY o.id, o.title, o.kind, o.area, g.kind, g.who
		ORDER BY 2, 1, 5`, accountID, groups, direct)
	if err != nil {
		return nil, err
	}
	all, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Held, error) {
		var h Held
		var who string
		var actions, values []string
		err := row.Scan(&h.Object, &h.Title, &h.Kind, &h.Area, &h.Via, &h.Name, &who, &actions, &values)
		if h.Via == ViaOwner {
			h.Role, h.Actions = RoleOwner, allowAll()
			return h, err
		}
		allows := h.Via == ToPerson || h.Via == ToTenant && !service || h.Via == ToGroup && slices.Contains(allow, who)
		h.Actions = map[string]string{}
		for i, a := range actions {
			if i < len(values) && (values[i] == Deny || allows) {
				h.Actions[a] = values[i]
			}
		}
		h.Role = PresetOf(h.Actions)
		return h, err
	})
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(all, func(h Held) bool { return len(h.Actions) == 0 }), nil
}

// AgentLimit returns what an account lets its own agents do (ADR-36): at most AgentActions, all of them by default.
func (db *DB) AgentLimit(ctx context.Context, accountID string) ([]string, error) {
	var actions []string
	err := db.pool.QueryRow(ctx, "SELECT actions FROM agent_limits WHERE account = $1", accountID).Scan(&actions)
	if errors.Is(err, pgx.ErrNoRows) {
		return slices.Clone(AgentActions), nil
	}
	return nonNil(actions), err
}

// SetAgentLimit sets what an account lets its own agents do. Only the account itself does, signed in in a session: an
// agent cannot widen its own limit.
func (db *DB) SetAgentLimit(ctx context.Context, accountID string, actions []string) error {
	if err := noAgent(ctx); err != nil {
		return err
	}
	var keep []string
	for _, a := range AgentActions {
		if slices.Contains(actions, a) {
			keep = append(keep, a)
		}
	}
	for _, a := range actions {
		if !slices.Contains(AgentActions, a) {
			return ErrEntry
		}
	}
	_, err := db.pool.Exec(ctx, `INSERT INTO agent_limits (account, actions) VALUES ($1, $2)
		ON CONFLICT (tenant, account) DO UPDATE SET actions = EXCLUDED.actions, changed = now()`, accountID, nonNil(keep))
	return err
}

// ErrLastAdmin is taking away one's own administration.
var ErrLastAdmin = errors.New("nobody takes their own administration away, so a tenant keeps an administrator")

// ErrMarkOnCommandLine is making an administrator in the web: the role of the app in the gate does that, and in an
// emergency the command line (ADR-32).
var ErrMarkOnCommandLine = errors.New("an administrator holds the role of the app in the gate; in an emergency, underdoc admin grant marks one")

// SetAdmin takes back the mark of an administrator from an account (ADR-32). The mark is the emergency access of the
// command line, and only the command line gives it; whoever administers through the role of the app in the gate does so
// while the gate gives it. Only administrators do; the caller checks.
func (db *DB) SetAdmin(ctx context.Context, actor Actor, accountID string, admin bool) error {
	if err := noAgent(ctx); err != nil {
		return err
	}
	if !actor.Admin {
		return ErrNotAllowed
	}
	if admin {
		return ErrMarkOnCommandLine
	}
	if accountID == actor.ID {
		return ErrLastAdmin
	}
	a, err := db.AccountByID(ctx, accountID)
	if err != nil {
		return err
	}
	if !a.Admin {
		return ErrSource
	}
	// The mark of the actor is what the request found at its start. Two marked administrators could each take the
	// other's mark at the same moment, so the database decides under a lock: an actor without the role must still be
	// marked.
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockAdmins); err != nil {
			return err
		}
		var still bool
		if err := tx.QueryRow(ctx, "SELECT coalesce((SELECT admin FROM accounts WHERE id = $1), false)", actor.ID).Scan(&still); err != nil {
			return err
		}
		if !actor.Role && !still {
			return ErrNotAllowed
		}
		_, err := tx.Exec(ctx, "UPDATE accounts SET admin = false WHERE id = $1", accountID)
		return err
	})
}

// User is an account with where it came from, whether it is marked as an administrator, and its groups taken over.
type User struct {
	Member
	Source string
	Admin  bool
	Groups []string
	// Bound tells that the account is bound to a lasting ID of the gate (ADR-32); one that is not waits for its binding.
	Bound bool
	// Locked tells that UnderDoc locked the account (T-33); Service that it is a service account (B-3).
	Locked, Service bool
}

// Everyone returns every account of the tenant with its source, its mark of an administrator and its groups taken over,
// for the administration of users (ADR-16, ADR-29).
func (db *DB) Everyone(ctx context.Context) ([]User, error) {
	rows, err := db.pool.Query(ctx, `SELECT id, login, name, source, admin, groups, remote_id IS NOT NULL, locked IS NOT NULL, service
		FROM accounts ORDER BY lower(name), login`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (User, error) {
		var p User
		err := row.Scan(&p.ID, &p.Login, &p.Name, &p.Source, &p.Admin, &p.Groups, &p.Bound, &p.Locked, &p.Service)
		p.Groups = nonNil(p.Groups)
		return p, err
	})
}

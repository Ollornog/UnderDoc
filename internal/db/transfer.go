// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/id"
	"github.com/Ollornog/UnderDoc/internal/pagekind"
)

// Export and import (ADR-13, ADR-19): the database hands out whole trees and takes whole trees in. Turning a tree into
// files and back is the job of package transfer; this file only reads and writes the database.

// Node is an object with everything below it and, for a page, its blocks.
type Node struct {
	Object
	// Key is the name the links of an imported tree use for this node, as /o/<key>; Import puts its new ID in their place.
	Key string
	// NextBlock is the number the next block of a page gets, so an ID deleted at the end is not given out again.
	NextBlock int
	Blocks    []Block
	Kids      []*Node
	// Entries are the entries of rights on it beyond the role of the tenant (Access), with names that mean the same on
	// another instance (T-15).
	Entries []RightsEntry
}

// RightsEntry is an entry of rights as an export writes it (T-15): its holder by a name another instance knows (the login of
// a person, the name of a group; none for the tenant and Agents), and the actions it allows and denies.
type RightsEntry struct {
	Kind, Name  string
	Subgroups   bool
	Allow, Deny []string
}

// Owned is a private collection with the login of the account it belongs to.
type Owned struct {
	Login string
	Node  *Node
}

// MaxExport is how many objects the web exports at once. The command line has no limit.
const MaxExport = 5000

// exportLimit is MaxExport, lowered by a test.
var exportLimit = MaxExport

// ErrNoAccount is an import for an account that does not exist.
var ErrNoAccount = errors.New("no such account: it is made at the first sign-in")

// Everything reads every collection that is not in the trash, as an administrator on the server needs it for a full
// export: the shared ones in their order, and the private ones of every account with its login (ADR-20 keeps those from
// administrators in the web, not from whoever runs the server).
func (db *DB) Everything(ctx context.Context) (shared []*Node, private []Owned, err error) {
	rows, err := db.pool.Query(ctx, `SELECT `+columns+`, next_block FROM objects o WHERE `+alive+` AND area = 'docs' ORDER BY position, created, id`)
	if err != nil {
		return nil, nil, err
	}
	nodes, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*Node, error) {
		n := &Node{}
		o := &n.Object
		err := row.Scan(append(fields(o), &n.NextBlock)...)
		return n, err
	})
	if err != nil {
		return nil, nil, err
	}
	byID := make(map[string]*Node, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}
	if err := db.fillBlocks(ctx, byID); err != nil {
		return nil, nil, err
	}
	if err := db.fillEntries(ctx, byID); err != nil {
		return nil, nil, err
	}
	logins, err := db.logins(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, n := range nodes {
		// A shared collection at the top that grants the tenant nothing says so, since an export from before ADR-29 has
		// no level there and means edit.
		if n.Parent == "" && !n.Private && n.Access == "" {
			n.Access = AccessNone
		}
		switch {
		case n.Parent != "":
			if p := byID[n.Parent]; p != nil {
				p.Kids = append(p.Kids, n)
			}
		case n.Private:
			private = append(private, Owned{Login: logins[n.Owner], Node: n})
		default:
			shared = append(shared, n)
		}
	}
	return shared, private, nil
}

func (db *DB) fillBlocks(ctx context.Context, byID map[string]*Node) error {
	rows, err := db.pool.Query(ctx, "SELECT page, id, body, coalesce(align, '') FROM blocks ORDER BY page, position")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var page string
		var b Block
		if err := rows.Scan(&page, &b.ID, &b.Body, &b.Align); err != nil {
			return err
		}
		if n := byID[page]; n != nil {
			n.Blocks = append(n.Blocks, b)
		}
	}
	return rows.Err()
}

// fillEntries gives each node the entries of rights on it (T-15): persons by their login, groups by their name, the tenant
// where its entries are no preset that Access names, and the denies on Agents. Nothing private has entries.
func (db *DB) fillEntries(ctx context.Context, byID map[string]*Node) error {
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	rows, err := db.pool.Query(ctx, `SELECT g.object, g.kind, g.who, g.action, g.value, g.subgroups,
			coalesce(CASE g.kind WHEN 'person' THEN (SELECT login FROM accounts a WHERE a.id = g.who)
				WHEN 'group' THEN (SELECT name FROM groups r WHERE r.id = g.who) END, '')
		FROM grants g WHERE g.object = ANY ($1::text[]) ORDER BY g.object, g.kind, g.who`, ids)
	if err != nil {
		return err
	}
	type key struct{ object, kind, who string }
	found := map[key]*RightsEntry{}
	var order []key
	err = func() error {
		defer rows.Close()
		for rows.Next() {
			var k key
			var action, value, name string
			var subgroups bool
			if err := rows.Scan(&k.object, &k.kind, &k.who, &action, &value, &subgroups, &name); err != nil {
				return err
			}
			e := found[k]
			if e == nil {
				e = &RightsEntry{Kind: k.kind, Name: name, Subgroups: subgroups}
				found[k] = e
				order = append(order, k)
			}
			if value == Deny {
				e.Deny = append(e.Deny, action)
			} else {
				e.Allow = append(e.Allow, action)
			}
		}
		return rows.Err()
	}()
	if err != nil {
		return err
	}
	byAction := func(a, b string) int { return slices.Index(Actions, a) - slices.Index(Actions, b) }
	for _, k := range order {
		n, e := byID[k.object], found[k]
		if n == nil || n.Private || (k.kind != ToTenant && k.kind != ToAgents && e.Name == "") {
			continue
		}
		// The role of the tenant goes out as access, when its entries are a preset; otherwise as entries, and access says
		// nothing (an import gives the tenant what the entries say).
		if k.kind == ToTenant && n.Access != "" && n.Access != AccessNone && n.Access != PresetCustom {
			continue
		}
		if k.kind == ToTenant && n.Access == PresetCustom {
			n.Access = ""
		}
		slices.SortFunc(e.Allow, byAction)
		slices.SortFunc(e.Deny, byAction)
		n.Entries = append(n.Entries, *e)
	}
	return nil
}

func (db *DB) logins(ctx context.Context) (map[string]string, error) {
	rows, err := db.pool.Query(ctx, "SELECT id, login FROM accounts")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var accountID, login string
		if err := rows.Scan(&accountID, &login); err != nil {
			return nil, err
		}
		out[accountID] = login
	}
	return out, rows.Err()
}

// Subtree reads an object with everything below it, as the viewer sees it, for an export from the web. More than
// MaxExport objects is ErrTooMany.
func (db *DB) Subtree(ctx context.Context, objectID, viewer string) (*Node, error) {
	o, err := db.Object(ctx, objectID, viewer)
	if err != nil {
		return nil, err
	}
	if o.Area != AreaDocs {
		return nil, ErrPlace
	}
	count := 0
	byID := map[string]*Node{}
	var read func(o Object) (*Node, error)
	read = func(o Object) (*Node, error) {
		if count++; count > exportLimit {
			return nil, ErrTooMany
		}
		n := &Node{Object: o}
		if HasBlocks(o.Kind) {
			if err := db.pool.QueryRow(ctx, "SELECT next_block FROM objects WHERE id = $1", o.ID).Scan(&n.NextBlock); err != nil {
				return nil, err
			}
			if n.Blocks, err = db.Blocks(ctx, o.ID, viewer); err != nil {
				return nil, err
			}
		}
		kids, err := db.Children(ctx, o.ID, viewer)
		if err != nil {
			return nil, err
		}
		for _, k := range kids {
			c, err := read(k)
			if err != nil {
				return nil, err
			}
			n.Kids = append(n.Kids, c)
		}
		byID[n.ID] = n
		return n, nil
	}
	top, err := read(o)
	if err != nil {
		return nil, err
	}
	// Whoever sees an object reads its rights (RightsOf), so the export carries them as well.
	if err := db.fillEntries(ctx, byID); err != nil {
		return nil, err
	}
	return top, nil
}

// Imported says what an import did.
type Imported struct {
	Top Object
	// Objects is how many objects were made, the top one included.
	Objects int
	// Renamed maps each ID that was taken or not valid to the new one it got instead.
	Renamed map[string]string
	// Skipped names the entries of rights that were left out, as a sentence each: a person or group not here, an entry
	// that cannot be (T-15).
	Skipped []string
}

var blockID = regexp.MustCompile(`^b[1-9][0-9]{0,8}$`)

// Import makes a new collection at the top from a tree read from files, owned by the account with that login, in one
// transaction. Every object keeps its ID where that is valid and free, and gets a new one where not; nothing that exists
// is ever touched. The kind of the top is always collection, its title the given one. Blocks keep their IDs, which the
// reader has made unique and valid in their page, and the next block of a page comes after the highest one and never
// before the NextBlock it brings. Every block starts its history as a version by the owner.
func (db *DB) Import(ctx context.Context, login, title string, top *Node) (Imported, error) {
	var owner string
	err := db.pool.QueryRow(ctx, "SELECT id FROM accounts WHERE login = $1 ORDER BY first_seen LIMIT 1", login).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return Imported{}, ErrNoAccount
	}
	if err != nil {
		return Imported{}, err
	}
	result := Imported{Renamed: map[string]string{}}
	err = pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		// First every object, so each has its ID; then the blocks, with the links between the imported pages and their
		// embeddings pointing at those IDs.
		type made struct {
			n  *Node
			id string
		}
		var all []made
		ids := map[string]string{}
		var write func(n *Node, parent string, position int) (Object, error)
		write = func(n *Node, parent string, position int) (Object, error) {
			o := n.Object
			o.Owner, o.Parent, o.Position = owner, parent, position
			o.Private = o.Private && parent == ""
			if IsDivider(o.Kind) {
				o.Title, o.Description, o.Access, o.Tags = o.Kind, "", "", nil
			}
			if clean, err := CleanTags(o.Tags); err == nil {
				o.Tags = clean
			} else {
				o.Tags = nil
			}
			m, err := insertWithID(ctx, tx, o)
			if err != nil {
				return Object{}, err
			}
			if o.ID != "" {
				ids[o.ID] = m.ID
				if m.ID != o.ID {
					result.Renamed[o.ID] = m.ID
				}
			}
			if n.Key != "" {
				ids[n.Key] = m.ID
			}
			result.Objects++
			all = append(all, made{n, m.ID})
			for i, k := range n.Kids {
				if _, err := write(k, m.ID, i); err != nil {
					return Object{}, err
				}
			}
			return m, nil
		}
		root := *top
		root.Kind, root.Title = KindCollection, title
		var position int
		if err := tx.QueryRow(ctx, "SELECT coalesce(max(position) + 1, 0) FROM objects WHERE parent IS NULL").Scan(&position); err != nil {
			return err
		}
		m, err := write(&root, "", position)
		if err != nil {
			return err
		}
		// An archived space arrives archived (T-34); nothing below the top is.
		if top.Archived && !root.Private {
			if _, err := tx.Exec(ctx, "UPDATE objects SET archived = now() WHERE id = $1", m.ID); err != nil {
				return err
			}
			m.Archived = true
		}
		result.Top = m
		// The entries of rights, each holder by its name here (T-15); nothing private gets any.
		for _, a := range all {
			if len(a.n.Entries) == 0 {
				continue
			}
			if root.Private || IsDivider(a.n.Kind) {
				result.Skipped = append(result.Skipped, fmt.Sprintf("%s: its entries of rights were left out, as nothing private has any", a.n.Title))
				continue
			}
			for _, e := range a.n.Entries {
				if why := importEntry(ctx, tx, a.id, owner, e); why != "" {
					result.Skipped = append(result.Skipped, fmt.Sprintf("%s: the entry of %s was left out: %s", a.n.Title, holderName(e), why))
				}
			}
		}
		for _, a := range all {
			next := max(a.n.NextBlock, 1)
			for i, b := range a.n.Blocks {
				if !blockID.MatchString(b.ID) {
					return errors.New("block ID " + strconv.Quote(b.ID) + " is not valid")
				}
				body := relink(b.Body, ids)
				if err := checkBody(body); err != nil {
					return err
				}
				number, _ := strconv.Atoi(b.ID[1:])
				next = max(next, number+1)
				if !ValidAlign(b.Align) {
					return errors.New("block " + b.ID + ": alignment " + strconv.Quote(b.Align) + " is not center or right")
				}
				if _, err := tx.Exec(ctx, "INSERT INTO blocks (page, id, position, body, align) VALUES ($1, $2, $3, $4, nullif($5, ''))", a.id, b.ID, i, body, b.Align); err != nil {
					return err
				}
				if err := syncRefs(ctx, tx, a.id, b.ID, body); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, "INSERT INTO block_versions (page, block, body, author) VALUES ($1, $2, $3, $4)", a.id, b.ID, body, owner); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(ctx, "UPDATE objects SET next_block = $2 WHERE id = $1", a.id, next); err != nil {
				return err
			}
		}
		return withinStorage(ctx, tx)
	})
	return result, err
}

// importEntry writes one entry of rights of an import on the object it was made as, the holder found by its name here,
// and says why not where it cannot: no such person or group, or an entry that cannot be (an unknown action, an allow for
// Agents, who are only ever narrowed).
func importEntry(ctx context.Context, tx pgx.Tx, objectID, by string, e RightsEntry) string {
	who := ""
	switch e.Kind {
	case ToPerson:
		err := tx.QueryRow(ctx, "SELECT id FROM accounts WHERE login = $1 ORDER BY first_seen LIMIT 1", e.Name).Scan(&who)
		if errors.Is(err, pgx.ErrNoRows) {
			return "there is no such account here"
		} else if err != nil {
			return err.Error()
		}
	case ToGroup:
		// As everywhere a group is named (groupNamed): the local one, else the one taken over from the sign-in; none is
		// made, and an archived one gets no new entries.
		err := tx.QueryRow(ctx, `SELECT id FROM groups WHERE name = $1 AND archived IS NULL
			ORDER BY source = 'local' DESC LIMIT 1`, e.Name).Scan(&who)
		if errors.Is(err, pgx.ErrNoRows) {
			return "there is no such group here"
		} else if err != nil {
			return err.Error()
		}
	case ToTenant, ToAgents:
	default:
		return "no such kind of holder"
	}
	actions := map[string]string{}
	for _, list := range []struct {
		names []string
		value string
	}{{e.Allow, Allow}, {e.Deny, Deny}} {
		for _, a := range list.names {
			if !IsAction(a) || (e.Kind == ToAgents && list.value == Allow) {
				return "the action " + strconv.Quote(a) + " cannot be " + list.value + "ed there"
			}
			if _, twice := actions[a]; twice {
				return "the action " + strconv.Quote(a) + " is allowed and denied"
			}
			actions[a] = list.value
		}
	}
	keys := make([]string, 0, len(actions))
	for _, a := range Actions {
		if _, ok := actions[a]; ok {
			keys = append(keys, a)
		}
	}
	if len(keys) == 0 {
		return "it names no action"
	}
	if err := setEntries(ctx, tx, objectID, e.Kind, who, keys, actions, e.Subgroups || e.Kind != ToGroup, by); err != nil {
		return err.Error()
	}
	return ""
}

// holderName names the holder of an entry in a sentence.
func holderName(e RightsEntry) string {
	switch e.Kind {
	case ToTenant:
		return "everyone"
	case ToAgents:
		return "Agents"
	}
	return e.Kind + " " + strconv.Quote(e.Name)
}

var (
	objectLink = regexp.MustCompile(`(/o/)([^\s)#/"?<>]+)`)
	// Both the line ::embed and the inline :embed of a part (ADR-22).
	embedRef = regexp.MustCompile(`(:embed\{ref=")([^"#]+)`)
)

// relink points the links and embeddings of an imported block at the IDs its pages got: /o/<old> and ref="<old>" become
// the new ID wherever the old one, or the key of an imported page, is known. Everything else stays as written.
func relink(body string, ids map[string]string) string {
	for _, re := range []*regexp.Regexp{objectLink, embedRef} {
		body = re.ReplaceAllStringFunc(body, func(m string) string {
			parts := re.FindStringSubmatch(m)
			if id, ok := ids[parts[2]]; ok {
				return parts[1] + id
			}
			return m
		})
	}
	return body
}

// insertWithID inserts an object with the ID it brings if that is valid and not taken by anything, the trash included,
// and with a new one otherwise. Its position is the one it brings.
func insertWithID(ctx context.Context, tx pgx.Tx, o Object) (Object, error) {
	if n := len([]rune(o.Title)); n < 1 || n > 200 {
		return Object{}, ErrTitle
	}
	want := o.ID
	for attempt := 0; ; attempt++ {
		o.ID = want
		if attempt > 0 || !id.Valid(want) {
			o.ID = id.New()
		}
		made, err := scan(tx.QueryRow(ctx, `
			INSERT INTO objects (id, kind, title, description, owner, parent, position, inherit, private, tags, width, contents, template, area, page_kind)
			VALUES ($1, $2, $3, $4, $5, nullif($6, ''), $7, $8, $9, $10, nullif($11, ''), nullif($12, ''), $13,
				coalesce((SELECT area FROM objects WHERE id = nullif($6, '')), $14), $15)
			ON CONFLICT (tenant, id) DO NOTHING
			RETURNING `+columns, o.ID, o.Kind, o.Title, o.Description, o.Owner, o.Parent, o.Position, !(o.Breaks && o.Parent != ""), o.Private, tagsOf(o),
			o.Width, o.Contents, o.Template && o.Kind == KindPage, areaOf(o), kindOfPageIn(o)))
		if errors.Is(err, pgx.ErrNoRows) && attempt < 6 {
			continue
		}
		if err != nil {
			return made, err
		}
		access := importedAccess(o)
		if access == "" {
			return made, nil
		}
		if err := setGrant(ctx, tx, made.ID, ToTenant, "", access, o.Owner); err != nil {
			return Object{}, err
		}
		made.Access = access
		return made, nil
	}
}

// AccessNone marks in an export a shared collection at the top that grants the whole tenant nothing.
const AccessNone = "none"

// importedAccess is the role an imported object grants the whole tenant: what it brings; at the top of the shared tree
// edit where it brings nothing, as an export from before ADR-29 means; nothing on something private or a divider.
func importedAccess(o Object) string {
	switch {
	case o.Private || IsDivider(o.Kind) || o.Access == AccessNone:
		return ""
	case grantable(o.Access):
		return o.Access
	case o.Parent == "" && o.Access == "":
		return RoleEdit
	}
	return ""
}

// kindOfPageIn is the kind of a page an import keeps: only a page has one, and only a kind there is (T-20).
func kindOfPageIn(o Object) string {
	if o.Kind != KindPage || !pagekind.Valid(o.PageKind) {
		return ""
	}
	return o.PageKind
}

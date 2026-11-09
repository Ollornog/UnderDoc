// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"regexp"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/id"
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
	rows, err := db.pool.Query(ctx, `SELECT `+columns+`, next_block FROM objects o WHERE `+alive+` ORDER BY position, created, id`)
	if err != nil {
		return nil, nil, err
	}
	nodes, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*Node, error) {
		n := &Node{}
		o := &n.Object
		err := row.Scan(&o.ID, &o.Kind, &o.Title, &o.Description, &o.Owner, &o.Parent, &o.Position, &o.Access, &o.Private, &o.Created, &o.Changed, &o.Tags, &n.NextBlock)
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
	logins, err := db.logins(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, n := range nodes {
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
	rows, err := db.pool.Query(ctx, "SELECT page, id, body FROM blocks ORDER BY page, position")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var page string
		var b Block
		if err := rows.Scan(&page, &b.ID, &b.Body); err != nil {
			return err
		}
		if n := byID[page]; n != nil {
			n.Blocks = append(n.Blocks, b)
		}
	}
	return rows.Err()
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
	count := 0
	var read func(o Object) (*Node, error)
	read = func(o Object) (*Node, error) {
		if count++; count > exportLimit {
			return nil, ErrTooMany
		}
		n := &Node{Object: o}
		if o.Kind == KindPage {
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
		return n, nil
	}
	return read(o)
}

// Imported says what an import did.
type Imported struct {
	Top Object
	// Objects is how many objects were made, the top one included.
	Objects int
	// Renamed maps each ID that was taken or not valid to the new one it got instead.
	Renamed map[string]string
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
		result.Top = m
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
				if _, err := tx.Exec(ctx, "INSERT INTO blocks (page, id, position, body) VALUES ($1, $2, $3, $4)", a.id, b.ID, i, body); err != nil {
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
		return nil
	})
	return result, err
}

var (
	objectLink = regexp.MustCompile(`(/o/)([^\s)#/"?<>]+)`)
	embedRef   = regexp.MustCompile(`(::embed\{ref=")([^"#]+)`)
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
			INSERT INTO objects (id, kind, title, description, owner, parent, position, access, private, tags)
			VALUES ($1, $2, $3, $4, $5, nullif($6, ''), $7, nullif($8, ''), $9, $10)
			ON CONFLICT (id) DO NOTHING
			RETURNING `+columns, o.ID, o.Kind, o.Title, o.Description, o.Owner, o.Parent, o.Position, o.Access, o.Private, tagsOf(o)))
		if errors.Is(err, pgx.ErrNoRows) && attempt < 6 {
			continue
		}
		return made, err
	}
}

// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// Who reads what an agent writes (T-38). An agent cannot be kept from writing what it read where more people read it; it
// is made visible instead: the agent learns the readers of a target before and after it writes, its reads are kept for a
// while, and a write to where more people read than read what it read carries a mark until a person checks it.

// AgentReadWindow is how long a read of an agent counts as what it may carry into its next write: the token context of
// an agent, which UnderDoc does not see, taken as a time (T-38).
const AgentReadWindow = 2 * time.Hour

// maxAgentReads is how many of the latest reads are compared with a target.
const maxAgentReads = 50

// Readers is who may read an object.
type Readers struct {
	// IDs are the accounts that may read it, links they took included.
	IDs []string
	// Everyone tells that every account of the tenant may read it.
	Everyone bool
	// Private tells that it lies in private notes: only its owner reads it.
	Private bool
	// Links counts the live links that show it to anyone who has one (all but upload), on it or above it.
	Links int
	// Groups are the groups an entry of read on it or above it allows, with how many of their persons read it.
	Groups []GroupReaders
	// Others counts the readers in none of Groups.
	Others int
}

// GroupReaders is a group that reads an object, by name, with how many of its persons do.
type GroupReaders struct {
	Name  string
	Count int
}

// ReadersOf returns who may read an object.
func (db *DB) ReadersOf(ctx context.Context, objectID string) (Readers, error) {
	var r Readers
	var total int
	err := db.pool.QueryRow(ctx, `SELECT object_readers($1), (SELECT count(*) FROM accounts),
			(WITH RECURSIVE up AS (
				SELECT id, parent, 0 AS depth FROM objects WHERE id = $1
				UNION ALL SELECT o.id, o.parent, up.depth + 1 FROM objects o JOIN up ON o.id = up.parent WHERE up.depth < 200
			) SELECT count(*) FROM share_links l JOIN up ON up.id = l.object
			  WHERE l.mode <> 'upload' AND (l.expires IS NULL OR l.expires > now())
			    AND (object_decide(l.object, l.created_by, 'share', false, false)).allowed)`, objectID).Scan(&r.IDs, &total, &r.Links)
	if err != nil {
		return Readers{}, err
	}
	r.IDs = nonNil(r.IDs)
	r.Everyone = total > 0 && len(r.IDs) == total
	if r.Private, err = isPrivate(ctx, db.pool, objectID); err != nil {
		return Readers{}, err
	}
	covered := map[string]bool{}
	rows, err := db.pool.Query(ctx, `WITH RECURSIVE up AS (
			SELECT id, parent, 0 AS depth FROM objects WHERE id = $1
			UNION ALL SELECT o.id, o.parent, up.depth + 1 FROM objects o JOIN up ON o.id = up.parent WHERE up.depth < 200
		) SELECT g.name, CASE WHEN g.source = 'synced' THEN
				(SELECT coalesce(array_agg(a.id), '{}') FROM accounts a WHERE g.name = ANY (a.groups) AND a.id = ANY ($2::text[]))
			ELSE (SELECT coalesce(array_agg(m.account), '{}') FROM group_members m WHERE m.grp = g.id AND m.account = ANY ($2::text[])) END
		FROM groups g WHERE g.archived IS NULL AND g.id IN (
			SELECT gr.who FROM grants gr JOIN up ON up.id = gr.object WHERE gr.kind = 'group' AND gr.action = 'read' AND gr.value = 'allow')
		ORDER BY g.name`, objectID, r.IDs)
	if err != nil {
		return Readers{}, err
	}
	var name string
	var members []string
	if _, err := pgx.ForEachRow(rows, []any{&name, &members}, func() error {
		if len(members) == 0 {
			return nil
		}
		r.Groups = append(r.Groups, GroupReaders{Name: name, Count: len(members)})
		for _, m := range members {
			covered[m] = true
		}
		return nil
	}); err != nil {
		return Readers{}, err
	}
	slices.SortStableFunc(r.Groups, func(a, b GroupReaders) int { return b.Count - a.Count })
	for _, id := range r.IDs {
		if !covered[id] {
			r.Others++
		}
	}
	return r, nil
}

// maxReadersLine is about how long the line of readers is at most.
const maxReadersLine = 60

// Line says the readers in one short line for an agent (T-38): "readers: only you", "readers: everyone", or the groups
// with how many of their persons read, the others, and the links that show it, never names of persons.
func (r Readers) Line(viewer string) string {
	switch {
	case len(r.IDs) == 1 && r.IDs[0] == viewer && r.Links == 0:
		return "readers: only you"
	case r.Everyone && r.Links == 0:
		return "readers: everyone"
	}
	var parts []string
	if r.Everyone {
		parts = append(parts, "everyone")
	} else {
		for _, g := range r.Groups {
			parts = append(parts, fmt.Sprintf("%s (%d)", g.Name, g.Count))
		}
		if r.Others > 0 {
			word := "persons"
			if r.Others == 1 {
				word = "person"
			}
			if len(r.Groups) == 0 {
				parts = append(parts, fmt.Sprintf("%d %s", r.Others, word))
			} else {
				parts = append(parts, fmt.Sprintf("%d more %s", r.Others, word))
			}
		}
		if len(parts) == 0 {
			parts = append(parts, "nobody")
		}
	}
	if r.Links > 0 {
		word := "public links"
		if r.Links == 1 {
			word = "public link"
		}
		parts = append(parts, fmt.Sprintf("%d %s", r.Links, word))
	}
	line := "readers: " + strings.Join(parts, ", ")
	if utf8.RuneCountInString(line) <= maxReadersLine {
		return line
	}
	// Too long: the first groups, then how many more, the links always.
	tail := ""
	if r.Links > 0 {
		tail = ", " + parts[len(parts)-1]
		parts = parts[:len(parts)-1]
	}
	for n := len(parts) - 1; n >= 1; n-- {
		short := "readers: " + strings.Join(parts[:n], ", ") + fmt.Sprintf(", +%d more", len(parts)-n) + tail
		if utf8.RuneCountInString(short) <= maxReadersLine || n == 1 {
			return short
		}
	}
	return line
}

// SameReaders tells whether two sets of readers are the same accounts, with or without links that show to anyone.
func SameReaders(a, b Readers) bool {
	return slices.Equal(a.IDs, b.IDs) && (a.Links > 0) == (b.Links > 0)
}

// Wider tells whether more may read target than may read source: an account reads target that does not read source, or a
// live link shows target and none shows source.
func Wider(target, source Readers) bool {
	if target.Links > 0 && source.Links == 0 {
		return true
	}
	for _, id := range target.IDs {
		if !slices.Contains(source.IDs, id) {
			return true
		}
	}
	return false
}

// NoteAgentRead keeps that an agent of the account read objects through MCP, now (T-38).
func (db *DB) NoteAgentRead(ctx context.Context, accountID string, objects ...string) error {
	if len(objects) == 0 {
		return nil
	}
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "DELETE FROM agent_reads WHERE account = $1 AND at < now() - $2::interval", accountID, AgentReadWindow.String()); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO agent_reads (account, object) SELECT $1, o.id FROM objects o WHERE o.id = ANY ($2::text[])
			ON CONFLICT (tenant, account, object) DO UPDATE SET at = now()`, accountID, objects)
		return err
	})
}

// AgentReads returns what agents of the account read in the last AgentReadWindow, the latest first, at most
// maxAgentReads, leaving out except.
func (db *DB) AgentReads(ctx context.Context, accountID, except string) ([]string, error) {
	rows, err := db.pool.Query(ctx, `SELECT r.object FROM agent_reads r JOIN objects o ON o.id = r.object
		WHERE r.account = $1 AND r.at >= now() - $2::interval AND r.object <> $3 AND o.deleted_at IS NULL
		ORDER BY r.at DESC LIMIT $4`, accountID, AgentReadWindow.String(), except, maxAgentReads)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// AgentMarkOn tells whether a write of an agent to an object is marked when it read narrower content before (T-38): as the
// space it lies in says, or by default off on an instance with one person and on otherwise.
func (db *DB) AgentMarkOn(ctx context.Context, objectID string) (bool, error) {
	var set *bool
	var persons int
	err := db.pool.QueryRow(ctx, `SELECT (WITH RECURSIVE up AS (
				SELECT id, parent, agent_mark, 0 AS depth FROM objects WHERE id = $1
				UNION ALL SELECT o.id, o.parent, o.agent_mark, up.depth + 1 FROM objects o JOIN up ON o.id = up.parent WHERE up.depth < 200
			) SELECT agent_mark FROM up WHERE parent IS NULL),
			(SELECT count(*) FROM accounts WHERE NOT service)`, objectID).Scan(&set, &persons)
	if err != nil {
		return false, err
	}
	if set != nil {
		return *set, nil
	}
	return persons > 1, nil
}

// SetAgentMark sets the mark of a space (T-38): on, off, or nil for the default of the instance. Whoever manages the
// space, in the browser.
func (db *DB) SetAgentMark(ctx context.Context, spaceID string, actor Actor, mark *bool) error {
	if err := browserSession(ctx, actor.ID); err != nil {
		return err
	}
	o, err := db.forRights(ctx, spaceID, actor)
	if err != nil {
		return err
	}
	if o.Parent != "" || o.Kind != KindCollection {
		return ErrPlace
	}
	_, err = db.pool.Exec(ctx, "UPDATE objects SET agent_mark = $2 WHERE id = $1", o.ID, mark)
	return err
}

type widerKey struct{}

// WithWider marks the writes of a request as those of an agent that read narrower content before (T-38): its changes
// carry the mark in the hint of changes by agents, on every kind of page, until a person checks them.
func WithWider(ctx context.Context) context.Context { return context.WithValue(ctx, widerKey{}, true) }

func requestWider(ctx context.Context) bool {
	v, _ := ctx.Value(widerKey{}).(bool)
	return v
}

// BlockAuthor is who last changed a block: the account and its name, the person an agent worked for included.
type BlockAuthor struct {
	Block, Account, Name string
}

// BlockAuthors returns who last changed each block of a page, in the order of the page (T-38).
func (db *DB) BlockAuthors(ctx context.Context, pageID string) ([]BlockAuthor, error) {
	rows, err := db.pool.Query(ctx, `SELECT b.id, coalesce(v.author, ''), coalesce(a.name, v.author, '')
		FROM blocks b LEFT JOIN LATERAL (SELECT author FROM block_versions WHERE page = b.page AND block = b.id ORDER BY version DESC LIMIT 1) v ON true
		LEFT JOIN accounts a ON a.id = v.author WHERE b.page = $1 ORDER BY b.position`, pageID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (BlockAuthor, error) {
		var a BlockAuthor
		err := row.Scan(&a.Block, &a.Account, &a.Name)
		return a, err
	})
}

// AgentMarkOf returns how a space sets the mark (T-38): "on", "off" or "default", what the default of the instance is,
// and whether it is on.
func (db *DB) AgentMarkOf(ctx context.Context, spaceID string) (string, bool, bool, error) {
	var set *bool
	var persons int
	if err := db.pool.QueryRow(ctx, "SELECT agent_mark, (SELECT count(*) FROM accounts WHERE NOT service) FROM objects WHERE id = $1",
		spaceID).Scan(&set, &persons); err != nil {
		return "", false, false, err
	}
	def := persons > 1
	switch {
	case set == nil:
		return "default", def, def, nil
	case *set:
		return "on", def, true, nil
	}
	return "off", def, false, nil
}

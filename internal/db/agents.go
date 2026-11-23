// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// What the MCP access needs of the database (ADR-34). Every function decides with the one check of rights, as the
// browser does; the connection of a request through MCP carries that it is an agent's (account.User.MCP).

// RulesTag was what made the rules page until B-5. It stays reserved: no agent sets it or takes it away, so nothing an
// agent does can look like a rules page to a person or a tool that still goes by the tag.
const RulesTag = "rules"

// PageVersion is the newest change of the blocks of a page the viewer sees, 0 for a page nobody wrote yet. A write names
// it as its base, so that it fails when the page changed since it was read (ADR-34). Moving a block inside the page makes
// no version and changes nothing of its text, so it does not count.
func (db *DB) PageVersion(ctx context.Context, pageID, viewer string) (int64, error) {
	if _, err := db.page(ctx, pageID, viewer); err != nil {
		return 0, err
	}
	var version int64
	err := db.pool.QueryRow(ctx, "SELECT coalesce(max(version), 0) FROM block_versions WHERE page = $1", pageID).Scan(&version)
	return version, err
}

// RulesPage returns the rules page the viewer reads: the page an administrator named (B-5; until then the oldest page of
// the kind agent notes tagged rules, which an agent could take over). ErrNotFound when there is none, or the viewer may
// not read it, or it lies in the trash.
func (db *DB) RulesPage(ctx context.Context, viewer string) (Object, error) {
	o, err := scan(db.pool.QueryRow(ctx, `SELECT `+columns+` FROM objects o
		WHERE o.id = (SELECT rules_page FROM agent_settings) AND `+alive+` AND `+visible, viewer))
	if errors.Is(err, pgx.ErrNoRows) {
		return Object{}, ErrNotFound
	}
	return o, err
}

// RulesPageID returns the rules page as it is set, whoever may read it; "" for none.
func (db *DB) RulesPageID(ctx context.Context) (string, error) {
	var id string
	err := db.pool.QueryRow(ctx, "SELECT coalesce(rules_page, '') FROM agent_settings").Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// ErrRulesPageAgents is a change of an agent to the rules page or below it: agents read it only (B-5).
var ErrRulesPageAgents = fmt.Errorf("%w: agents only read the rules page; a person changes it in the browser", ErrNotAllowed)

// ErrRulesPage is a rules page that is no page the administrator reads, or one that is private or archived (B-6).
var ErrRulesPage = errors.New("the rules page is a page you read, neither private nor in an archived space")

// RulesPageFromTag tells whether the rules page is still the one migration 038 took over from the tag rules, which an
// administrator has not confirmed yet (B-6).
func (db *DB) RulesPageFromTag(ctx context.Context) (bool, error) {
	var fromTag bool
	err := db.pool.QueryRow(ctx, "SELECT from_tag AND rules_page IS NOT NULL FROM agent_settings").Scan(&fromTag)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return fromTag, err
}

// SetRulesPage names the rules page of the tenant, or none with "" (B-5). Only an administrator does, in the browser:
// agents read it and do nothing else with it or below it, the one check says so (object_decide, migration 038). The page
// is one the administrator reads; any kind, agent notes is the one meant for it.
func (db *DB) SetRulesPage(ctx context.Context, actor Actor, pageID string) error {
	if err := browserSession(ctx, actor.ID); err != nil {
		return err
	}
	if !actor.Admin {
		return ErrNotAllowed
	}
	if pageID != "" {
		o, err := db.Object(ctx, pageID, actor.ID)
		if err != nil {
			return err
		}
		if o.Kind != KindPage {
			return ErrRulesPage
		}
		// A private page is its owner's alone, and an archived one cannot be kept up to date (B-6).
		private, err := isPrivate(ctx, db.pool, o.ID)
		if err != nil {
			return err
		}
		archived, err := db.InArchive(ctx, o.ID)
		if err != nil {
			return err
		}
		if private || archived {
			return ErrRulesPage
		}
	}
	// Naming it, also the same page again, confirms it: it is no longer the one taken over from the tag.
	_, err := db.pool.Exec(ctx, `INSERT INTO agent_settings (rules_page, changed_by, from_tag) VALUES (nullif($1, ''), $2, false)
		ON CONFLICT (tenant) DO UPDATE SET rules_page = EXCLUDED.rules_page, changed_by = EXCLUDED.changed_by, changed = now(),
			from_tag = false`, pageID, actor.ID)
	return err
}

// UploadLinkTime is how long a one-time upload link holds (ADR-34).
const UploadLinkTime = 15 * time.Minute

// LinkUpload gives an upload of the viewer, which has received nothing yet, a one-time link and returns its token. Only
// the hash of the token is kept.
func (db *DB) LinkUpload(ctx context.Context, uploadID, viewer string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	tag, err := db.pool.Exec(ctx, `UPDATE uploads SET link_hash = $3, link_until = now() + make_interval(secs => $4)
		WHERE id = $1 AND owner = $2 AND received = 0`, uploadID, viewer, sum[:], UploadLinkTime.Seconds())
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrNotFound
	}
	return token, err
}

// TakeUploadLink finds the upload of a one-time link and takes the link away, in one step, so it works once; a link
// past its time is not found.
func (db *DB) TakeUploadLink(ctx context.Context, token string) (Upload, error) {
	sum := sha256.Sum256([]byte(token))
	var u Upload
	err := db.pool.QueryRow(ctx, `UPDATE uploads SET link_hash = NULL, link_until = NULL, touched = now()
		WHERE link_hash = $1 AND link_until > now() AND received = 0
		RETURNING id, owner, coalesce(parent, ''), coalesce(file, ''), coalesce(page, ''), coalesce(root, ''), name, size, received, touched`, sum[:]).
		Scan(&u.ID, &u.Owner, &u.Parent, &u.File, &u.Page, &u.Root, &u.Name, &u.Size, &u.Received, &u.Touched)
	if errors.Is(err, pgx.ErrNoRows) {
		return Upload{}, ErrNotFound
	}
	return u, err
}

// ErrPublish is a move or copy by an agent to where someone may read who may not read it where it is: publishing, which
// a person does in the browser (B-5, ADR-36).
var ErrPublish = fmt.Errorf("%w: an agent moves or copies nothing to where more people may read it than may read it now; a person does that in the browser", ErrNotAllowed)

// agentNoWider refuses, for a request of an agent, to put what sources hold below target when some account may read
// target but may not read one of the sources now (B-5): moving a private page into a shared collection publishes it.
// What lies below target inherits from it, and what the object decides itself it decides there as here, so nobody
// reads more than before when every reader of target reads the source already. A person may; the check counts persons,
// not agents.
//
// A copy keeps of an object below its top only the role it grants everyone, not its entries for persons or groups, its
// denies or a break of its inheritance, so every copied object is a source of its own (B-6). A move keeps them, so its
// top is enough. A live read link on target or above it serves target to anyone with the link: that counts as readers
// without limit, so only what such a link serves already goes there (B-6). A read link on what an agent moves that is
// dead where it lies, because its maker may not share it there, comes alive where its maker may share target: that
// counts too (B-6).
func (db *DB) agentNoWider(ctx context.Context, sources []string, target string, move bool) error {
	if !requestAgent(ctx) || target == "" || len(sources) == 0 {
		return nil
	}
	var wider bool
	if err := db.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM accounts a, unnest($1::text[]) s
			WHERE (object_decide($2, a.id, 'read', false, true)).allowed AND NOT (object_decide(s, a.id, 'read', false, true)).allowed)
		OR (object_public($2) AND EXISTS (SELECT 1 FROM unnest($1::text[]) s WHERE NOT object_public(s)))
		OR ($3 AND EXISTS (WITH RECURSIVE down AS (
				SELECT id FROM objects WHERE id = ANY ($1::text[])
				UNION ALL SELECT c.id FROM objects c JOIN down ON c.parent = down.id
			) SELECT 1 FROM share_links l JOIN down ON down.id = l.object
			WHERE l.mode = 'read' AND (l.expires IS NULL OR l.expires > now())
			  AND NOT (object_decide(l.object, l.created_by, 'share', false, false)).allowed
			  AND (object_decide($2, l.created_by, 'share', false, false)).allowed))`,
		sources, target, move).Scan(&wider); err != nil {
		return err
	}
	if wider {
		return ErrPublish
	}
	return nil
}

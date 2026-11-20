// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/id"
	"github.com/Ollornog/UnderDoc/internal/markdown"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// Passwords in the text (ADR-7). The text of a page holds only ::secret{id="…"}; a secret is a row of its own, bound to
// one page, with every value it had as a version sealed by the vault. The database never sees a value: the web hands in
// what seals and gets back what is sealed. Rights come from the page of the secret alone (secret_may, migration 028).

// SecretSealName is the name a version of a secret is sealed under. The vault binds it with the tenant, so a sealed value
// copied to another secret, version or tenant does not open.
func SecretSealName(secretID string, number int) string {
	return "page:" + secretID + ":" + strconv.Itoa(number)
}

// Seal seals a value under a name, for the tenant of the request; the web hands it in with the value inside.
type Seal func(name string) ([]byte, error)

// Actions of the log for secrets: each time, on the page of the secret, never with a value.
const (
	LogSecretMade     = "secret made"
	LogSecretRevealed = "secret revealed"
	LogSecretCopied   = "secret copied"
	LogSecretChanged  = "secret changed"
	LogSecretRestored = "secret restored"
	LogSecretReaders  = "secret readers"
	LogSecretMoved    = "secret moved"
	LogSecretDeleted  = "secret deleted"
)

// Errors of secrets.
var (
	ErrSecretHeld = errors.New("this block holds a password you may not reveal, so it stays on its page")
	ErrReader     = errors.New("a reader is a person or a group of this workspace, at most 200")
)

// PageSecret is a secret of a page as a viewer may know it: never its value.
type PageSecret struct {
	ID, Page   string
	Restricted bool
	// May tells that the viewer may reveal and copy it; MayChange that they may also change it, see its versions and
	// delete it (edit on the page); MayRestrict that they manage the page and set its readers.
	May, MayChange, MayRestrict bool
	// Referenced tells that a block of its page refers to it now.
	Referenced bool
	// The newest version, for whoever may reveal it.
	Version   int
	Changed   time.Time
	ChangedBy string
	// Readers, for a restricted secret, to whoever may reveal it or manages the page.
	Readers []SecretReader
	// Versions, newest first, to whoever may change it.
	Versions []SecretVersion
}

// SecretReader is a person or group that reveals a restricted secret.
type SecretReader struct {
	Kind, Who, Name string
}

// SecretVersion is one value a secret had: who set it, when, and where it came from.
type SecretVersion struct {
	Number int
	At     time.Time
	Author string
	Name   string
	Origin string
}

// SecretState is what rendering needs of a secret: its page, and whether the viewer may reveal it.
type SecretState struct {
	Page string
	May  bool
}

// logIn writes an entry of the log in the transaction of what it logs, so nothing is revealed unless it is logged.
// Nothing of a private page is logged, as for everything private (ADR-29).
func logIn(ctx context.Context, tx pgx.Tx, actor, action, page, detail string) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_log (actor, action, object, detail) SELECT $1, $2, $3, $4 WHERE NOT object_private($3)`,
		actor, action, page, detail)
	return err
}

// secretOf reads a secret for a viewer in a transaction, locked: its page, whether the viewer may reveal it, and their
// role on its page counted without links. A secret the viewer may not reveal is not found, the same as one that does not
// exist, unless they read its page and asked to see it anyway (see).
func secretOf(ctx context.Context, tx pgx.Tx, secretID, viewer string) (page string, may bool, role int, err error) {
	err = tx.QueryRow(ctx, `SELECT s.page, secret_may(s.id, $2), object_role(s.page, $2, false) FROM page_secrets s WHERE s.id = $1 FOR UPDATE OF s`,
		secretID, viewer).Scan(&page, &may, &role)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !may {
		return "", false, 0, ErrNotFound
	}
	return page, may, role, err
}

// CreatePageSecret makes a secret of a page, with its first value sealed by seal, for whoever may edit the page. It
// starts unrestricted: whoever reads the page reveals it. The text gets the reference from the editor.
func (db *DB) CreatePageSecret(ctx context.Context, pageID, viewer string, seal Seal) (string, error) {
	o, err := db.page(ctx, pageID, viewer)
	if err != nil {
		return "", err
	}
	if err := db.mayEdit(ctx, o, viewer); err != nil {
		return "", err
	}
	secretID := id.New()
	err = pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO page_secrets (id, page, created_by) VALUES ($1, $2, $3)", secretID, pageID, viewer); err != nil {
			return err
		}
		sealed, err := seal(SecretSealName(secretID, 1))
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO page_secret_versions (secret, number, sealed, author) VALUES ($1, 1, $2, $3)", secretID, sealed, viewer); err != nil {
			return err
		}
		return logIn(ctx, tx, viewer, LogSecretMade, pageID, "secret "+secretID)
	})
	if err != nil {
		return "", err
	}
	return secretID, nil
}

// RevealPageSecret returns a version of a secret, sealed under the name it returns, for whoever may reveal it, after the
// log took it down in the same transaction. number 0 is the newest; an older one needs edit on the page, as versions do
// (ADR-29). copied says the value goes to the clipboard, not on the screen; the log tells them apart.
func (db *DB) RevealPageSecret(ctx context.Context, secretID, viewer string, number int, copied bool) (sealed []byte, name string, err error) {
	err = pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		page, _, role, err := secretOf(ctx, tx, secretID, viewer)
		if err != nil {
			return err
		}
		if number != 0 && role < Rank(RoleEdit) {
			return ErrNotAllowed
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT number, sealed FROM page_secret_versions WHERE secret = $1 AND ($2 = 0 OR number = $2)
			ORDER BY number DESC LIMIT 1`, secretID, number).Scan(&n, &sealed); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		action, detail := LogSecretRevealed, "secret "+secretID
		if copied {
			action = LogSecretCopied
		}
		if number != 0 {
			detail += " version " + strconv.Itoa(n)
		}
		name = SecretSealName(secretID, n)
		return logIn(ctx, tx, viewer, action, page, detail)
	})
	if err != nil {
		return nil, "", err
	}
	return sealed, name, nil
}

// changeSecret checks that the viewer may change a secret: reveal it and edit its page.
func changeSecret(ctx context.Context, tx pgx.Tx, secretID, viewer string) (string, error) {
	page, _, role, err := secretOf(ctx, tx, secretID, viewer)
	if err != nil {
		return "", err
	}
	if role < Rank(RoleEdit) {
		return "", ErrNotAllowed
	}
	return page, nil
}

// addSecretVersion puts a new version of a secret, sealed by seal, after the newest.
func addSecretVersion(ctx context.Context, tx pgx.Tx, secretID, viewer, origin string, seal func(name string) ([]byte, error)) (int, error) {
	var n int
	if err := tx.QueryRow(ctx, "SELECT coalesce(max(number), 0) + 1 FROM page_secret_versions WHERE secret = $1", secretID).Scan(&n); err != nil {
		return 0, err
	}
	sealed, err := seal(SecretSealName(secretID, n))
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec(ctx, "INSERT INTO page_secret_versions (secret, number, sealed, author, origin) VALUES ($1, $2, $3, $4, $5)",
		secretID, n, sealed, viewer, origin)
	return n, err
}

// ChangePageSecret gives a secret a new value, as a new version, for whoever may reveal it and edit its page.
func (db *DB) ChangePageSecret(ctx context.Context, secretID, viewer string, seal Seal) error {
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		page, err := changeSecret(ctx, tx, secretID, viewer)
		if err != nil {
			return err
		}
		n, err := addSecretVersion(ctx, tx, secretID, viewer, "", seal)
		if err != nil {
			return err
		}
		return logIn(ctx, tx, viewer, LogSecretChanged, page, "secret "+secretID+" version "+strconv.Itoa(n))
	})
}

// RestorePageSecret makes an older version the newest again, as a new version: reseal opens the old one under its name
// and seals its value under the new name.
func (db *DB) RestorePageSecret(ctx context.Context, secretID, viewer string, number int,
	reseal func(oldName string, old []byte, newName string) ([]byte, error)) error {
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		page, err := changeSecret(ctx, tx, secretID, viewer)
		if err != nil {
			return err
		}
		var old []byte
		if err := tx.QueryRow(ctx, "SELECT sealed FROM page_secret_versions WHERE secret = $1 AND number = $2", secretID, number).Scan(&old); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		origin := "restored from version " + strconv.Itoa(number)
		n, err := addSecretVersion(ctx, tx, secretID, viewer, origin, func(name string) ([]byte, error) {
			return reseal(SecretSealName(secretID, number), old, name)
		})
		if err != nil {
			return err
		}
		return logIn(ctx, tx, viewer, LogSecretRestored, page, "secret "+secretID+" version "+strconv.Itoa(n)+" "+origin)
	})
}

// DeletePageSecret deletes a secret with every version, for whoever may change it. A reference left in the text shows
// "not available here" from then on.
func (db *DB) DeletePageSecret(ctx context.Context, secretID, viewer string) error {
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		page, err := changeSecret(ctx, tx, secretID, viewer)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM page_secrets WHERE id = $1", secretID); err != nil {
			return err
		}
		return logIn(ctx, tx, viewer, LogSecretDeleted, page, "secret "+secretID)
	})
}

// SetPageSecretReaders restricts a secret to persons and groups, or with restricted false lets everyone who reads its
// page reveal it again, for whoever manages the page. There is only narrowing: a reader also has to read the page.
func (db *DB) SetPageSecretReaders(ctx context.Context, secretID, viewer string, restricted bool, readers []SecretReader) error {
	if len(readers) > 200 || !restricted && len(readers) > 0 {
		return ErrReader
	}
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		var page string
		var role int
		err := tx.QueryRow(ctx, `SELECT s.page, object_role(s.page, $2, false) FROM page_secrets s JOIN objects o ON o.id = s.page
			WHERE s.id = $1 AND o.deleted_at IS NULL FOR UPDATE OF s`, secretID, viewer).Scan(&page, &role)
		if errors.Is(err, pgx.ErrNoRows) || err == nil && role == 0 {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if role < Rank(RoleManage) {
			return ErrNotAllowed
		}
		var named []string
		seen := map[string]bool{}
		for _, r := range readers {
			var known bool
			switch r.Kind {
			case "person":
				err = tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM accounts WHERE id = $1)", r.Who).Scan(&known)
			case "group":
				err = tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM groups WHERE id = $1)", r.Who).Scan(&known)
			}
			if err != nil {
				return err
			}
			if !known {
				return ErrReader
			}
			if key := r.Kind + ":" + r.Who; !seen[key] {
				seen[key] = true
				named = append(named, key)
			}
		}
		if _, err := tx.Exec(ctx, "DELETE FROM page_secret_readers WHERE secret = $1", secretID); err != nil {
			return err
		}
		for _, key := range named {
			kind, who, _ := strings.Cut(key, ":")
			if _, err := tx.Exec(ctx, "INSERT INTO page_secret_readers (secret, kind, who) VALUES ($1, $2, $3)", secretID, kind, who); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, "UPDATE page_secrets SET restricted = $2 WHERE id = $1", secretID, restricted); err != nil {
			return err
		}
		detail := "secret " + secretID + ": whoever reads the page"
		if restricted {
			detail = "secret " + secretID + ": only " + strings.Join(named, ", ")
			if len(named) == 0 {
				detail = "secret " + secretID + ": nobody"
			}
		}
		return logIn(ctx, tx, viewer, LogSecretReaders, page, detail)
	})
}

const secretColumns = `s.id, s.page, s.restricted, secret_may(s.id, $1), object_role(s.page, $1, false),
	EXISTS (SELECT 1 FROM blocks b WHERE b.page = s.page AND strpos(b.body, '::secret{id="' || s.id || '"}') > 0),
	v.number, v.at, v.author`

func scanSecret(row pgx.Row) (PageSecret, error) {
	var s PageSecret
	var role int
	err := row.Scan(&s.ID, &s.Page, &s.Restricted, &s.May, &role, &s.Referenced, &s.Version, &s.Changed, &s.ChangedBy)
	s.MayChange = s.May && role >= Rank(RoleEdit)
	s.MayRestrict = role >= Rank(RoleManage)
	if !s.May {
		s.Version, s.Changed, s.ChangedBy = 0, time.Time{}, ""
	}
	return s, err
}

const secretFrom = `FROM page_secrets s JOIN LATERAL (SELECT number, at, author FROM page_secret_versions
	WHERE secret = s.id ORDER BY number DESC LIMIT 1) v ON true`

// PageSecrets lists the secrets of a page the viewer sees, oldest first, with what they may do with each.
func (db *DB) PageSecrets(ctx context.Context, pageID, viewer string) ([]PageSecret, error) {
	if _, err := db.page(ctx, pageID, viewer); err != nil {
		return nil, err
	}
	rows, err := db.pool.Query(ctx, `SELECT `+secretColumns+` `+secretFrom+` WHERE s.page = $2 ORDER BY s.created, s.id`, viewer, pageID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (PageSecret, error) { return scanSecret(row) })
}

// PageSecretOf returns one secret for a viewer who sees its page, with its readers and versions as far as they may know
// them. A secret on a page the viewer does not see is not found.
func (db *DB) PageSecretOf(ctx context.Context, secretID, viewer string) (PageSecret, error) {
	s, err := scanSecret(db.pool.QueryRow(ctx, `SELECT `+secretColumns+` `+secretFrom+` WHERE s.id = $2`, viewer, secretID))
	if errors.Is(err, pgx.ErrNoRows) {
		return PageSecret{}, ErrNotFound
	}
	if err != nil {
		return PageSecret{}, err
	}
	if _, err := db.page(ctx, s.Page, viewer); err != nil {
		return PageSecret{}, ErrNotFound
	}
	if s.Restricted && (s.May || s.MayRestrict) {
		rows, err := db.pool.Query(ctx, `SELECT r.kind, r.who, coalesce(a.name, g.name, r.who) FROM page_secret_readers r
			LEFT JOIN accounts a ON r.kind = 'person' AND a.id = r.who LEFT JOIN groups g ON r.kind = 'group' AND g.id = r.who
			WHERE r.secret = $1 ORDER BY r.kind, 3`, secretID)
		if err != nil {
			return PageSecret{}, err
		}
		if s.Readers, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (SecretReader, error) {
			var r SecretReader
			err := row.Scan(&r.Kind, &r.Who, &r.Name)
			return r, err
		}); err != nil {
			return PageSecret{}, err
		}
	}
	if s.MayChange {
		rows, err := db.pool.Query(ctx, `SELECT v.number, v.at, v.author, coalesce(a.name, v.author), v.origin FROM page_secret_versions v
			LEFT JOIN accounts a ON a.id = v.author WHERE v.secret = $1 ORDER BY v.number DESC`, secretID)
		if err != nil {
			return PageSecret{}, err
		}
		if s.Versions, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (SecretVersion, error) {
			var v SecretVersion
			err := row.Scan(&v.Number, &v.At, &v.Author, &v.Name, &v.Origin)
			return v, err
		}); err != nil {
			return PageSecret{}, err
		}
	}
	return s, nil
}

// SecretStates tells, for the IDs a rendered block refers to, the page of each secret that exists and whether the viewer
// may reveal it. An ID without a secret is left out. Rendering shows a secret only in a block of its own page.
func (db *DB) SecretStates(ctx context.Context, viewer string, ids []string) (map[string]SecretState, error) {
	states := map[string]SecretState{}
	if len(ids) == 0 {
		return states, nil
	}
	rows, err := db.pool.Query(ctx, "SELECT s.id, s.page, secret_may(s.id, $1) FROM page_secrets s WHERE s.id = ANY ($2)", viewer, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var secretID string
		var s SecretState
		if err := rows.Scan(&secretID, &s.Page, &s.May); err != nil {
			return nil, err
		}
		states[secretID] = s
	}
	return states, rows.Err()
}

// moveSecrets takes the secrets of a page that a block refers to along to another page, when the block moves there
// (MoveBlockTo), if the viewer may reveal each; otherwise the move is refused (ErrSecretHeld). References to secrets of
// other pages stay what they are.
func moveSecrets(ctx context.Context, tx pgx.Tx, from, to, body, viewer string) error {
	for _, secretID := range markdown.SecretIDs(body) {
		var may bool
		err := tx.QueryRow(ctx, "SELECT secret_may(id, $3) FROM page_secrets WHERE id = $1 AND page = $2 FOR UPDATE", secretID, from, viewer).Scan(&may)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if !may {
			return ErrSecretHeld
		}
		if _, err := tx.Exec(ctx, "UPDATE page_secrets SET page = $2 WHERE id = $1", secretID, to); err != nil {
			return err
		}
		if err := logIn(ctx, tx, viewer, LogSecretMoved, to, "secret "+secretID+" from "+from); err != nil {
			return err
		}
	}
	return nil
}

// TOTPSecretName is the name the TOTP secret of an account is sealed under.
func TOTPSecretName(accountID string) string { return "totp:" + accountID }

// Rekey seals again every sealed value of a tenant (ADR-7): the secrets of pages with every version, the TOTP secrets,
// the secrets of other systems such as the client secret of OIDC, and the NT hashes of Samba. reseal gets the name a
// value is bound to and what is sealed, and returns it sealed under the new key, or nil to leave it as it is. All in one
// transaction per tenant; it returns how many values changed.
func (db *DB) Rekey(ctx context.Context, tenantID string, reseal func(name string, sealed []byte) ([]byte, error)) (int, error) {
	ctx = tenant.With(ctx, tenantID)
	changed := 0
	err := pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		type sealedRow struct {
			name   string
			sealed []byte
			update func(sealed []byte) error
		}
		var all []sealedRow
		collect := func(sql string, each func(rows pgx.Rows) (sealedRow, error)) error {
			rows, err := tx.Query(ctx, sql)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				r, err := each(rows)
				if err != nil {
					return err
				}
				all = append(all, r)
			}
			return rows.Err()
		}
		if err := collect("SELECT secret, number, sealed FROM page_secret_versions FOR UPDATE", func(rows pgx.Rows) (sealedRow, error) {
			var secretID string
			var n int
			var sealed []byte
			err := rows.Scan(&secretID, &n, &sealed)
			return sealedRow{SecretSealName(secretID, n), sealed, func(s []byte) error {
				_, err := tx.Exec(ctx, "UPDATE page_secret_versions SET sealed = $3 WHERE secret = $1 AND number = $2", secretID, n, s)
				return err
			}}, err
		}); err != nil {
			return err
		}
		if err := collect("SELECT id, totp_sealed FROM accounts WHERE totp_sealed IS NOT NULL FOR UPDATE", func(rows pgx.Rows) (sealedRow, error) {
			var accountID string
			var sealed []byte
			err := rows.Scan(&accountID, &sealed)
			return sealedRow{TOTPSecretName(accountID), sealed, func(s []byte) error {
				_, err := tx.Exec(ctx, "UPDATE accounts SET totp_sealed = $2 WHERE id = $1", accountID, s)
				return err
			}}, err
		}); err != nil {
			return err
		}
		if err := collect("SELECT name, sealed FROM secrets FOR UPDATE", func(rows pgx.Rows) (sealedRow, error) {
			var name string
			var sealed []byte
			err := rows.Scan(&name, &sealed)
			return sealedRow{name, sealed, func(s []byte) error {
				_, err := tx.Exec(ctx, "UPDATE secrets SET sealed = $2 WHERE name = $1", name, s)
				return err
			}}, err
		}); err != nil {
			return err
		}
		if err := collect("SELECT account, nt_hash FROM samba_accounts FOR UPDATE", func(rows pgx.Rows) (sealedRow, error) {
			var accountID string
			var sealed []byte
			err := rows.Scan(&accountID, &sealed)
			return sealedRow{SambaSecretName(accountID), sealed, func(s []byte) error {
				_, err := tx.Exec(ctx, "UPDATE samba_accounts SET nt_hash = $2 WHERE account = $1", accountID, s)
				return err
			}}, err
		}); err != nil {
			return err
		}
		for _, r := range all {
			again, err := reseal(r.name, r.sealed)
			if err != nil {
				return errors.New("a sealed value of " + tenantID + " (" + strings.SplitN(r.name, ":", 2)[0] + ") opens with neither key")
			}
			if again == nil {
				continue
			}
			if err := r.update(again); err != nil {
				return err
			}
			changed++
		}
		return nil
	})
	return changed, err
}

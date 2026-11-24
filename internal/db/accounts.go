// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/account"
)

// Accounts (ADR-32): every person comes through the gate and is bound by the lasting ID of their account there. Accounts
// made before keep where they came from, as their source. Everything here belongs to the tenant of ctx.

// Sources of accounts: the gate, and those of the sign-in built in before ADR-32, which wait for their binding.
const (
	SourceGate  = "gate"
	SourceOIDC  = "oidc"
	SourceLocal = "local"
)

// Errors about accounts.
var (
	ErrSource = errors.New("this account came in another way")
	// ErrBound is a lasting ID of the gate bound to another account already, or an account bound to another ID.
	ErrBound = errors.New("bound already")
	// ErrAmbiguous is a login that several accounts have: the key has to name one.
	ErrAmbiguous = errors.New("several accounts have this login: name one by its key")
	// ErrRemoteID is no lasting ID of the gate: those are numbers.
	ErrRemoteID = errors.New("the lasting ID of the gate is a number")
)

var remoteIDPattern = regexp.MustCompile(`^[0-9]{1,30}$`)

// Account is an account with where it comes from, whether it is marked as an administrator (the emergency access) and
// whether UnderDoc locked it (T-33).
type Account struct {
	account.User
	Source  string
	Admin   bool
	Service bool
	Locked  bool
}

const accountColumns = `id, login, name, email, groups, source, admin, coalesce(remote_id, ''), gate_kind, service, locked IS NOT NULL`

func scanAccount(row pgx.Row) (Account, error) {
	var a Account
	err := row.Scan(&a.ID, &a.Login, &a.Name, &a.Email, &a.Groups, &a.Source, &a.Admin, &a.RemoteID, &a.GateKind, &a.Service, &a.Locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	return a, err
}

// AccountByID returns an account of the tenant.
func (db *DB) AccountByID(ctx context.Context, accountID string) (Account, error) {
	return scanAccount(db.pool.QueryRow(ctx, `SELECT `+accountColumns+` FROM accounts WHERE id = $1`, accountID))
}

// Accounts returns every account of the tenant, by login.
func (db *DB) Accounts(ctx context.Context) ([]Account, error) {
	rows, err := db.pool.Query(ctx, `SELECT `+accountColumns+` FROM accounts ORDER BY lower(login), id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Account, error) { return scanAccount(row) })
}

// Bind binds an account made before ADR-32, named by its key or its login, to the lasting ID of its person in the gate
// (`underdoc account bind`). Its next request through the gate comes into it, with everything it owns. An ID bound to
// another account, or an account bound to another ID, is refused, so nobody takes over someone else's account.
func (db *DB) Bind(ctx context.Context, accountRef, remoteID string) (Account, error) {
	if !remoteIDPattern.MatchString(remoteID) {
		return Account{}, ErrRemoteID
	}
	var a Account
	err := pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		var err error
		a, err = scanAccount(tx.QueryRow(ctx, `SELECT `+accountColumns+` FROM accounts WHERE id = $1 FOR UPDATE`, accountRef))
		if errors.Is(err, ErrNotFound) {
			rows, qerr := tx.Query(ctx, `SELECT `+accountColumns+` FROM accounts WHERE lower(login) = lower($1) FOR UPDATE`, accountRef)
			if qerr != nil {
				return qerr
			}
			found, cerr := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Account, error) { return scanAccount(row) })
			switch {
			case cerr != nil:
				return cerr
			case len(found) == 0:
				return ErrNotFound
			case len(found) > 1:
				return ErrAmbiguous
			}
			a, err = found[0], nil
		}
		if err != nil {
			return err
		}
		if a.RemoteID == remoteID {
			return nil
		}
		if a.RemoteID != "" {
			return ErrBound
		}
		tag, err := tx.Exec(ctx, `UPDATE accounts SET remote_id = $2 WHERE id = $1
			AND NOT EXISTS (SELECT 1 FROM accounts WHERE remote_id = $2)`, a.ID, remoteID)
		if err == nil && tag.RowsAffected() == 0 {
			return ErrBound
		}
		a.RemoteID = remoteID
		return err
	})
	return a, err
}

// MarkAdmin marks the account bound to a lasting ID of the gate as an administrator, or takes the mark back: the
// emergency access on the command line (ADR-32), for a gate that lost the role of the app. An account that does not
// exist yet is made, bound to that ID, so the mark holds from its first request.
func (db *DB) MarkAdmin(ctx context.Context, remoteID string, admin bool) (Account, error) {
	if !remoteIDPattern.MatchString(remoteID) {
		return Account{}, ErrRemoteID
	}
	if admin {
		if _, err := db.BindGate(ctx, account.User{RemoteID: remoteID, Login: remoteID, Name: remoteID}); err != nil {
			return Account{}, err
		}
	}
	a, err := scanAccount(db.pool.QueryRow(ctx, `UPDATE accounts SET admin = $2 WHERE remote_id = $1 RETURNING `+accountColumns, remoteID, admin))
	return a, err
}

// AccountByRemoteID returns the account bound to a lasting ID of the gate.
func (db *DB) AccountByRemoteID(ctx context.Context, remoteID string) (Account, error) {
	return scanAccount(db.pool.QueryRow(ctx, `SELECT `+accountColumns+` FROM accounts WHERE remote_id = $1`, remoteID))
}

// ErrLogin is a login or a name of an account that is empty, too long or carries a control character.
var ErrLogin = errors.New("a login and a name have 1 to 200 characters and no control characters")

// AddAccount makes an account bound to a lasting ID of the gate before its first sign-in, without an administrator's
// mark (T-33): `underdoc account add` for an import, an invitation through the gate's account API, and an account the
// gate's operator handed to UnderDoc (adopt). Its person comes into it at their first sign-in, with what was prepared
// for them. Accounts are only ever bound by that ID: an account under the same login or name stays another one. An ID
// that is bound already keeps its account as it is; created says whether one was made.
func (db *DB) AddAccount(ctx context.Context, remoteID, login, name, email string) (Account, bool, error) {
	if !remoteIDPattern.MatchString(remoteID) {
		return Account{}, false, ErrRemoteID
	}
	login, name, email = strings.TrimSpace(login), strings.TrimSpace(name), strings.TrimSpace(email)
	if name == "" {
		name = login
	}
	for _, s := range []string{login, name} {
		if n := utf8.RuneCountInString(s); n < 1 || n > 200 || strings.ContainsFunc(s, unicode.IsControl) {
			return Account{}, false, ErrLogin
		}
	}
	if utf8.RuneCountInString(email) > 320 || strings.ContainsFunc(email, unicode.IsControl) {
		return Account{}, false, ErrLogin
	}
	if a, err := db.AccountByRemoteID(ctx, remoteID); err == nil {
		return a, false, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Account{}, false, err
	}
	// Made as the first request of the person would make it, but as nobody's session: no groups of the gate are taken
	// over before the person brings them, and no mark of an administrator is set.
	a, err := db.BindGate(ctx, account.User{RemoteID: remoteID, Login: login, Name: name, Email: email, Kind: account.KindPerson})
	if err != nil {
		return Account{}, false, err
	}
	return a, true, nil
}

// SetLocked locks an account in UnderDoc, or unlocks it (T-33): locked, it comes in through no way and may do nothing
// (migration 039). by is the account that did it, kept with the lock.
func (db *DB) SetLocked(ctx context.Context, accountID string, locked bool, by string) (Account, error) {
	if locked {
		return scanAccount(db.pool.QueryRow(ctx, `UPDATE accounts SET locked = coalesce(locked, now()), locked_by = $2
			WHERE id = $1 RETURNING `+accountColumns, accountID, by))
	}
	return scanAccount(db.pool.QueryRow(ctx, `UPDATE accounts SET locked = NULL, locked_by = '' WHERE id = $1
		RETURNING `+accountColumns, accountID))
}

// Managed is how a group admin may manage the account of a person (B-7).
type Managed struct {
	// Manages tells that they may manage it at all: the person is in a group they administer and in no other local
	// group, or they invited the person.
	Manages bool
	// Invited tells that they invited the person: only then do they give them a new sign-in link.
	Invited bool
}

// MayManagePerson tells how an account manages the account of a person as a group admin (T-33, B-7): the person is in a
// local group the actor administers (directly, or in a group inside one, none on the way archived) and in no local group
// they do not administer, or the actor invited them. A group admin can put any account into their group, so being in it
// alone gives them nothing over the person; the groups the gate names are its roles and its groups, which the gate
// manages, and do not count here.
func (db *DB) MayManagePerson(ctx context.Context, actorID, personID string) (Managed, error) {
	mine, err := db.AdministeredGroups(ctx, actorID)
	if err != nil || len(mine) == 0 {
		return Managed{}, err
	}
	var inMine, onlyMine, invited bool
	err = db.pool.QueryRow(ctx, `SELECT
			EXISTS (SELECT 1 FROM group_members m JOIN groups g ON g.id = m.grp
				WHERE m.account = $1 AND g.archived IS NULL AND m.grp = ANY ($2::text[])),
			NOT EXISTS (SELECT 1 FROM group_members m WHERE m.account = $1 AND NOT m.grp = ANY ($2::text[])),
			coalesce((SELECT invited_by = $3 AND $3 <> '' FROM accounts WHERE id = $1), false)`,
		personID, mine, actorID).Scan(&inMine, &onlyMine, &invited)
	if err != nil {
		return Managed{}, err
	}
	return Managed{Manages: invited || inMine && onlyMine, Invited: invited}, nil
}

// MarkInvited keeps who invited an account, once: the account made for an invitation (B-7).
func (db *DB) MarkInvited(ctx context.Context, accountID, by string) error {
	_, err := db.pool.Exec(ctx, "UPDATE accounts SET invited_by = $2 WHERE id = $1 AND invited_by = ''", accountID, by)
	return err
}

// RemoveFromMyGroups takes a person out of every group the actor administers that they are directly in, and returns
// how many: what a lock by a group admin does (B-7), since the account itself is not theirs to lock.
func (db *DB) RemoveFromMyGroups(ctx context.Context, actor Actor, personID string) (int, error) {
	if err := noAgent(ctx); err != nil {
		return 0, err
	}
	mine, err := db.AdministeredGroups(ctx, actor.ID)
	if err != nil {
		return 0, err
	}
	tag, err := db.pool.Exec(ctx, `DELETE FROM group_members m USING groups g
		WHERE g.id = m.grp AND g.source = 'local' AND m.account = $1 AND m.grp = ANY ($2::text[])`, personID, mine)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// seeExternal records a user under the ID it carries, never under the key or the login of an account made locally before.
func (db *DB) seeExternal(ctx context.Context, user account.User, source string) error {
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		// The login of a local account is its own: nobody from outside comes in under it.
		var local bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM accounts WHERE source = 'local' AND (lower(login) = lower($2) OR id = $1))",
			user.ID, user.Login).Scan(&local); err != nil {
			return err
		}
		if local {
			return ErrSource
		}
		var known bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM accounts WHERE id = $1)", user.ID).Scan(&known); err != nil {
			return err
		}
		if !known {
			if err := roomForUser(ctx, tx); err != nil {
				return err
			}
		}
		var inserted bool
		err := tx.QueryRow(ctx, `
			INSERT INTO accounts (id, login, name, email, groups, source) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (tenant, id) DO UPDATE SET login = EXCLUDED.login, name = EXCLUDED.name, email = EXCLUDED.email,
				groups = EXCLUDED.groups, source = EXCLUDED.source, last_seen = now()
			WHERE accounts.source <> 'local'
			RETURNING (xmax = 0)`, user.ID, user.Login, user.Name, user.Email, nonNil(user.Groups), source).Scan(&inserted)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSource
		}
		if err != nil {
			return err
		}
		// The groups the sign-in names are groups taken over (ADR-29).
		if err := syncGroups(ctx, tx, user.Groups); err != nil || !inserted {
			return err
		}
		_, err = insertObject(ctx, tx, Object{Kind: KindCollection, Title: MyNotes, Owner: user.ID, Private: true})
		return err
	})
}

// CountAccounts returns how many accounts the tenant has.
func (db *DB) CountAccounts(ctx context.Context) (int, error) {
	var n int
	err := db.pool.QueryRow(ctx, "SELECT count(*) FROM accounts").Scan(&n)
	return n, err
}

// --- Secrets of other systems, sealed by the vault (docs/sign-in.md).

var secretName = regexp.MustCompile(`^[a-z0-9_.:-]{1,100}$`)

// PutSealedSecret keeps a sealed value under its name.
func (db *DB) PutSealedSecret(ctx context.Context, name string, sealed []byte) error {
	if !secretName.MatchString(name) {
		return errors.New("not a name of a secret")
	}
	_, err := db.pool.Exec(ctx, `INSERT INTO secrets (name, sealed) VALUES ($1, $2)
		ON CONFLICT (tenant, name) DO UPDATE SET sealed = EXCLUDED.sealed, changed = now()`, name, sealed)
	return err
}

// SealedSecret returns the sealed value of a name.
func (db *DB) SealedSecret(ctx context.Context, name string) ([]byte, error) {
	var sealed []byte
	err := db.pool.QueryRow(ctx, "SELECT sealed FROM secrets WHERE name = $1", name).Scan(&sealed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return sealed, err
}

// DeleteSecret removes a secret.
func (db *DB) DeleteSecret(ctx context.Context, name string) error {
	_, err := db.pool.Exec(ctx, "DELETE FROM secrets WHERE name = $1", name)
	return err
}

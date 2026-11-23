// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"regexp"

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

// Account is an account with where it comes from and whether it is marked as an administrator (the emergency access).
type Account struct {
	account.User
	Source string
	Admin  bool
}

const accountColumns = `id, login, name, email, groups, source, admin, coalesce(remote_id, ''), gate_kind`

func scanAccount(row pgx.Row) (Account, error) {
	var a Account
	err := row.Scan(&a.ID, &a.Login, &a.Name, &a.Email, &a.Groups, &a.Source, &a.Admin, &a.RemoteID, &a.GateKind)
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

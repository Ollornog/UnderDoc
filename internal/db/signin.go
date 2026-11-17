// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/id"
)

// Sign-in built in (ADR-23). Everything here belongs to the tenant of ctx, as everything else does.

// Ways in.
const (
	MethodPasskey      = "passkey"
	MethodPasswordTOTP = "password_totp"
	MethodPassword     = "password"
	MethodOIDC         = "oidc"
	MethodGate         = "gate"
)

// Methods are all ways in, in the order the sign-in page offers them.
var Methods = []string{MethodPasskey, MethodPasswordTOTP, MethodPassword, MethodOIDC, MethodGate}

// Sources of accounts.
const (
	SourceGate  = "gate"
	SourceOIDC  = "oidc"
	SourceLocal = "local"
)

// Errors about signing in.
var (
	ErrNoMethod   = errors.New("at least one way to sign in")
	ErrMethod     = errors.New("unknown way to sign in")
	ErrLogin      = errors.New("a login has 1 to 63 lower case letters, digits, dots, hyphens and underscores, and starts with a letter or digit")
	ErrLoginTaken = errors.New("this login is taken")
	ErrOIDC       = errors.New("OIDC needs an issuer with https and a client ID")
	ErrSource     = errors.New("this account signs in another way")
)

var loginPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

// ValidLogin tells whether s can be the login of a local account.
func ValidLogin(s string) bool { return loginPattern.MatchString(s) }

// SignIn is how a tenant lets people in.
type SignIn struct {
	Methods         []string
	Registration    bool
	OIDCName        string
	OIDCIssuer      string
	OIDCClientID    string
	OIDCGroupsClaim string
	// AdminGroup names the group of the gate or of OIDC whose members administer the tenant; empty for the default.
	AdminGroup string
}

// Has tells whether a way in is switched on.
func (s SignIn) Has(method string) bool { return slices.Contains(s.Methods, method) }

// DefaultSignIn is how a tenant without settings lets people in: with a passkey, and nobody registers.
var DefaultSignIn = SignIn{Methods: []string{MethodPasskey}, OIDCGroupsClaim: "groups"}

// SignIn returns the settings of the tenant, or DefaultSignIn.
func (db *DB) SignIn(ctx context.Context) (SignIn, error) {
	var s SignIn
	err := db.pool.QueryRow(ctx, `SELECT methods, registration, oidc_name, oidc_issuer, oidc_client_id, oidc_groups_claim, admin_group
		FROM sign_in_settings`).Scan(&s.Methods, &s.Registration, &s.OIDCName, &s.OIDCIssuer, &s.OIDCClientID, &s.OIDCGroupsClaim, &s.AdminGroup)
	if errors.Is(err, pgx.ErrNoRows) {
		d := DefaultSignIn
		d.Methods = slices.Clone(d.Methods)
		return d, nil
	}
	return s, err
}

// SetSignIn saves the settings of the tenant. The order of the methods is always that of Methods.
func (db *DB) SetSignIn(ctx context.Context, s SignIn) error {
	var methods []string
	for _, m := range Methods {
		if slices.Contains(s.Methods, m) {
			methods = append(methods, m)
		}
	}
	for _, m := range s.Methods {
		if !slices.Contains(Methods, m) {
			return ErrMethod
		}
	}
	if len(methods) == 0 {
		return ErrNoMethod
	}
	s.OIDCIssuer = strings.TrimSpace(s.OIDCIssuer)
	s.OIDCClientID = strings.TrimSpace(s.OIDCClientID)
	if slices.Contains(methods, MethodOIDC) && (!strings.HasPrefix(s.OIDCIssuer, "https://") || s.OIDCClientID == "") {
		return ErrOIDC
	}
	if strings.TrimSpace(s.OIDCGroupsClaim) == "" {
		s.OIDCGroupsClaim = "groups"
	}
	_, err := db.pool.Exec(ctx, `
		INSERT INTO sign_in_settings (methods, registration, oidc_name, oidc_issuer, oidc_client_id, oidc_groups_claim, admin_group)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (tenant) DO UPDATE SET methods = EXCLUDED.methods, registration = EXCLUDED.registration,
			oidc_name = EXCLUDED.oidc_name, oidc_issuer = EXCLUDED.oidc_issuer, oidc_client_id = EXCLUDED.oidc_client_id,
			oidc_groups_claim = EXCLUDED.oidc_groups_claim, admin_group = EXCLUDED.admin_group, changed = now()`,
		methods, s.Registration, strings.TrimSpace(s.OIDCName), s.OIDCIssuer, s.OIDCClientID, strings.TrimSpace(s.OIDCGroupsClaim),
		strings.TrimSpace(s.AdminGroup))
	return err
}

// --- Accounts of every source

// Account is an account with where it comes from and, for a local one, its credentials.
type Account struct {
	account.User
	Source       string
	Admin        bool
	PasswordHash string
	TOTPSealed   []byte
}

const accountColumns = `id, login, name, email, groups, source, admin, coalesce(password_hash, ''), totp_sealed`

func scanAccount(row pgx.Row) (Account, error) {
	var a Account
	err := row.Scan(&a.ID, &a.Login, &a.Name, &a.Email, &a.Groups, &a.Source, &a.Admin, &a.PasswordHash, &a.TOTPSealed)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	return a, err
}

// AccountByID returns an account of the tenant.
func (db *DB) AccountByID(ctx context.Context, accountID string) (Account, error) {
	return scanAccount(db.pool.QueryRow(ctx, `SELECT `+accountColumns+` FROM accounts WHERE id = $1`, accountID))
}

// LocalAccount returns the local account with a login, in any case of letters.
func (db *DB) LocalAccount(ctx context.Context, login string) (Account, error) {
	return scanAccount(db.pool.QueryRow(ctx, `SELECT `+accountColumns+` FROM accounts WHERE source = 'local' AND lower(login) = lower($1)`, login))
}

// SeeExternal records a user that the gate or OIDC let in. Such an account can move between gate and OIDC, so a tenant
// can switch from one to the other, but never to or from a local account, and never under the login of one.
func (db *DB) SeeExternal(ctx context.Context, user account.User, source string) error {
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
		if err != nil || !inserted {
			return err
		}
		_, err = insertObject(ctx, tx, Object{Kind: KindCollection, Title: MyNotes, Owner: user.ID, Private: true})
		return err
	})
}

// NewLocal is what a local account starts with when an invitation is accepted: its ID, chosen beforehand so a passkey
// or a sealed TOTP secret can be bound to it, and at least one way in.
type NewLocal struct {
	ID           string
	PasswordHash string
	TOTPSealed   []byte
	TOTPStep     int64
	PasskeyID    []byte
	Passkey      json.RawMessage
	PasskeyName  string
}

// ErrNoWayIn is a new local account without a password and without a passkey.
var ErrNoWayIn = errors.New("a password or a passkey")

// AcceptInvite uses an open invitation and makes its local account with its private notes and its first way in, all in
// one transaction: an invitation is used once, and never leaves an account without a way in.
func (db *DB) AcceptInvite(ctx context.Context, hash []byte, in NewLocal) (Account, error) {
	if in.PasswordHash == "" && len(in.PasskeyID) == 0 {
		return Account{}, ErrNoWayIn
	}
	if !id.Valid(in.ID) {
		in.ID = id.New()
	}
	var a Account
	err := pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		var inv Invite
		err := tx.QueryRow(ctx, `UPDATE invites SET used = now() WHERE hash = $1 AND used IS NULL AND expires > now()
			RETURNING login, name, admin`, hash).Scan(&inv.Login, &inv.Name, &inv.Admin)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		var taken bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM accounts WHERE lower(login) = $1)", inv.Login).Scan(&taken); err != nil {
			return err
		}
		if taken {
			return ErrLoginTaken
		}
		_, err = tx.Exec(ctx, `INSERT INTO accounts (id, login, name, source, admin, password_hash, totp_sealed, totp_step)
			VALUES ($1, $2, $3, 'local', $4, nullif($5, ''), $6, $7)`, in.ID, inv.Login, inv.Name, inv.Admin, in.PasswordHash, in.TOTPSealed, in.TOTPStep)
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return ErrLoginTaken
		}
		if err != nil {
			return err
		}
		if len(in.PasskeyID) > 0 {
			if _, err := tx.Exec(ctx, "INSERT INTO passkeys (id, account, name, credential) VALUES ($1, $2, $3, $4)",
				in.PasskeyID, in.ID, clip(in.PasskeyName, 100), in.Passkey); err != nil {
				return err
			}
		}
		if _, err := insertObject(ctx, tx, Object{Kind: KindCollection, Title: MyNotes, Owner: in.ID, Private: true}); err != nil {
			return err
		}
		a, err = scanAccount(tx.QueryRow(ctx, `SELECT `+accountColumns+` FROM accounts WHERE id = $1`, in.ID))
		return err
	})
	return a, err
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// SetPassword sets or, with an empty hash, removes the password of a local account.
func (db *DB) SetPassword(ctx context.Context, accountID, hash string) error {
	return db.updateLocal(ctx, "UPDATE accounts SET password_hash = nullif($2, '') WHERE id = $1 AND source = 'local'", accountID, hash)
}

// SetTOTP sets the sealed TOTP secret of a local account, or removes it with nil. A new secret starts its steps anew.
func (db *DB) SetTOTP(ctx context.Context, accountID string, sealed []byte) error {
	return db.updateLocal(ctx, "UPDATE accounts SET totp_sealed = $2, totp_step = 0 WHERE id = $1 AND source = 'local'", accountID, sealed)
}

func (db *DB) updateLocal(ctx context.Context, sql string, args ...any) error {
	tag, err := db.pool.Exec(ctx, sql, args...)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// UseTOTPStep records the step of a TOTP code as used, if it comes after the last one. It reports false for a step
// used already, so the same code never works twice, also not in two requests at the same moment.
func (db *DB) UseTOTPStep(ctx context.Context, accountID string, step int64) (bool, error) {
	tag, err := db.pool.Exec(ctx, "UPDATE accounts SET totp_step = $2 WHERE id = $1 AND totp_step < $2", accountID, step)
	return tag.RowsAffected() == 1, err
}

// TOTPStep returns the last step used by an account.
func (db *DB) TOTPStep(ctx context.Context, accountID string) (int64, error) {
	var step int64
	err := db.pool.QueryRow(ctx, "SELECT totp_step FROM accounts WHERE id = $1", accountID).Scan(&step)
	return step, err
}

// CountAccounts returns how many accounts the tenant has.
func (db *DB) CountAccounts(ctx context.Context) (int, error) {
	var n int
	err := db.pool.QueryRow(ctx, "SELECT count(*) FROM accounts").Scan(&n)
	return n, err
}

// --- Passkeys

// Passkey is one passkey of an account.
type Passkey struct {
	ID         []byte
	Account    string
	Name       string
	Credential json.RawMessage
	Created    time.Time
	LastUsed   *time.Time
}

const passkeyColumns = `id, account, name, credential, created, last_used`

func scanPasskey(row pgx.Row) (Passkey, error) {
	var p Passkey
	err := row.Scan(&p.ID, &p.Account, &p.Name, &p.Credential, &p.Created, &p.LastUsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return Passkey{}, ErrNotFound
	}
	return p, err
}

// Passkeys returns the passkeys of an account, oldest first.
func (db *DB) Passkeys(ctx context.Context, accountID string) ([]Passkey, error) {
	rows, err := db.pool.Query(ctx, `SELECT `+passkeyColumns+` FROM passkeys WHERE account = $1 ORDER BY created`, accountID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Passkey, error) { return scanPasskey(row) })
}

// PasskeyByID returns the passkey with a credential ID.
func (db *DB) PasskeyByID(ctx context.Context, credentialID []byte) (Passkey, error) {
	return scanPasskey(db.pool.QueryRow(ctx, `SELECT `+passkeyColumns+` FROM passkeys WHERE id = $1`, credentialID))
}

// AddPasskey keeps a new passkey of a local account.
func (db *DB) AddPasskey(ctx context.Context, accountID, name string, credentialID []byte, credential json.RawMessage) error {
	tag, err := db.pool.Exec(ctx, `INSERT INTO passkeys (id, account, name, credential)
		SELECT $1, id, $3, $4 FROM accounts WHERE id = $2 AND source = 'local'`, credentialID, accountID, clip(name, 100), credential)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return ErrPasskeyTaken
	}
	return err
}

// ErrPasskeyTaken is a passkey kept already.
var ErrPasskeyTaken = errors.New("this passkey is kept already")

// UsedPasskey keeps what a sign-in changed of a passkey, such as its counter, and when it was used.
func (db *DB) UsedPasskey(ctx context.Context, credentialID []byte, credential json.RawMessage) error {
	_, err := db.pool.Exec(ctx, "UPDATE passkeys SET credential = $2, last_used = now() WHERE id = $1", credentialID, credential)
	return err
}

// RemovePasskey takes away a passkey of an account.
func (db *DB) RemovePasskey(ctx context.Context, accountID string, credentialID []byte) error {
	tag, err := db.pool.Exec(ctx, "DELETE FROM passkeys WHERE id = $1 AND account = $2", credentialID, accountID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// --- Sessions

// CreateSession keeps a new session under the hash of its token.
func (db *DB) CreateSession(ctx context.Context, hash []byte, accountID, method string, expires time.Time) error {
	_, err := db.pool.Exec(ctx, "INSERT INTO sessions (hash, account, method, expires) VALUES ($1, $2, $3, $4)", hash, accountID, method, expires)
	return err
}

// Session returns the account of a session that has not expired and was seen within idle, and marks it seen.
func (db *DB) Session(ctx context.Context, hash []byte, idle time.Duration) (accountID, method string, err error) {
	err = db.pool.QueryRow(ctx, `UPDATE sessions SET seen = now() WHERE hash = $1 AND expires > now() AND seen > now() - $2::interval
		RETURNING account, method`, hash, idle.String()).Scan(&accountID, &method)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return accountID, method, err
}

// EndSession ends one session.
func (db *DB) EndSession(ctx context.Context, hash []byte) error {
	_, err := db.pool.Exec(ctx, "DELETE FROM sessions WHERE hash = $1", hash)
	return err
}

// EndSessions ends every session of an account but the one with the hash keep.
func (db *DB) EndSessions(ctx context.Context, accountID string, keep []byte) error {
	_, err := db.pool.Exec(ctx, "DELETE FROM sessions WHERE account = $1 AND hash <> coalesce($2, ''::bytea)", accountID, keep)
	return err
}

// --- Ceremonies

// PutCeremony keeps what the second step of a ceremony needs, under the hash of the token the browser carries.
func (db *DB) PutCeremony(ctx context.Context, hash []byte, kind, accountID string, data json.RawMessage, expires time.Time) error {
	_, err := db.pool.Exec(ctx, "INSERT INTO ceremonies (hash, kind, account, data, expires) VALUES ($1, $2, nullif($3, ''), $4, $5)",
		hash, kind, accountID, data, expires)
	return err
}

// TakeCeremony returns a ceremony and removes it, so it is used once. An expired one is not found.
func (db *DB) TakeCeremony(ctx context.Context, hash []byte, kind string) (accountID string, data json.RawMessage, err error) {
	var fresh bool
	err = db.pool.QueryRow(ctx, `DELETE FROM ceremonies WHERE hash = $1 AND kind = $2 RETURNING coalesce(account, ''), data, expires > now()`,
		hash, kind).Scan(&accountID, &data, &fresh)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !fresh {
		return "", nil, ErrNotFound
	}
	return accountID, data, err
}

// --- Invitations

// Invite is an invitation to a local account.
type Invite struct {
	Login   string
	Name    string
	Admin   bool
	MadeBy  string
	Expires time.Time
}

// CreateInvite keeps an invitation under the hash of its token. The login must be free.
func (db *DB) CreateInvite(ctx context.Context, hash []byte, in Invite) error {
	in.Login = strings.ToLower(strings.TrimSpace(in.Login))
	if !ValidLogin(in.Login) {
		return ErrLogin
	}
	in.Name = strings.TrimSpace(in.Name)
	if n := len([]rune(in.Name)); n < 1 || n > 200 {
		return ErrTitle
	}
	var taken bool
	if err := db.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM accounts WHERE lower(login) = $1)
		OR EXISTS (SELECT 1 FROM invites WHERE login = $1 AND used IS NULL AND expires > now())`, in.Login).Scan(&taken); err != nil {
		return err
	}
	if taken {
		return ErrLoginTaken
	}
	_, err := db.pool.Exec(ctx, "INSERT INTO invites (hash, login, name, admin, made_by, expires) VALUES ($1, $2, $3, $4, $5, $6)",
		hash, in.Login, in.Name, in.Admin, in.MadeBy, in.Expires)
	return err
}

// OpenInvite returns an invitation that is neither used nor expired.
func (db *DB) OpenInvite(ctx context.Context, hash []byte) (Invite, error) {
	var in Invite
	err := db.pool.QueryRow(ctx, "SELECT login, name, admin, made_by, expires FROM invites WHERE hash = $1 AND used IS NULL AND expires > now()",
		hash).Scan(&in.Login, &in.Name, &in.Admin, &in.MadeBy, &in.Expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return Invite{}, ErrNotFound
	}
	return in, err
}

// RecentRegistrations counts the invitations people made for themselves within the last while.
func (db *DB) RecentRegistrations(ctx context.Context, within time.Duration) (int, error) {
	var n int
	err := db.pool.QueryRow(ctx, "SELECT count(*) FROM invites WHERE made_by = '' AND created > now() - $1::interval", within.String()).Scan(&n)
	return n, err
}

// --- The log of sign-ins

// SignInEvent is one sign-in, or a try.
type SignInEvent struct {
	At     time.Time
	Login  string
	Method string
	OK     bool
	Reason string
}

// LogSignIn keeps a sign-in or a failed try.
func (db *DB) LogSignIn(ctx context.Context, e SignInEvent) error {
	login := []rune(e.Login)
	if len(login) > 200 {
		login = login[:200]
	}
	_, err := db.pool.Exec(ctx, "INSERT INTO sign_in_log (login, method, ok, reason) VALUES ($1, $2, $3, $4)", string(login), e.Method, e.OK, e.Reason)
	return err
}

// SignInLog returns the latest sign-ins of the tenant, newest first.
func (db *DB) SignInLog(ctx context.Context, limit int) ([]SignInEvent, error) {
	rows, err := db.pool.Query(ctx, "SELECT at, login, method, ok, reason FROM sign_in_log ORDER BY at DESC, id DESC LIMIT $1", limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (SignInEvent, error) {
		var e SignInEvent
		err := row.Scan(&e.At, &e.Login, &e.Method, &e.OK, &e.Reason)
		return e, err
	})
}

// LogDays is how long the log of sign-ins keeps an entry.
const LogDays = 180

// ExpireSignIns removes what has run out in the tenant of ctx: sessions, ceremonies, invitations a month after their
// end, and entries of the log older than LogDays.
func (db *DB) ExpireSignIns(ctx context.Context) error {
	_, err := db.pool.Exec(ctx, `
		DELETE FROM sessions WHERE expires < now();
		DELETE FROM ceremonies WHERE expires < now();
		DELETE FROM invites WHERE expires < now() - interval '30 days';
		DELETE FROM sign_in_log WHERE at < now() - interval '`+strconv.Itoa(LogDays)+` days'`)
	return err
}

// --- Secrets of other systems, sealed (package vault seals and opens them)

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

// SPDX-License-Identifier: MIT

package db

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Approvals of the gate (ADR-35, T-33): what UnderDoc asked the gate to have confirmed waits here until its answer comes
// back, bound to the account that asked; and the ids of the confirmations it accepted, so that each holds once.

// Kinds of approvals UnderDoc waits for.
const (
	ApprovalPurge = "purge"
	ApprovalEmpty = "empty"
	ApprovalReset = "reset"
)

// ApprovalWait is how long an approval waits for its answer: the gate's own wait (10 minutes) and its pickup (5 minutes).
const ApprovalWait = 15 * time.Minute

// PendingApproval is an approval of the gate UnderDoc waits for.
type PendingApproval struct {
	// ID is the id the gate gave it (apv_…).
	ID string
	// Account is the account that asked; only it takes the answer.
	Account string
	// Kind is ApprovalPurge, ApprovalEmpty or ApprovalReset.
	Kind string
	// Object is what it is for: the object to delete for good, or the account whose access is reset.
	Object string
	// Fields are the values the gate was asked to have confirmed, exactly as sent.
	Fields map[string]any
	// Nonce binds the way back to the browser that asked (the state of the gate's browser entry).
	Nonce string
}

// PutApproval keeps an approval UnderDoc asked the gate for, and forgets those older than ApprovalWait.
func (db *DB) PutApproval(ctx context.Context, p PendingApproval) error {
	fields, err := json.Marshal(p.Fields)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "DELETE FROM gate_approvals WHERE created < now() - $1::interval", ApprovalWait.String()); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO gate_approvals (id, account, kind, object, fields, nonce)
			VALUES ($1, $2, $3, $4, $5, $6)`, p.ID, p.Account, p.Kind, p.Object, fields, p.Nonce)
		return err
	})
}

// PendingApprovalOf returns an approval the account asked for and still waits for, without taking it.
func (db *DB) PendingApprovalOf(ctx context.Context, id, accountID string) (PendingApproval, error) {
	return scanApproval(db.pool.QueryRow(ctx, `SELECT id, account, kind, object, fields, nonce FROM gate_approvals
		WHERE id = $1 AND account = $2 AND created >= now() - $3::interval`, id, accountID, ApprovalWait.String()))
}

// TakeApproval takes an approval the account asked for: once, and only by that account.
func (db *DB) TakeApproval(ctx context.Context, id, accountID string) (PendingApproval, error) {
	return scanApproval(db.pool.QueryRow(ctx, `DELETE FROM gate_approvals WHERE id = $1 AND account = $2
		AND created >= now() - $3::interval RETURNING id, account, kind, object, fields, nonce`, id, accountID, ApprovalWait.String()))
}

func scanApproval(row pgx.Row) (PendingApproval, error) {
	var p PendingApproval
	var fields []byte
	err := row.Scan(&p.ID, &p.Account, &p.Kind, &p.Object, &fields, &p.Nonce)
	if errors.Is(err, pgx.ErrNoRows) {
		return PendingApproval{}, ErrNotFound
	}
	if err != nil {
		return PendingApproval{}, err
	}
	// Numbers stay numbers as sent (json.Number), so that 1 and "1" are told apart as the gate tells them apart.
	dec := json.NewDecoder(bytes.NewReader(fields))
	dec.UseNumber()
	if err := dec.Decode(&p.Fields); err != nil {
		return PendingApproval{}, err
	}
	return p, nil
}

// RememberJTI keeps the id of a confirmation of the gate until it expires, and reports true only the first time: a
// confirmation holds once (the JTIStore of the gate's client). A failure refuses the confirmation.
func (db *DB) RememberJTI(ctx context.Context, jti string, until int64) (bool, error) {
	var first bool
	err := pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "DELETE FROM approval_jti WHERE until < now() - interval '1 day'"); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "INSERT INTO approval_jti (jti, until) VALUES ($1, to_timestamp($2)) ON CONFLICT (tenant, jti) DO NOTHING", jti, until)
		first = tag.RowsAffected() == 1
		return err
	})
	return first, err
}

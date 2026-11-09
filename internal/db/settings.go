// SPDX-License-Identifier: MIT

package db

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

// MaxSettings is the largest the settings of an account may be, in bytes of JSON.
const MaxSettings = 64 << 10

// ErrSettings is settings that are not one JSON object of at most MaxSettings bytes.
var ErrSettings = errors.New("settings must be one JSON object of at most 64 KiB")

// Settings returns the settings of the workspace of an account: one JSON object, empty until it saves some.
func (db *DB) Settings(ctx context.Context, accountID string) (json.RawMessage, error) {
	var raw string
	err := db.pool.QueryRow(ctx, "SELECT settings::text FROM accounts WHERE id = $1", accountID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

// SaveSettings replaces the settings of an account. Their shape is the app's; the database only takes care that they
// are one object and not too large.
func (db *DB) SaveSettings(ctx context.Context, accountID string, raw []byte) error {
	var object map[string]json.RawMessage
	if len(raw) > MaxSettings || !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) || json.Unmarshal(raw, &object) != nil {
		return ErrSettings
	}
	tag, err := db.pool.Exec(ctx, "UPDATE accounts SET settings = $2::jsonb WHERE id = $1", accountID, string(raw))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

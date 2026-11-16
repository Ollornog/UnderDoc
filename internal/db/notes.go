// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/account"
)

// QuickNotes is the title of the board of quick notes every account has in its private notes (ADR-16, M-8).
const QuickNotes = "Quick notes"

// QuickNotesOf returns the account's board of quick notes, made in its private notes the first time it is asked for. New
// lands there without choosing a place first.
func (db *DB) QuickNotesOf(ctx context.Context, user account.User) (Object, error) {
	var board Object
	err := pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		// One account at a time, so two first notes at once make one board.
		if _, err := tx.Exec(ctx, "SELECT 1 FROM accounts WHERE id = $1 FOR UPDATE", user.ID); err != nil {
			return err
		}
		var err error
		board, err = scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM objects o WHERE kind = 'notes' AND owner = $1 AND `+alive+`
			AND parent IN (SELECT id FROM objects WHERE parent IS NULL AND private AND owner = $1 AND deleted_at IS NULL)
			ORDER BY created, id LIMIT 1`, user.ID))
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var notes string
		err = tx.QueryRow(ctx, `SELECT id FROM objects WHERE parent IS NULL AND private AND owner = $1 AND kind = 'collection'
			AND deleted_at IS NULL ORDER BY created, id LIMIT 1`, user.ID).Scan(&notes)
		if errors.Is(err, pgx.ErrNoRows) {
			made, err := insertObject(ctx, tx, Object{Kind: KindCollection, Title: MyNotes, Owner: user.ID, Private: true})
			if err != nil {
				return err
			}
			notes = made.ID
		} else if err != nil {
			return err
		}
		board, err = insertObject(ctx, tx, Object{Kind: KindNotes, Title: QuickNotes, Owner: user.ID, Parent: notes})
		return err
	})
	return board, err
}

// AddNote puts a card on top of the account's board of quick notes and returns the board with the card.
func (db *DB) AddNote(ctx context.Context, user account.User, body string) (Object, Block, error) {
	board, err := db.QuickNotesOf(ctx, user)
	if err != nil {
		return Object{}, Block{}, err
	}
	card, err := db.AddBlock(ctx, board.ID, user.ID, "", body)
	return board, card, err
}

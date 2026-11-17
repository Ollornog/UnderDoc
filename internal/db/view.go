// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
)

// Widths and places of the contents a document can have (ADR-16); empty is the default of the workspace.
var (
	widths   = []string{"", "slim", "wide", "full"}
	contents = []string{"", "marker", "side", "hidden"}
)

// ValidView tells whether a width and a place of the contents are known; empty is the default.
func ValidView(width, place string) bool {
	return slices.Contains(widths, width) && slices.Contains(contents, place)
}

// ErrView is a width or a place of the contents the workspace does not know.
var ErrView = errors.New("width is slim, wide or full; contents are marker, side or hidden")

// SetView keeps how a document is shown: its width and where its contents show, each only when given. Whoever may change
// the object sets it (ADR-20), as with the tags; lines and spaces have no view.
func (db *DB) SetView(ctx context.Context, objectID, viewer string, width, place *string) (Object, error) {
	if (width != nil && !slices.Contains(widths, *width)) || (place != nil && !slices.Contains(contents, *place)) {
		return Object{}, ErrView
	}
	o, err := db.Object(ctx, objectID, viewer)
	if err != nil {
		return Object{}, err
	}
	if IsDivider(o.Kind) {
		return Object{}, ErrPlace
	}
	if err := db.mayEdit(ctx, o, viewer); err != nil {
		return Object{}, err
	}
	if width != nil {
		o.Width = *width
	}
	if place != nil {
		o.Contents = *place
	}
	return scan(db.pool.QueryRow(ctx, "UPDATE objects SET width = nullif($2, ''), contents = nullif($3, '') WHERE id = $1 RETURNING "+columns,
		objectID, o.Width, o.Contents))
}

// ErrAlign is an alignment of a block other than left, center or right.
var ErrAlign = errors.New("a block is aligned left, center or right")

// ValidAlign tells whether a block can be aligned so; empty is the left.
func ValidAlign(align string) bool { return align == "" || align == "center" || align == "right" }

// AlignBlock aligns a block of a page left, center or right (ADR-16). The text and its versions stay as they are, so it
// makes no version; the page counts as changed.
func (db *DB) AlignBlock(ctx context.Context, pageID, blockID, viewer, align string) error {
	if align == "left" {
		align = ""
	}
	if !ValidAlign(align) {
		return ErrAlign
	}
	return db.change(ctx, pageID, viewer, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "UPDATE blocks SET align = nullif($3, '') WHERE page = $1 AND id = $2", pageID, blockID, align)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

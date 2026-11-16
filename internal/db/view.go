// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"slices"
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

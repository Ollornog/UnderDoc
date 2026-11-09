// SPDX-License-Identifier: MIT

package db

import "context"

// SetAccess sets what all others may do with an object: edit, comment or read, or "" to take it from above (ADR-20). Only
// the owner sets it, so no one who may edit can shut the others out. Lines and spaces have no level of their own.
func (db *DB) SetAccess(ctx context.Context, objectID, viewer, access string) (Object, error) {
	switch access {
	case "", "edit", "comment", "read":
	default:
		return Object{}, ErrPlace
	}
	o, err := db.Object(ctx, objectID, viewer)
	if err != nil {
		return Object{}, err
	}
	if o.Owner != viewer {
		return Object{}, ErrNotAllowed
	}
	if IsDivider(o.Kind) {
		return Object{}, ErrPlace
	}
	return scan(db.pool.QueryRow(ctx, "UPDATE objects SET access = nullif($2, ''), changed = now() WHERE id = $1 RETURNING "+columns, objectID, access))
}

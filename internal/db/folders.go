// SPDX-License-Identifier: MIT

package db

import (
	"context"

	"github.com/Ollornog/UnderDoc/internal/account"
)

// A folder and a collection are the same but for one thing: a collection can hold collections that show in nav2, a
// folder shows only in nav3 (ADR-16, 2026-11-09). Either turns into the other.

// CreateFolder makes a folder in parent. A folder never stands at the top, and whoever may change the parent may add
// one (ADR-20).
func (db *DB) CreateFolder(ctx context.Context, owner account.User, title, parent string) (Object, error) {
	if parent == "" {
		return Object{}, ErrPlace
	}
	p, err := db.Object(ctx, parent, owner.ID)
	if err != nil {
		return Object{}, err
	}
	if err := db.mayEdit(ctx, p, owner.ID); err != nil {
		return Object{}, err
	}
	if IsDivider(p.Kind) {
		return Object{}, ErrPlace
	}
	return insertObject(ctx, db.pool, Object{Kind: KindFolder, Title: title, Owner: owner.ID, Parent: parent})
}

// Convert turns a collection into a folder or a folder into a collection. Only a collection stands at the top, so one
// there stays a collection; the private notes of an account stand there too.
func (db *DB) Convert(ctx context.Context, objectID, viewer, kind string) (Object, error) {
	if kind != KindCollection && kind != KindFolder {
		return Object{}, ErrPlace
	}
	o, err := db.Object(ctx, objectID, viewer)
	if err != nil {
		return Object{}, err
	}
	if err := db.mayEdit(ctx, o, viewer); err != nil {
		return Object{}, err
	}
	if o.Kind != KindCollection && o.Kind != KindFolder {
		return Object{}, ErrPlace
	}
	if kind == KindFolder && o.Parent == "" {
		return Object{}, ErrPlace
	}
	return scan(db.pool.QueryRow(ctx, "UPDATE objects SET kind = $2, changed = now() WHERE id = $1 RETURNING "+columns, objectID, kind))
}

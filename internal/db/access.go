// SPDX-License-Identifier: MIT

package db

import "context"

// SetAccess sets what the whole tenant may do with an object: edit, comment or read, or "" to take it from above again.
// It is the level of ADR-20 expressed in the roles of ADR-29: a level narrower than the one above breaks the inheritance
// (which copies what held from above) and grants the tenant that level; "" takes the tenant's grant away and restores
// the inheritance, and at the top, where nothing is above, it means edit as in ADR-20. Whoever manages the object sets it, and an administrator; lines and spaces have no level.
func (db *DB) SetAccess(ctx context.Context, objectID string, actor Actor, access string) (Object, error) {
	switch access {
	case "", RoleEdit, RoleComment, RoleRead:
	default:
		return Object{}, ErrPlace
	}
	o, err := db.forRights(ctx, objectID, actor)
	if err != nil {
		return Object{}, err
	}
	if access == "" && o.Parent == "" {
		// At the top nothing is above: "" means the default of ADR-20 there, edit.
		access = RoleEdit
	}
	if access == "" {
		if err := db.SetGrant(ctx, objectID, actor, ToTenant, "", RoleNone); err != nil {
			return Object{}, err
		}
		if o.Parent != "" {
			if err := db.SetInherit(ctx, objectID, actor, true); err != nil {
				return Object{}, err
			}
		}
		return db.objectForRights(ctx, objectID)
	}
	if o.Parent != "" {
		above, err := db.tenantAbove(ctx, o)
		if err != nil {
			return Object{}, err
		}
		if Rank(access) < Rank(above) {
			if err := db.SetInherit(ctx, objectID, actor, false); err != nil {
				return Object{}, err
			}
		}
	}
	if err := db.SetGrant(ctx, objectID, actor, ToTenant, "", access); err != nil {
		return Object{}, err
	}
	return db.objectForRights(ctx, objectID)
}

// tenantAbove is the highest role the whole tenant holds on the objects above o, while o inherits from them.
func (db *DB) tenantAbove(ctx context.Context, o Object) (string, error) {
	if o.Breaks || o.Parent == "" {
		return RoleNone, nil
	}
	var rank int
	err := db.pool.QueryRow(ctx, `WITH RECURSIVE up AS (
			SELECT id, parent, inherit FROM objects WHERE id = $1
			UNION ALL SELECT p.id, p.parent, p.inherit FROM objects p JOIN up ON p.id = up.parent WHERE up.inherit
		)
		SELECT coalesce(max(role_rank(g.role)), 0) FROM up JOIN grants g ON g.object = up.id AND g.kind = 'tenant'`, o.Parent).Scan(&rank)
	return roleOf(rank), err
}

// objectForRights reads an object again after a change of its rights, whoever made it.
func (db *DB) objectForRights(ctx context.Context, objectID string) (Object, error) {
	return scan(db.pool.QueryRow(ctx, `SELECT `+columns+` FROM objects WHERE id = $1`, objectID))
}

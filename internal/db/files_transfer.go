// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// The files in a backup (ADR-28: the database and the content store together). The database hands out every root of the
// view Files with its tree and every version, and the attachments of pages; package transfer writes them with the
// content from the store, and reads them back.

// FileNode is a root, folder or file of the view Files with everything below it. Logins stand for accounts, so a backup
// can come back into another database.
type FileNode struct {
	Object
	// OwnerLogin is the login of the owner.
	OwnerLogin string
	// Role, Writers, Quota and Login are set on a root: Login is the user of a user's folder, Quota -1 for none.
	Role    string
	Writers string
	Quota   int64
	Login   string
	// Versions of a file, oldest first; Author holds the login.
	Versions []FileVersion
	// Tables of a spreadsheet that have an ID (ADR-31).
	Tables []FileTable
	Kids   []*FileNode
}

// AttachmentRecord is an attachment of a page with the login of its author.
type AttachmentRecord struct {
	Attachment
	AuthorLogin string
}

// AllFiles reads every root of the view Files with its tree and versions, and every attachment of a page, all that is
// not in the trash, as a full backup on the server needs it.
func (db *DB) AllFiles(ctx context.Context) ([]*FileNode, []AttachmentRecord, error) {
	logins, err := db.logins(ctx)
	if err != nil {
		return nil, nil, err
	}
	rows, err := db.pool.Query(ctx, `SELECT `+columns+`, coalesce(r.role, ''), CASE WHEN r.object IS NULL THEN '' ELSE `+writersOf+` END, coalesce(r.quota, -1), coalesce(r.account, '')
		FROM objects o LEFT JOIN file_roots r ON r.object = o.id WHERE `+alive+` AND area = 'files' AND kind <> 'line' AND kind <> 'space'
		ORDER BY position, created, id`)
	if err != nil {
		return nil, nil, err
	}
	nodes, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*FileNode, error) {
		n := &FileNode{}
		o := &n.Object
		var account string
		err := row.Scan(append(fields(o), &n.Role, &n.Writers, &n.Quota, &account)...)
		n.OwnerLogin, n.Login = logins[o.Owner], logins[account]
		return n, err
	})
	if err != nil {
		return nil, nil, err
	}
	byID := map[string]*FileNode{}
	for _, n := range nodes {
		byID[n.ID] = n
	}
	tables, err := db.tablesForExport(ctx)
	if err != nil {
		return nil, nil, err
	}
	for file, list := range tables {
		if n := byID[file]; n != nil {
			n.Tables = list
		}
	}
	versions, err := db.pool.Query(ctx, `SELECT file, number, blob, size, name, content_type, author, at FROM file_versions ORDER BY file, number`)
	if err != nil {
		return nil, nil, err
	}
	err = forEach(versions, func(row pgx.CollectableRow) error {
		var file string
		var v FileVersion
		if err := row.Scan(&file, &v.Number, &v.Blob, &v.Size, &v.Name, &v.ContentType, &v.Author, &v.At); err != nil {
			return err
		}
		if n := byID[file]; n != nil {
			v.Author = logins[v.Author]
			n.Versions = append(n.Versions, v)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	var roots []*FileNode
	for _, n := range nodes {
		if n.Parent == "" {
			roots = append(roots, n)
		} else if p := byID[n.Parent]; p != nil {
			p.Kids = append(p.Kids, n)
		}
	}
	rows, err = db.pool.Query(ctx, `SELECT `+attachmentColumns+` FROM attachments a
		WHERE EXISTS (SELECT 1 FROM objects o WHERE o.id = a.page AND `+alive+`) ORDER BY page, length(id), id`)
	if err != nil {
		return nil, nil, err
	}
	attachments, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (AttachmentRecord, error) {
		a, err := scanAttachment(row)
		return AttachmentRecord{Attachment: a, AuthorLogin: logins[a.Author]}, err
	})
	return roots, attachments, err
}

func forEach(rows pgx.Rows, fn func(row pgx.CollectableRow) error) error {
	defer rows.Close()
	for rows.Next() {
		if err := fn(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// FilesImported says what an import of files did.
type FilesImported struct {
	Roots, Objects, Versions, Attachments int
	// Renamed maps each ID that was taken or not valid to the new one it got instead.
	Renamed  map[string]string
	Warnings []string
}

// ImportFiles brings roots of the view Files with their trees and versions, and attachments of pages, into the database
// in one transaction; every blob they name must be in the store already (have tells). Logins become accounts where an
// account has that login, the account of ownerLogin otherwise. Objects keep their IDs where those are free. A folder of a
// user comes back as that user's folder if the user has none yet, the folder of the tenant if there is none yet; else
// each becomes a collection, and the warnings say so. An attachment whose page is not there is left out with a warning.
func (db *DB) ImportFiles(ctx context.Context, ownerLogin string, roots []*FileNode, attachments []AttachmentRecord, have func(sha string) bool) (FilesImported, error) {
	result := FilesImported{Renamed: map[string]string{}}
	accounts := map[string]string{}
	rows, err := db.pool.Query(ctx, "SELECT login, id FROM accounts ORDER BY first_seen DESC")
	if err != nil {
		return result, err
	}
	if err := forEach(rows, func(row pgx.CollectableRow) error {
		var login, id string
		err := row.Scan(&login, &id)
		accounts[login] = id
		return err
	}); err != nil {
		return result, err
	}
	owner, ok := accounts[ownerLogin]
	if !ok {
		return result, ErrNoAccount
	}
	account := func(login string) string {
		if id, ok := accounts[login]; ok && login != "" {
			return id
		}
		return owner
	}
	err = pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockRoots); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock_shared($1)", lockBlobs); err != nil {
			return err
		}
		var write func(n *FileNode, parent string, position int) error
		write = func(n *FileNode, parent string, position int) error {
			o := n.Object
			o.Owner, o.Parent, o.Position, o.Area = account(n.OwnerLogin), parent, position, AreaFiles
			if o.Kind != KindCollection && o.Kind != KindFolder && o.Kind != KindFile {
				return fmt.Errorf("%s: a kind the files do not have: %q", o.ID, o.Kind)
			}
			if clean, err := CleanTags(o.Tags); err == nil {
				o.Tags = clean
			} else {
				o.Tags = nil
			}
			if o.Kind == KindFile {
				name, err := CleanFileName(o.Title)
				if err != nil {
					return fmt.Errorf("%s: %w", o.ID, err)
				}
				o.Title = name
			}
			made, err := insertWithID(ctx, tx, o)
			if err != nil {
				return err
			}
			if made.ID != o.ID && o.ID != "" {
				result.Renamed[o.ID] = made.ID
			}
			result.Objects++
			for _, v := range n.Versions {
				if !have(v.Blob) {
					return fmt.Errorf("%s: version %d: blob %s is not in the store", o.ID, v.Number, v.Blob)
				}
				if _, err := tx.Exec(ctx, "INSERT INTO blobs (sha256, size) VALUES ($1, $2) ON CONFLICT (tenant, sha256) DO NOTHING", v.Blob, v.Size); err != nil {
					return err
				}
				at := v.At
				if at.IsZero() {
					at = time.Now()
				}
				if _, err := tx.Exec(ctx, `INSERT INTO file_versions (file, number, blob, size, name, content_type, author, at)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, made.ID, v.Number, v.Blob, v.Size, v.Name, v.ContentType, account(v.Author), at); err != nil {
					return err
				}
				result.Versions++
			}
			for _, t := range n.Tables {
				if err := importTable(ctx, tx, made.ID, o.Owner, t); err != nil {
					return err
				}
			}
			for i, k := range n.Kids {
				if err := write(k, made.ID, i); err != nil {
					return err
				}
			}
			if parent != "" {
				return nil
			}
			role, user := n.Role, ""
			switch role {
			case RootTenant:
				var taken bool
				if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM file_roots WHERE role = 'tenant')").Scan(&taken); err != nil {
					return err
				}
				if taken {
					role = RootCollection
					result.Warnings = append(result.Warnings, fmt.Sprintf("%q: there is a folder of the tenant already, it came back as a collection", o.Title))
				}
			case RootUser:
				id, known := accounts[n.Login]
				var taken bool
				if known {
					if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM file_roots WHERE role = 'user' AND account = $1)", id).Scan(&taken); err != nil {
						return err
					}
				}
				if !known || taken {
					role = RootCollection
					result.Warnings = append(result.Warnings, fmt.Sprintf("%q of %s: no account without a folder of its own, it came back as a collection", o.Title, n.Login))
				} else {
					user = id
					if _, err := tx.Exec(ctx, "UPDATE objects SET owner = $2, private = true WHERE id = $1", made.ID, id); err != nil {
						return err
					}
				}
			default:
				role = RootCollection
			}
			quota := &n.Quota
			if n.Quota < 0 {
				quota = nil
			}
			result.Roots++
			if _, err = tx.Exec(ctx, "INSERT INTO file_roots (object, role, account, quota) VALUES ($1, $2, nullif($3, ''), $4)",
				made.ID, role, user, quota); err != nil {
				return err
			}
			// The writers of a root come back as groups with the role edit (ADR-29).
			for _, group := range strings.Split(n.Writers, ",") {
				if group = strings.TrimSpace(group); group == "" {
					continue
				}
				groupID, err := groupNamed(ctx, tx, group)
				if err != nil {
					return err
				}
				if err := setGrant(ctx, tx, made.ID, ToGroup, groupID, RoleEdit, ""); err != nil {
					return err
				}
			}
			return nil
		}
		var position int
		if err := tx.QueryRow(ctx, "SELECT coalesce(max(position) + 1, 0) FROM objects WHERE parent IS NULL").Scan(&position); err != nil {
			return err
		}
		for i, root := range roots {
			if root.Kind != KindCollection {
				return fmt.Errorf("%s: a root is a collection", root.ID)
			}
			if err := write(root, "", position+i); err != nil {
				return err
			}
		}
		for _, a := range attachments {
			var page bool
			if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM objects WHERE id = $1 AND kind = 'page')", a.Page).Scan(&page); err != nil {
				return err
			}
			if !page {
				result.Warnings = append(result.Warnings, fmt.Sprintf("attachment %s of page %s: the page is not there, left out", a.ID, a.Page))
				continue
			}
			if !have(a.Blob) {
				return fmt.Errorf("attachment %s of page %s: blob %s is not in the store", a.ID, a.Page, a.Blob)
			}
			if _, err := tx.Exec(ctx, "INSERT INTO blobs (sha256, size) VALUES ($1, $2) ON CONFLICT (tenant, sha256) DO NOTHING", a.Blob, a.Size); err != nil {
				return err
			}
			tag, err := tx.Exec(ctx, `INSERT INTO attachments (page, id, blob, size, name, content_type, author, at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				ON CONFLICT (tenant, page, id) DO NOTHING`, a.Page, a.ID, a.Blob, a.Size, a.Name, a.ContentType, account(a.AuthorLogin), a.At)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				result.Warnings = append(result.Warnings, fmt.Sprintf("attachment %s of page %s: the page has one with this ID, left out", a.ID, a.Page))
				continue
			}
			var number int
			_, _ = fmt.Sscanf(a.ID, "a%d", &number)
			if _, err := tx.Exec(ctx, "UPDATE objects SET next_attachment = greatest(next_attachment, $2) WHERE id = $1", a.Page, number+1); err != nil {
				return err
			}
			result.Attachments++
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return result, err
}

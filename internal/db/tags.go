// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

// MaxTags is how many tags an object carries at most; a tag is at most MaxTag characters.
const (
	MaxTags = 50
	MaxTag  = 40
)

// ErrTag is a tag that is empty, too long or made of other characters than letters, digits, "-", "_" and ".".
var ErrTag = errors.New("a tag is 1 to 40 letters, digits, dashes, underscores or dots")

var tagForm = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N}._-]*$`)

// CleanTags makes tags as they are kept: lower case, without a leading #, each once, in the order given.
func CleanTags(tags []string) ([]string, error) {
	out := []string{}
	for _, t := range tags {
		t = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(t), "#"))
		if t == "" {
			continue
		}
		if len([]rune(t)) > MaxTag || !tagForm.MatchString(t) {
			return nil, ErrTag
		}
		if !contains(out, t) {
			out = append(out, t)
		}
	}
	if len(out) > MaxTags {
		return nil, ErrTag
	}
	return out, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// SetTags gives an object its tags. Whoever may change the object may tag it (ADR-20); lines and spaces carry none.
func (db *DB) SetTags(ctx context.Context, objectID, viewer string, tags []string) (Object, error) {
	clean, err := CleanTags(tags)
	if err != nil {
		return Object{}, err
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
	return scan(db.pool.QueryRow(ctx, "UPDATE objects SET tags = $2, changed = now() WHERE id = $1 RETURNING "+columns, objectID, clean))
}

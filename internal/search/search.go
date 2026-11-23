// SPDX-License-Identifier: MIT

// Package search finds what a reader may read (ADR-24). Content goes to a search service in sections, one record per
// heading and what follows it (ADR-22), so a hit opens where it was found. The service is an Engine: Meilisearch by
// default, OpenSearch as a choice, both held to the same tests. Without one, a simple search of titles stands in.
//
// The index is fed through a queue in the database (Indexer), so it can be thrown away and built again at any time.
// Sources say what there is to index: pages of the database and of the folder today; tables, lists and files of later
// milestones only add a Source of their own.
//
// Ranking: error codes first, exactly as written (codes.go), then the engine's relevance with the title above the
// headings above the text, exact words above typing errors, and newer only to break a tie.
package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"time"
)

// Types of what is found. Sources of later milestones add theirs (table, list, database).
const (
	TypePage       = "page"
	TypeFolder     = "folder"
	TypeCollection = "collection"
	// TypeFile is a file of the view Files, or an attachment of a page, found by the text taken out of it (M-2).
	TypeFile = "file"
)

// Everyone is the reader every signed-in account of a tenant counts as, where the whole tenant holds a role. A record that
// only some may read lists them as ReaderAccount(id) and ReaderGroup(id) (ADR-29).
const Everyone = "all"

// ReaderAccount is the reader token of one account.
func ReaderAccount(accountID string) string { return "account:" + accountID }

// ReaderGroup is the reader token of one group.
func ReaderGroup(groupID string) string { return "group:" + groupID }

// ReaderDirect is the reader token of the persons directly in one group: a grant to a group without its subgroups
// (ADR-36).
func ReaderDirect(groupID string) string { return "direct:" + groupID }

// Readers returns the reader tokens of an account and the groups it belongs to, with the groups inside which it is
// directly: what it may read is decided by the entries that name one of them.
func Readers(accountID string, groups ...string) []string {
	readers := []string{Everyone, ReaderAccount(accountID)}
	for _, g := range groups {
		readers = append(readers, ReaderGroup(g))
	}
	return readers
}

// ReadersOf is Readers with the groups an account is in directly, for the grants that leave out the subgroups.
func ReadersOf(accountID string, groups, direct []string) []string {
	readers := Readers(accountID, groups...)
	for _, g := range direct {
		readers = append(readers, ReaderDirect(g))
	}
	return readers
}

// MaxLevels is how many objects with entries on read, from a record up, the index keeps (ADR-36). A record whose
// entries lie on more levels is found by its owner alone (fail closed).
const MaxLevels = 6

// Level is one object from a record up that has entries on read for someone: the tokens allowed and denied there. The
// first level that names one of a reader's tokens decides, and on it deny beats allow (ADR-36).
type Level struct {
	Allow []string
	Deny  []string
}

// LevelField names the field of the index that holds the allowed (or denied) tokens of a level.
func LevelField(i int, deny bool) string {
	if deny {
		return fmt.Sprintf("l%dd", i)
	}
	return fmt.Sprintf("l%da", i)
}

// Doc is one record in the index: a section of a page, or an object that has no sections, such as a folder.
type Doc struct {
	// ID is unique in the index of a tenant; Replace sets it.
	ID string `json:"id"`
	// Source and Key say where the record came from, so the source can replace all records of one key.
	Source string `json:"source"`
	Key    string `json:"key"`
	// Object is what a hit opens, and Anchor the block or heading there. An empty anchor opens the top.
	Object string `json:"object"`
	Anchor string `json:"anchor"`
	// Rank is the order of the section in its object.
	Rank    int      `json:"rank"`
	Type    string   `json:"type"`
	Title   string   `json:"title"`
	Heading string   `json:"heading"`
	Body    string   `json:"body"`
	Tags    []string `json:"tags"`
	// Codes holds the canonical codes of title, heading and body as a whole, CodeParts their parts, and TitleCodes and
	// HeadingCodes the whole codes of the title and the heading. Prepare fills them.
	Codes        []string `json:"codes"`
	CodeParts    []string `json:"code_parts"`
	TitleCodes   []string `json:"title_codes"`
	HeadingCodes []string `json:"heading_codes"`
	// Path names the places above the object, for showing.
	Path           string `json:"path"`
	Collection     string `json:"collection"`
	CollectionName string `json:"collection_name"`
	Author         string `json:"author"`
	AuthorName     string `json:"author_name"`
	// Changed is in Unix seconds.
	Changed int64 `json:"changed"`
	// Readers are the tokens that always find this record: its owner, who keeps every action on it, or the owner of the
	// private tree it lies in. Everyone else is decided by the levels below (ADR-36). Nobody else finds a record without
	// levels.
	Readers []string `json:"readers"`
	// L0A to L5D are the levels of entries on read from the record up, nearest first (Level, MaxLevels): the allowed
	// (A) and the denied (D) tokens of each.
	L0A []string `json:"l0a"`
	L0D []string `json:"l0d"`
	L1A []string `json:"l1a"`
	L1D []string `json:"l1d"`
	L2A []string `json:"l2a"`
	L2D []string `json:"l2d"`
	L3A []string `json:"l3a"`
	L3D []string `json:"l3d"`
	L4A []string `json:"l4a"`
	L4D []string `json:"l4d"`
	L5A []string `json:"l5a"`
	L5D []string `json:"l5d"`
	// NoAgents tells that a deny on read for Agents holds on the record or above it: no agent finds it.
	NoAgents bool `json:"no_agents"`
	// Origin says where the text came from, for text taken out of files: typed, browser, server or ai.
	Origin string `json:"origin,omitempty"`
}

// Prepare fills the code fields, drops the marker characters of engines from the text and makes lists non-nil, so every
// engine stores the same.
func (d *Doc) Prepare() {
	d.Title, d.Heading, d.Body = dropMarks(d.Title), dropMarks(d.Heading), dropMarks(d.Body)
	d.TitleCodes, d.HeadingCodes, d.Codes, d.CodeParts = []string{}, []string{}, []string{}, []string{}
	for _, field := range []struct {
		codes *[]string
		text  string
	}{{&d.TitleCodes, d.Title}, {&d.HeadingCodes, d.Heading}, {&d.Codes, d.Body}} {
		for _, token := range FindCodes(field.text) {
			*field.codes = appendNew(*field.codes, token.Code)
			d.Codes = appendNew(d.Codes, token.Code)
			for _, part := range token.Parts {
				d.CodeParts = appendNew(d.CodeParts, part)
			}
		}
	}
	if d.Tags == nil {
		d.Tags = []string{}
	}
	if d.Readers == nil {
		d.Readers = []string{}
	}
	for _, f := range d.levelFields() {
		if *f == nil {
			*f = []string{}
		}
	}
}

func (d *Doc) levelFields() []*[]string {
	return []*[]string{&d.L0A, &d.L0D, &d.L1A, &d.L1D, &d.L2A, &d.L2D, &d.L3A, &d.L3D, &d.L4A, &d.L4D, &d.L5A, &d.L5D}
}

// SetLevels keeps the levels of a record, at most MaxLevels; Levels reads them back.
func (d *Doc) SetLevels(levels []Level) {
	f := d.levelFields()
	for i := 0; i < MaxLevels; i++ {
		*f[2*i], *f[2*i+1] = []string{}, []string{}
		if i < len(levels) {
			*f[2*i], *f[2*i+1] = append([]string{}, levels[i].Allow...), append([]string{}, levels[i].Deny...)
		}
	}
}

// Levels returns the levels of a record.
func (d *Doc) Levels() []Level {
	f := d.levelFields()
	var out []Level
	for i := 0; i < MaxLevels; i++ {
		if len(*f[2*i]) == 0 && len(*f[2*i+1]) == 0 {
			break
		}
		out = append(out, Level{Allow: *f[2*i], Deny: *f[2*i+1]})
	}
	return out
}

// MayRead decides from the record alone whether a reader with these tokens finds it, as every engine does in its
// filter: one of the always tokens, or the first level that names a token allows it and denies none; an agent never
// finds a record with NoAgents.
func (d *Doc) MayRead(readers []string, agent bool) bool {
	if agent && d.NoAgents {
		return false
	}
	has := func(list []string) bool {
		return slices.ContainsFunc(list, func(t string) bool { return slices.Contains(readers, t) })
	}
	if has(d.Readers) {
		return true
	}
	for _, l := range d.Levels() {
		a, x := has(l.Allow), has(l.Deny)
		if a || x {
			return a && !x
		}
	}
	return false
}

// Key names one key of a source: the unit a source replaces at once, such as one page.
type Key struct {
	Source string
	Key    string
}

// Filters narrow a search, as the results page offers them. Within one filter the values are alternatives, between
// filters all must hold.
type Filters struct {
	Types       []string
	Collections []string
	Authors     []string
	Tags        []string
	// From and To bound the time of the last change, zero means open.
	From, To time.Time
}

// Fields of a record a query can be limited to by codes.
const (
	FieldTitleCodes   = "title_codes"
	FieldHeadingCodes = "heading_codes"
	FieldCodes        = "codes"
	FieldCodeParts    = "code_parts"
)

// CodeCondition holds when the field has one of the codes.
type CodeCondition struct {
	Field string
	Codes []string
}

// Query is what an engine runs. Readers are never empty: a query without them would find what nobody may read.
type Query struct {
	// Text is searched in title, headings, tags and text. Empty finds all that the filters let through.
	Text string
	// Readers are the tokens of the reader (ReadersOf), Agent whether the reader is an agent (ADR-36).
	Readers []string
	Agent   bool
	Filters Filters
	// Codes limits the query to records that meet one of the conditions.
	Codes []CodeCondition
	// Newest sorts by the time of the last change instead of relevance, for a query without text.
	Newest bool
	// Phrase finds the text only as written, its words in this order and without typing errors.
	Phrase bool
	// Limit is the number of records to return, at most 1000.
	Limit int
	// OnePerObject keeps only the best record of each object.
	OnePerObject bool
	// Facets asks for the counts by type, collection, author and tag.
	Facets bool
}

// Facet names.
const (
	FacetType       = "type"
	FacetCollection = "collection"
	FacetAuthor     = "author"
	FacetTag        = "tags"
)

// Facets counts records by the values of a field.
type Facets map[string]map[string]int

// Found is one record an engine returned. Snippet is a piece of the text with the matching words marked; an engine that
// cannot mark leaves it empty.
type Found struct {
	Doc     Doc
	Snippet []Fragment
}

// Answer is what an engine returns for a query.
type Answer struct {
	Found  []Found
	Total  int
	Facets Facets
}

// Engine is a search service. The index of each tenant is its own (ADR-23).
type Engine interface {
	// Name is meilisearch or opensearch.
	Name() string
	// Prepare makes the index of a tenant ready, with its settings. It reports whether the index had to be made anew or
	// its settings changed in a way that needs every record again.
	Prepare(ctx context.Context, tenant string) (rebuild bool, err error)
	// Replace removes every record of the keys and adds docs instead. Docs have their IDs and code fields set.
	Replace(ctx context.Context, tenant string, keys []Key, docs []Doc) error
	// Search runs a query.
	Search(ctx context.Context, tenant string, q Query) (Answer, error)
	// Drop removes the index of a tenant.
	Drop(ctx context.Context, tenant string) error
}

// ErrNoReaders guards against a query that would ignore who reads.
var ErrNoReaders = errors.New("search: a query needs the readers")

// MaxLimit is the most records one query returns.
const MaxLimit = 1000

// IndexName is the name of the index of a tenant: the prefix of the installation and the tenant. A tenant whose name
// engines would not accept as it is goes by a hash of it.
func IndexName(prefix, tenant string) string {
	clean := tenant != "" && len(tenant) <= 48
	for _, r := range tenant {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			clean = false
		}
	}
	if !clean {
		sum := sha256.Sum256([]byte(tenant))
		tenant = "t" + hex.EncodeToString(sum[:10])
	}
	return prefix + "-" + tenant
}

// StopWords are German words too common to say anything about a page. A query ignores them, so "Webshop nicht
// erreichbar" ranks by "Webshop" and "erreichbar" (measured in docs/search.md).
var StopWords = []string{"der", "die", "das", "den", "dem", "des", "ein", "eine", "einen", "einem", "einer", "und", "oder",
	"nicht", "ist", "im", "in", "zu", "zum", "zur", "mit", "auf", "für", "von", "bei", "am", "an", "wie", "was", "wer",
	"will", "geht", "es", "sich", "auch", "nur", "noch", "kein", "keine", "keinen", "darf", "kann", "muss", "soll", "wird",
	"werden", "wurde", "sind", "hat", "haben", "aus", "als", "so", "dass", "wenn", "aber"}

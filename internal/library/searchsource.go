// SPDX-License-Identifier: MIT

package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"github.com/Ollornog/UnderDoc/internal/document"
	"github.com/Ollornog/UnderDoc/internal/search"
	"github.com/Ollornog/UnderDoc/internal/tree"
)

// Names of the folder source in the search (ADR-24): the source, and the collection its pages count as.
const (
	SourceFolder     = "folder"
	FolderCollection = "~folder"
	// FolderLabel names the root of the folder source in the workspace.
	FolderLabel = "Home"
)

// SearchSource returns the pages of the folder as a source of the search. Everyone signed in reads them.
func (s *Store) SearchSource() search.Source { return folderSource{s} }

type folderSource struct{ store *Store }

func (folderSource) Name() string { return SourceFolder }

func (f folderSource) Keys(context.Context) ([]string, error) {
	lib := f.store.Library()
	if lib == nil {
		return nil, nil
	}
	keys := make([]string, 0, len(lib.documents))
	for id := range lib.documents {
		keys = append(keys, string(id))
	}
	return keys, nil
}

func (f folderSource) Docs(_ context.Context, key string) ([]search.Doc, error) {
	lib := f.store.Library()
	if lib == nil {
		return nil, nil
	}
	doc, ok := lib.Document(document.ID(key))
	if !ok {
		return nil, nil
	}
	base := search.Doc{Object: key, Type: search.TypePage, Title: doc.Title, Tags: doc.Tags, Path: lib.Place(doc.ID),
		Collection: FolderCollection, CollectionName: FolderLabel, Changed: doc.Modified.Unix(), Readers: []string{search.Everyone}}
	return search.SectionDocs(base, search.Sections(search.PieceParts(lib.Outline(doc.ID), doc.Title))), nil
}

// Place names the folders a document lies in, from the root down, or the root's label at the top.
func (lib *Library) Place(id document.ID) string {
	folder, ok := lib.Tree().Location(id)
	if !ok || folder == lib.Tree().Root() {
		return FolderLabel
	}
	var names []string
	for _, f := range tree.Ancestors(folder)[1:] {
		names = append(names, f.Name)
	}
	return strings.Join(names, " › ")
}

// watcher hands the keys of changed documents to the search.
type watcher struct {
	mu     sync.Mutex
	notify func([]string)
	seen   map[document.ID]string
}

// Watch calls notify with the IDs of every document now, and later with those that changed or went whenever the folder
// changes, so the search queue gets them.
func (s *Store) Watch(notify func(ids []string)) {
	s.watch.mu.Lock()
	s.watch.notify = notify
	s.watch.seen = nil
	s.watch.mu.Unlock()
	s.changed(s.Library())
}

func (s *Store) changed(lib *Library) {
	s.watch.mu.Lock()
	defer s.watch.mu.Unlock()
	if s.watch.notify == nil || lib == nil {
		return
	}
	now := make(map[document.ID]string, len(lib.documents))
	var keys []string
	for id, doc := range lib.documents {
		sum := sha256.New()
		_, _ = fmt.Fprintf(sum, "%s\x00%s\x00%v\x00%s\x00%d\x00%v", doc.Title, doc.Text, doc.Tags, lib.Place(id), doc.Modified.Unix(), lib.Outline(id))
		now[id] = hex.EncodeToString(sum.Sum(nil))
		if s.watch.seen[id] != now[id] {
			keys = append(keys, string(id))
		}
	}
	for id := range s.watch.seen {
		if _, still := now[id]; !still {
			keys = append(keys, string(id))
		}
	}
	s.watch.seen = now
	if len(keys) > 0 {
		s.watch.notify(keys)
	}
}

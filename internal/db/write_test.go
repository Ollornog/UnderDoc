// SPDX-License-Identifier: MIT

package db

import (
	"errors"
	"slices"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/pagekind"
)

// TestWriteBlocks: a page written as a whole against its version keeps the blocks it names, makes the others, deletes
// what it leaves out and puts them in its order; a section only touches its blocks; a dry run keeps nothing; a page
// changed since is refused.
func TestWriteBlocks(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.createShared(ctx, anna, "Team")
	page, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	var ids []string
	after := ""
	for _, body := range []string{"Intro.", "## One", "First.", "## Two", "Second."} {
		b, err := db.AddBlock(ctx, page.ID, anna.ID, after, body)
		if err != nil {
			t.Fatal(err)
		}
		ids, after = append(ids, b.ID), b.ID
	}
	v, err := db.PageVersion(ctx, page.ID, anna.ID)
	if err != nil || v == 0 {
		t.Fatalf("version: %d %v", v, err)
	}
	if v2, err := db.PageVersion(ctx, page.ID, carl.ID); err != nil || v2 != v {
		t.Errorf("carl reads the version as anna does: %d %v", v2, err)
	}
	whole := []Block{{ID: ids[0], Body: "Intro."}, {ID: ids[3], Body: "## Two"}, {Body: "New."}, {ID: ids[1], Body: "## One!"}, {ID: "b99", Body: "Made up."}}
	dry, err := db.WriteBlocks(ctx, page.ID, anna.ID, v, "", whole, true)
	if err != nil || !dry.DryRun || dry.Version != v {
		t.Fatalf("dry run: %+v %v", dry, err)
	}
	if now, _ := db.PageVersion(ctx, page.ID, anna.ID); now != v {
		t.Errorf("the dry run kept something: %d → %d", v, now)
	}
	got, err := db.WriteBlocks(ctx, page.ID, anna.ID, v, "", whole, false)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, b := range got.Blocks {
		order = append(order, b.Body)
	}
	if !slices.Equal(order, []string{"Intro.", "## Two", "New.", "## One!", "Made up."}) || got.Version <= v {
		t.Errorf("the page after the write: %v, version %d", order, got.Version)
	}
	kinds := map[string]int{}
	for _, c := range got.Changes {
		kinds[c.Kind]++
	}
	if kinds[ChangeAdded] != 2 || kinds[ChangeChanged] != 1 || kinds[ChangeDeleted] != 2 || kinds[ChangeKept] != 2 {
		t.Errorf("the changes: %+v", got.Changes)
	}
	if slices.ContainsFunc(got.Blocks, func(b Block) bool { return b.ID == "b99" }) {
		t.Error("a block took an ID a writer made up")
	}
	if _, err := db.WriteBlocks(ctx, page.ID, anna.ID, v, "", whole, false); !errors.As(err, new(*ErrChanged)) {
		t.Errorf("a write against an old version: %v", err)
	}
	// The section of "## Two" ends at "## One!": what follows stays.
	two, err := db.WriteBlocks(ctx, page.ID, anna.ID, got.Version, ids[3], []Block{{ID: ids[3], Body: "## Two"}, {Body: "Newer."}}, false)
	if err != nil {
		t.Fatal(err)
	}
	order = order[:0]
	for _, b := range two.Blocks {
		order = append(order, b.Body)
	}
	if !slices.Equal(order, []string{"Intro.", "## Two", "Newer.", "## One!", "Made up."}) {
		t.Errorf("the page after writing the section of Two: %v", order)
	}
	// The section of "## One!", which ends the page with "Made up.".
	v = two.Version
	section, err := db.WriteBlocks(ctx, page.ID, anna.ID, v, ids[1], []Block{{ID: ids[1], Body: "## One"}, {ID: ids[0], Body: "Not the intro."}}, false)
	if err != nil {
		t.Fatal(err)
	}
	order = order[:0]
	for _, b := range section.Blocks {
		order = append(order, b.Body)
	}
	if !slices.Equal(order, []string{"Intro.", "## Two", "Newer.", "## One", "Not the intro."}) || section.Blocks[0].ID != ids[0] {
		t.Errorf("the page after writing a section: %v %+v", order, section.Blocks)
	}
	// Bob reads the page as everyone does, but writes nothing; a block that breaks a rule is refused.
	if _, err := db.SetAccess(ctx, team.ID, Actor{ID: anna.ID}, "read"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.WriteBlocks(ctx, page.ID, bob.ID, section.Version, "", nil, true); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("bob writes, as a dry run: %v", err)
	}
	if _, err := db.WriteBlocks(ctx, page.ID, anna.ID, section.Version, "", []Block{{Body: "---\nkind: how-to\n---\nx"}}, false); !errors.Is(err, ErrRule) {
		t.Errorf("front matter in a block: %v", err)
	}
}

// TestLinksAndFilesInTheCheck: a link to an object that finds nothing is "link to nowhere", a file attached that no
// block shows is "file nobody links to", and a page beyond 25 KB says so (ADR-34).
func TestLinksAndFilesInTheCheck(t *testing.T) {
	db, ctx := setup(t)
	if err := db.EnsureRoots(ctx, anna, false, ""); err != nil {
		t.Fatal(err)
	}
	team, _ := db.createShared(ctx, anna, "Team")
	page, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	private, _ := db.CreatePage(ctx, anna, "Mine", team.ID)
	if err := db.SetEntries(ctx, private.ID, admin, ToPerson, bob.ID, entries(ActRead, Deny), true); err != nil {
		t.Fatal(err)
	}
	upload(t, db, anna.ID, UploadTarget{Page: page.ID}, "shown.png", 7, sha(1))
	upload(t, db, anna.ID, UploadTarget{Page: page.ID}, "forgotten.pdf", 8, sha(2))
	body := "See [the team](/o/" + team.ID + "), [mine](underdoc:" + private.ID + "), [gone](/o/zzzzzzzzzz), [a block](/o/" + page.ID + "#b9) " +
		"and ![shot](" + AttachmentPath(page.ID, "a1") + ").\n\n```\n[in code](/o/yyyyyyyyyy)\n```"
	if _, err := db.AddBlock(ctx, page.ID, anna.ID, "", body); err != nil {
		t.Fatal(err)
	}
	codes := func(viewer string) map[string][]string {
		t.Helper()
		found, err := db.Findings(ctx, page.ID, viewer)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string][]string{}
		for _, f := range found {
			out[f.Code] = append(out[f.Code], f.Message)
		}
		return out
	}
	annas := codes(anna.ID)
	if len(annas[pagekind.CodeLink]) != 2 || len(annas[pagekind.CodeUnlinkedFile]) != 1 || annas[pagekind.CodeUnlinkedFile][0] != "The file forgotten.pdf is attached, but nothing in the page links to it." {
		t.Errorf("anna's check: %v", annas)
	}
	if bobs := codes(bob.ID); len(bobs[pagekind.CodeLink]) != 3 {
		t.Errorf("for bob the link to what he may not read leads nowhere: %v", bobs)
	}
	if _, err := db.AddBlock(ctx, page.ID, anna.ID, "", longText(26<<10)); err != nil {
		t.Fatal(err)
	}
	if len(codes(anna.ID)[pagekind.CodePageSize]) != 1 {
		t.Error("a page beyond 25 KB is not told")
	}
}

func longText(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(i%26)
		if i%80 == 79 {
			b[i] = ' '
		}
	}
	return string(b)
}

// TestUploadLinks: a one-time link works once and only in its time, only for an upload of its owner that has received
// nothing, and keeps only its hash (ADR-34).
func TestUploadLinks(t *testing.T) {
	db, ctx := setup(t)
	if err := db.EnsureRoots(ctx, anna, false, ""); err != nil {
		t.Fatal(err)
	}
	mine := rootOfRole(t, db, anna.ID, RootUser)
	u, err := db.StartUpload(ctx, anna.ID, UploadTarget{Parent: mine.ID}, "a.txt", 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.LinkUpload(ctx, u.ID, bob.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob links anna's upload: %v", err)
	}
	token, err := db.LinkUpload(ctx, u.ID, anna.ID)
	if err != nil || len(token) != 43 {
		t.Fatalf("link: %q %v", token, err)
	}
	var stored int
	if err := db.pool.QueryRow(ctx, "SELECT count(*) FROM uploads WHERE link_hash = $1::bytea", []byte(token)).Scan(&stored); err != nil || stored != 0 {
		t.Errorf("the token itself is kept: %d %v", stored, err)
	}
	got, err := db.TakeUploadLink(ctx, token)
	if err != nil || got.ID != u.ID || got.Owner != anna.ID || got.Size != 3 {
		t.Fatalf("take: %+v %v", got, err)
	}
	if _, err := db.TakeUploadLink(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Errorf("the link a second time: %v", err)
	}
	token, _ = db.LinkUpload(ctx, u.ID, anna.ID)
	if _, err := db.pool.Exec(ctx, "UPDATE uploads SET link_until = now() - interval '1 second' WHERE id = $1", u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.TakeUploadLink(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Errorf("a link past its time: %v", err)
	}
	token, _ = db.LinkUpload(ctx, u.ID, anna.ID)
	if err := db.Received(ctx, u.ID, anna.ID, 0, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.TakeUploadLink(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Errorf("a link of an upload that received something: %v", err)
	}
}

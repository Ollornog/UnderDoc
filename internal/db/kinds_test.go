// SPDX-License-Identifier: MIT

package db

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/pagekind"
)

func findingCodes(found []pagekind.Finding) []string {
	var out []string
	for _, f := range found {
		out = append(out, f.Code)
	}
	return out
}

// TestPageKinds: a page takes a kind and gives it up, from a template or set later; only a page has one, only a kind
// there is, and only whoever may change the page sets it (T-20).
func TestPageKinds(t *testing.T) {
	db := open(t)
	ctx := inMain()
	team, _ := db.createShared(ctx, anna, "Team")
	page, err := db.CreatePageOf(ctx, anna, "Restart", team.ID, "", "How-to")
	if err != nil || page.PageKind != pagekind.HowTo {
		t.Fatalf("a page made with a kind: %+v %v", page, err)
	}
	if got, _ := db.Object(ctx, page.ID, anna.ID); got.PageKind != pagekind.HowTo {
		t.Errorf("the kind is kept: %q", got.PageKind)
	}
	if _, err := db.CreatePageOf(ctx, anna, "Odd", team.ID, "", "manual"); !errors.Is(err, ErrKind) {
		t.Errorf("an unknown kind: %v", err)
	}
	from, _ := db.CreatePageFrom(ctx, anna, "Notes of today", team.ID, BuiltIn+"meeting")
	if from.PageKind != pagekind.Record {
		t.Errorf("a page from the meeting template is a record: %q", from.PageKind)
	}
	set, err := db.SetPageKind(ctx, page.ID, anna.ID, "Referenz")
	if err != nil || set.PageKind != pagekind.Reference {
		t.Fatalf("set in German: %+v %v", set, err)
	}
	if _, err := db.SetTemplate(ctx, page.ID, anna.ID, true); err != nil {
		t.Fatal(err)
	}
	made, _ := db.CreatePageFrom(ctx, bob, "Second", team.ID, page.ID)
	if made.PageKind != pagekind.Reference {
		t.Errorf("a page from a page offered as a template takes its kind: %q", made.PageKind)
	}
	if set, err := db.SetPageKind(ctx, page.ID, anna.ID, ""); err != nil || set.PageKind != "" {
		t.Errorf("no kind again: %+v %v", set, err)
	}
	if _, err := db.SetPageKind(ctx, team.ID, anna.ID, "how-to"); !errors.Is(err, ErrKind) {
		t.Errorf("a collection takes no kind: %v", err)
	}
	if _, err := db.SetPageKind(ctx, page.ID, anna.ID, "notes"); !errors.Is(err, ErrKind) {
		t.Errorf("notes is no kind of page: %v", err)
	}
	shown, _ := db.CreateCollection(ctx, anna, "Shown", "")
	if err := db.SetGrant(ctx, shown.ID, Actor{ID: anna.ID}, ToTenant, "", RoleRead); err != nil {
		t.Fatal(err)
	}
	read, _ := db.CreatePage(ctx, anna, "Read only", shown.ID)
	if _, err := db.SetPageKind(ctx, read.ID, bob.ID, "how-to"); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("a reader sets no kind: %v", err)
	}
	copied, err := db.Copy(ctx, made.ID, anna.ID, team.ID, "")
	if err != nil || copied.PageKind != pagekind.Reference {
		t.Errorf("a copy keeps the kind: %+v %v", copied, err)
	}
}

// TestBlockingRules: front matter in a block and a part ID used twice are refused at every write of a block, with the
// finding and its fix; everything else is written (T-20).
func TestBlockingRules(t *testing.T) {
	db := open(t)
	ctx := inMain()
	team, _ := db.createShared(ctx, anna, "Team")
	page, _ := db.CreatePageOf(ctx, anna, "Guide", team.ID, "", pagekind.HowTo)
	_, err := db.AddBlock(ctx, page.ID, anna.ID, "", "---\nkind: how-to\ndescription: A guide\n---\nThe text.")
	var rule *RuleError
	if !errors.As(err, &rule) || !errors.Is(err, ErrRule) || len(rule.Findings) != 1 || rule.Findings[0].Code != pagekind.CodeFrontMatter ||
		rule.Findings[0].Fix == "" {
		t.Fatalf("front matter in a new block: %v", err)
	}
	b, err := db.AddBlock(ctx, page.ID, anna.ID, "", "- a list first, only a warning")
	if err != nil {
		t.Fatalf("a warning blocks nothing: %v", err)
	}
	_, err = db.EditBlock(ctx, page.ID, b.ID, anna.ID, "<!-- part t1 -->a<!-- /part t1 --> and <!-- part t1 -->b<!-- /part t1 -->")
	if !errors.As(err, &rule) || rule.Findings[0].Code != pagekind.CodeID || rule.Findings[0].Block != b.ID {
		t.Fatalf("a part ID used twice: %v", err)
	}
	if blocks, _ := db.Blocks(ctx, page.ID, anna.ID); len(blocks) != 1 || blocks[0].Body != "- a list first, only a warning" {
		t.Errorf("nothing refused was written: %q", bodies(blocks))
	}
}

// TestFindings: the check of a page names what it breaks, copied values with the reference they come from as far as the
// viewer reads it, and embeddings that find nothing (T-20).
func TestFindings(t *testing.T) {
	db := open(t)
	ctx := inMain()
	team, _ := db.createShared(ctx, anna, "Team")
	ref, _ := db.CreatePageOf(ctx, anna, "Network", team.ID, "", pagekind.Reference)
	addresses, _ := db.AddBlock(ctx, ref.ID, anna.ID, "", "## Backup\n\nThe backup host is <!-- part t1 -->backup.example.com<!-- /part t1 -->.")
	mine, _ := db.CreateCollection(ctx, anna, "Mine", "")
	secret, _ := db.CreatePageOf(ctx, anna, "Hidden", mine.ID, "", pagekind.Record)
	_, _ = db.AddBlock(ctx, secret.ID, anna.ID, "", "Vault at 198.51.100.7.")
	guide, _ := db.CreatePageOf(ctx, bob, "Restore", team.ID, "", pagekind.HowTo)
	first, _ := db.AddBlock(ctx, guide.ID, bob.ID, "", "This guide brings files back from backup.example.com and from 198.51.100.7.")
	_, _ = db.AddBlock(ctx, guide.ID, bob.ID, first.ID, "::embed{ref=\""+ref.ID+"#b9\"}\n\n::embed{ref=\""+secret.ID+"\"}")

	found, err := db.Findings(ctx, guide.ID, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := findingCodes(found); !slices.Equal(got, []string{pagekind.CodeSteps, pagekind.CodeCopy, pagekind.CodeEmbed, pagekind.CodeEmbed}) {
		t.Fatalf("findings for bob: %v %+v", got, found)
	}
	if found[1].Source != ref.ID+"#"+addresses.ID+".t1" || !strings.Contains(found[1].Message, "Network") || found[1].Block != first.ID {
		t.Errorf("COPY names the part in the reference: %+v", found[1])
	}
	if strings.Contains(found[2].Source, "#b9") == false {
		t.Errorf("EMBED names the address: %+v", found[2])
	}
	if found[3].Source != secret.ID {
		t.Errorf("a page bob does not see finds nothing for him: %+v", found[3])
	}
	// anna reads the record bob does not: for her, the second value is copied too, and its embedding finds it.
	found, _ = db.Findings(ctx, guide.ID, anna.ID)
	if got := findingCodes(found); !slices.Equal(got, []string{pagekind.CodeSteps, pagekind.CodeCopy, pagekind.CodeCopy, pagekind.CodeEmbed}) {
		t.Errorf("findings for anna: %v", got)
	}
	if _, err := db.Findings(ctx, secret.ID, bob.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("no findings of a page one does not see: %v", err)
	}
	// A page of a kind that does not compare values has no COPY.
	if _, err := db.SetPageKind(ctx, guide.ID, bob.ID, pagekind.Tutorial); err != nil {
		t.Fatal(err)
	}
	if got := findingCodes(must(db.Findings(ctx, guide.ID, bob.ID))); !slices.Equal(got, []string{pagekind.CodeEmbed, pagekind.CodeEmbed}) {
		t.Errorf("a tutorial: %v", got)
	}
}

// TestRuleProblems: the list of problems names every page that breaks the rules of its kind and folders without an
// overview or with too many pages, only in what the viewer sees (T-20).
func TestRuleProblems(t *testing.T) {
	db := open(t)
	ctx := inMain()
	team, _ := db.createShared(ctx, anna, "Team")
	folder, _ := db.CreateFolder(ctx, anna, "Guides", team.ID)
	for i := range pagekind.MaxPagesInFolder + 1 {
		p, _ := db.CreatePageOf(ctx, anna, "Guide "+strconv.Itoa(i), folder.ID, "", pagekind.Tutorial)
		if i == 0 {
			_, _ = db.AddBlock(ctx, p.ID, anna.ID, "", "- starts with a list")
		}
	}
	good, _ := db.CreateFolder(ctx, anna, "Fine", team.ID)
	_, _ = db.CreatePageOf(ctx, anna, "About", good.ID, "", pagekind.Overview)
	_, _ = db.CreatePageOf(ctx, anna, "One", good.ID, "", pagekind.Tutorial)
	mine, _ := db.CreateCollection(ctx, anna, "Mine", "")
	hidden, _ := db.CreatePage(ctx, anna, "Private", mine.ID)
	_, _ = db.AddBlock(ctx, hidden.ID, anna.ID, "", "Text without a kind.")

	got := map[string][]string{}
	list, more, err := db.RuleProblems(ctx, bob.ID)
	if err != nil || more {
		t.Fatal(err, more)
	}
	for _, p := range list {
		got[p.Title] = findingCodes(p.Findings)
	}
	want := map[string][]string{"Guide 0": {pagekind.CodeStart}, "Guides": {pagekind.CodeOverview, pagekind.CodeFolderSize}}
	if len(got) != len(want) || !slices.Equal(got["Guide 0"], want["Guide 0"]) || !slices.Equal(got["Guides"], want["Guides"]) {
		t.Errorf("problems for bob: %v", got)
	}
	list, _, _ = db.RuleProblems(ctx, anna.ID)
	if !slices.ContainsFunc(list, func(p PageFindings) bool {
		return p.Title == "Private" && p.Findings[0].Code == pagekind.CodeKindMissing
	}) {
		t.Errorf("anna sees her own page without a kind: %+v", list)
	}
}

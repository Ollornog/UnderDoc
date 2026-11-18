// SPDX-License-Identifier: MIT

package db

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTextWaysAreLayered(t *testing.T) {
	db := open(t)
	ctx := inMain()
	off, on := false, true
	all := Ways{Browser: true, Server: true, AI: true}
	if got, _ := db.TextWays(ctx, Ways{Browser: true}, anna.ID); got != (Ways{Browser: true}) {
		t.Fatalf("only what the hoster allows: %+v", got)
	}
	if err := db.SetTextSettings(ctx, "", WaySettings{AI: &off, Browser: &off}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetTextSettings(ctx, anna.ID, WaySettings{Browser: &off, AI: &on}); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.TextWays(ctx, all, anna.ID); got != (Ways{Server: true}) {
		t.Fatalf("anna: the tenant switched AI and the browser off, her own on does not count: %+v", got)
	}
	if got, _ := db.TextWays(ctx, all, bob.ID); got != (Ways{Server: true}) {
		t.Fatalf("bob: %+v", got)
	}
}

func TestTextQueueAndTexts(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if err := db.EnsureRoots(ctx, anna, false, ""); err != nil {
		t.Fatal(err)
	}
	mine := rootOfRole(t, db, anna.ID, RootUser)
	file := upload(t, db, anna.ID, UploadTarget{Parent: mine.ID}, "a.pdf", 10, sha(1)).File
	if err := db.QueueText(ctx, sha(1), "application/pdf", anna.ID, "server", "server", "ai"); err != nil {
		t.Fatal(err)
	}
	first, err := db.ClaimTextJob(ctx, time.Minute)
	if err != nil || first.Blob != sha(1) || first.Tries != 1 {
		t.Fatalf("%+v %v", first, err)
	}
	second, err := db.ClaimTextJob(ctx, time.Minute)
	if err != nil || second.Way == first.Way {
		t.Fatalf("the second job: %+v %v", second, err)
	}
	if _, err := db.ClaimTextJob(ctx, time.Minute); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a claimed job is held: %v", err)
	}
	if err := db.FinishTextJob(ctx, first, first.Way, "  Card payment E-1042  ", nil); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishTextJob(ctx, second, "", "", errors.New("model down")); err != nil {
		t.Fatal(err)
	}
	texts, err := db.FileTexts(ctx, file.ID, anna.ID)
	if err != nil || len(texts) != 1 || texts[0].Text != "Card payment E-1042" || texts[0].Source != first.Way {
		t.Fatalf("%+v %v", texts, err)
	}
	// The browser of a user who may change the file gives its text too; others may not.
	if err := db.SaveFileText(ctx, file.ID, anna.ID, "browser", strings.Repeat("ä", MaxFileText)); err != nil {
		t.Fatal(err)
	}
	if texts, _ := db.FileTexts(ctx, file.ID, anna.ID); len(texts) != 2 || len(texts[1].Text) > MaxFileText || texts[1].Source != "browser" {
		t.Fatalf("%d texts, the second %d bytes", len(texts), len(texts[1].Text))
	}
	if err := db.SaveFileText(ctx, file.ID, bob.ID, "browser", "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob gave a text to anna's file: %v", err)
	}
	if err := db.SaveFileText(ctx, file.ID, anna.ID, "mind reading", "x"); !errors.Is(err, ErrText) {
		t.Fatalf("an unknown source: %v", err)
	}
	// The text goes with its blob.
	_ = db.Delete(ctx, file.ID, anna.ID)
	_ = db.Purge(ctx, file.ID, anna.ID)
	if _, err := db.CollectBlobs(ctx, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	var left int
	_ = db.pool.QueryRow(ctx, "SELECT count(*) FROM file_texts").Scan(&left)
	if left != 0 {
		t.Fatalf("%d texts stayed without their blob", left)
	}
}

func TestAIQuota(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if err := db.UseAIPages(ctx, anna.ID, 3, 5, 8); err != nil {
		t.Fatal(err)
	}
	if err := db.UseAIPages(ctx, anna.ID, 3, 5, 8); !errors.Is(err, ErrQuota) {
		t.Fatalf("past the user's quota: %v", err)
	}
	if err := db.UseAIPages(ctx, bob.ID, 5, 5, 7); !errors.Is(err, ErrQuota) {
		t.Fatalf("past the tenant's quota: %v", err)
	}
	if err := db.UseAIPages(ctx, bob.ID, 5, 5, 0); err != nil {
		t.Fatalf("without a tenant quota: %v", err)
	}
}

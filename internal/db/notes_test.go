// SPDX-License-Identifier: MIT

package db

import (
	"sync"
	"testing"
)

func TestQuickNotes(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if err := db.SeeAccount(ctx, anna); err != nil {
		t.Fatal(err)
	}
	// Two first notes at once make one board.
	var wg sync.WaitGroup
	boards := make([]string, 2)
	for i := range boards {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, _, err := db.AddNote(ctx, anna, "note "+string(rune('a'+i)))
			if err != nil {
				t.Error(err)
			}
			boards[i] = b.ID
		}()
	}
	wg.Wait()
	if boards[0] == "" || boards[0] != boards[1] {
		t.Fatalf("one board: %q", boards)
	}
	board, card, err := db.AddNote(ctx, anna, "Call the crane company")
	if err != nil || board.ID != boards[0] || board.Kind != KindNotes || board.Title != QuickNotes {
		t.Fatalf("the board: %+v %v", board, err)
	}
	notes, _ := db.Collections(ctx, anna.ID)
	if !notes[0].Private || board.Parent != notes[0].ID {
		t.Errorf("the board lies in the private notes: %+v %+v", board, notes[0])
	}
	cards, _ := db.Blocks(ctx, board.ID, anna.ID)
	if len(cards) != 3 || cards[0].ID != card.ID || cards[0].Body != "Call the crane company" {
		t.Errorf("the newest card comes first: %+v", cards)
	}
	if _, err := db.Object(ctx, board.ID, bob.ID); err == nil {
		t.Errorf("bob sees anna's quick notes")
	}
	// Cards are blocks: edited, moved and deleted as blocks of a page.
	if _, err := db.EditBlock(ctx, board.ID, card.ID, anna.ID, "Call the crane company on Monday"); err != nil {
		t.Errorf("edit a card: %v", err)
	}
	if err := db.DeleteBlock(ctx, board.ID, cards[2].ID, anna.ID); err != nil {
		t.Errorf("delete a card: %v", err)
	}
	if _, err := db.SetTemplate(ctx, board.ID, anna.ID, true); err == nil {
		t.Errorf("a board of quick notes is no template")
	}
	// Without private notes, the first note makes them.
	if _, _, err := db.AddNote(ctx, bob, "mine"); err != nil {
		t.Fatal(err)
	}
	if mine, _ := db.Collections(ctx, bob.ID); len(mine) == 0 || !mine[0].Private || mine[0].Title != MyNotes {
		t.Errorf("bob's private notes: %+v", mine)
	}
}

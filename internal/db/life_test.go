// SPDX-License-Identifier: MIT

package db

import (
	"errors"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// The ways of ADR-22 write blocks too: a copy in every use that grows a page, and a new master, keep the limit on storage
// of the tenant, and a refused way changes nothing.
func TestTheWaysKeepTheLimit(t *testing.T) {
	db := open(t)
	ctx := inMain()
	team, _ := db.createShared(ctx, anna, "Team")
	src, _ := db.CreatePage(ctx, anna, "Source", team.ID)
	user, _ := db.CreatePage(ctx, anna, "User", team.ID)
	if _, err := db.AddBlock(ctx, src.ID, anna.ID, "", strings.Repeat("s", 60)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddBlock(ctx, user.ID, anna.ID, "", `::embed{ref="`+src.ID+`#b1"}`); err != nil {
		t.Fatal(err)
	}
	mixed, _ := db.CreatePage(ctx, anna, "Mixed", team.ID)
	if _, err := db.AddBlock(ctx, mixed.ID, anna.ID, "", "Text.\n\n::embed{ref=\""+src.ID+"#b1\"}"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetTenantLimits(ctx, tenant.Main, HosterLimits{MaxStorage: ptr(int64(1))}); err != nil {
		t.Fatal(err)
	}
	for _, way := range []Way{{Way: WayCopies}, {Way: WayMaster, Master: user.ID + "#b1"}, {Way: WayMaster, Master: mixed.ID + "#b1"}} {
		if err := db.DeleteBlockWays(ctx, src.ID, "b1", anna.ID, map[string]Way{"*": way}); !errors.Is(err, ErrQuota) {
			t.Errorf("%s past the limit: %v", way.Way, err)
		}
	}
	if b, _ := db.Blocks(ctx, src.ID, anna.ID); len(b) != 1 {
		t.Errorf("a refused way leaves the source as it was: %v", b)
	}
	if b, _ := db.Blocks(ctx, user.ID, anna.ID); len(b) != 1 || !strings.HasPrefix(b[0].Body, "::embed") {
		t.Errorf("and the use: %v", b)
	}
}

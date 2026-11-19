// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/Ollornog/EightPerDay/core"
	epdweb "github.com/Ollornog/EightPerDay/web"
	epdauth "github.com/Ollornog/EightPerDay/web/auth"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/module"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// The time tracking (module Time, docs/time-tracking.md) is EightPerDay: its interface runs below /time, inside the
// workspace, with the account of UnderDoc's sign-in and in the tenant of the request. Its tables are in this database
// (db.TimeStore), so row-level security keeps the tenants apart as everywhere else. The tenant is an organization
// there, its accounts are persons, and its administrators administer the organization (db.MapTenant).

// timeBase is where the time tracking answers.
const timeBase = "/time"

// timeTheme is where the workspace keeps its theme (assets/app/theme.js), so the frame follows it.
const timeTheme = "ud.theme"

// timeTracking returns the handler of the time tracking, behind the guard of its module and the mapping of the tenant.
func (h *handler) timeTracking() (http.Handler, error) {
	inner, err := epdweb.New(slog.New(quiet{h.log.Handler()}), epdweb.Options{
		Store: h.options.DB.TimeStore(),
		Embedding: &epdweb.Embedding{
			Base:      timeBase,
			ThemeKey:  timeTheme,
			Directory: timeDirectory{h.options.DB},
			Account: func(r *http.Request) *epdauth.Account {
				user, ok := account.From(r.Context())
				if !ok {
					return nil
				}
				return &epdauth.Account{ID: db.Person(user.ID), Username: user.Login, DisplayName: user.Name}
			},
		},
	})
	if err != nil {
		return nil, err
	}
	mapped := h.mapTenant(inner)
	return h.guard(module.Time, mapped.ServeHTTP), nil
}

// mapTenant keeps the organization of the tenant and the membership of its administrators in the time tracking
// before a request goes in, at most every few minutes per account.
func (h *handler) mapTenant(next http.Handler) http.Handler {
	var mu sync.Mutex
	done := map[string]time.Time{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := h.me(r)
		t, _ := tenant.From(r.Context())
		key := t + "\x00" + user.ID
		mu.Lock()
		last, ok := done[key]
		mu.Unlock()
		if !ok || time.Since(last) > 5*time.Minute {
			if err := h.options.DB.MapTenant(r.Context(), user.ID, user.Admin); err != nil {
				h.log.Error("time tracking: tenant not mapped", slog.String("error", err.Error()))
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			mu.Lock()
			done[key] = time.Now()
			mu.Unlock()
		}
		next.ServeHTTP(w, r)
	})
}

// timeDirectory names the accounts of the tenant for the time tracking.
type timeDirectory struct{ db *db.DB }

func timeAccount(a db.TimeAccount) *epdauth.Account {
	return &epdauth.Account{ID: a.Person, Username: a.Login, DisplayName: a.Name, Language: a.Language}
}

func (d timeDirectory) Account(ctx context.Context, person string) (*epdauth.Account, error) {
	a, err := d.db.TimeAccount(ctx, person)
	if err != nil {
		return nil, err
	}
	return timeAccount(a), nil
}

func (d timeDirectory) Accounts(ctx context.Context) ([]*epdauth.Account, error) {
	all, err := d.db.TimeAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*epdauth.Account, len(all))
	for i, a := range all {
		out[i] = timeAccount(a)
	}
	return out, nil
}

// quiet passes on what the time tracking logs as a warning or worse; its line per request is UnderDoc's already.
type quiet struct{ slog.Handler }

func (q quiet) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= slog.LevelWarn && q.Handler.Enabled(ctx, level)
}

func (q quiet) WithAttrs(attrs []slog.Attr) slog.Handler { return quiet{q.Handler.WithAttrs(attrs)} }
func (q quiet) WithGroup(name string) slog.Handler       { return quiet{q.Handler.WithGroup(name)} }

// apiTimeAbsences lists the absences of the user of the request between from and to (dates, at most two years apart),
// over all their employers, private and managed, for a calendar (M-18): core.CalendarAbsences, without notes. Only the
// person sees their own; what an organization sees of its members stays in the time tracking.
func (h *handler) apiTimeAbsences(w http.ResponseWriter, r *http.Request) {
	from, err1 := core.ParseDate(r.URL.Query().Get("from"))
	to, err2 := core.ParseDate(r.URL.Query().Get("to"))
	if err1 != nil || err2 != nil || to.Before(from) || to.After(from.AddDays(2*366)) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from and to are dates (2026-11-19), at most two years apart"})
		return
	}
	data, err := core.Load(r.Context(), h.options.DB.TimeStore(), db.Person(h.me(r).ID), from, to)
	if err != nil {
		h.log.Error("time tracking: absences failed", slog.String("error", err.Error()))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"absences": core.CalendarAbsences(data, from, to)})
}

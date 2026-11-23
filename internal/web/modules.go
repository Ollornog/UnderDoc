// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"log/slog"
	"net/http"
	"slices"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/module"
)

// Modules (ADR-25): the hoster allows them per instance and per tenant, the administrator of a tenant switches them,
// and each user hides what is on. A module that is off has no route: its addresses answer 404 as if they never were,
// its entries are gone from the workspace, and its jobs do not run. Its data stays as it was.

// tenantModules returns what the tenant of ctx set, without the choice of a user.
func (h *handler) tenantModules(ctx context.Context) module.Tenant {
	if h.options.DB == nil {
		return module.Tenant{}
	}
	allowed, on, err := h.options.DB.TenantModules(ctx)
	if err != nil {
		h.log.Error("modules failed", slog.String("error", err.Error()))
		return module.Tenant{Allowed: []string{}}
	}
	return module.Tenant{Allowed: allowed, On: on}
}

// moduleStates returns every module as the user of the request has it.
func (h *handler) moduleStates(r *http.Request) []module.State {
	ctx := r.Context()
	t := h.tenantModules(ctx)
	if h.options.DB != nil {
		hidden, err := h.options.DB.HiddenModules(ctx, h.me(r).ID)
		if err != nil {
			h.log.Error("hidden modules failed", slog.String("error", err.Error()))
		}
		t.Hidden = hidden
	}
	return h.options.Modules.States(t)
}

// ModuleOn returns whether a module is on for the tenant of a context, for the jobs that run outside a request.
func ModuleOn(database *db.DB, policy module.Policy, id string) func(context.Context) bool {
	return func(ctx context.Context) bool {
		allowed, on, err := database.TenantModules(ctx)
		return err == nil && policy.On(module.Tenant{Allowed: allowed, On: on}, id)
	}
}

// moduleOn tells whether a module is on for the tenant of ctx.
func (h *handler) moduleOn(ctx context.Context, id string) bool {
	return h.options.Modules.On(h.tenantModules(ctx), id)
}

// guard lets a request of a module through only while the module is on for its tenant.
func (h *handler) guard(id string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.moduleOn(r.Context(), id) {
			if len(r.URL.Path) > 5 && r.URL.Path[:5] == "/api/" {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
				return
			}
			h.notFound(w, r)
			return
		}
		next(w, r)
	}
}

type apiModule struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Allowed     bool   `json:"allowed"`
	On          bool   `json:"on"`
	Hidden      bool   `json:"hidden"`
}

func apiModules(states []module.State) []apiModule {
	out := make([]apiModule, 0, len(states))
	for _, s := range states {
		out = append(out, apiModule{ID: s.ID, Name: s.Name, Description: s.Description, Allowed: s.Allowed, On: s.On, Hidden: s.Hidden})
	}
	return out
}

// --- Pages

type modulesView struct {
	Version string
	Title   string
	Tenant  string
	Error   string
	Notice  string
	Modules []module.State
	Admin   bool
}

func (h *handler) modulesPage(w http.ResponseWriter, r *http.Request, name, title string, status int, notice string) {
	h.page(w, status, name, modulesView{Version: h.assetVersion, Title: title, Tenant: h.tenantName(r.Context()), Notice: notice,
		Modules: h.moduleStates(r), Admin: h.me(r).Admin})
}

// adminModules shows the modules of the tenant to its administrator.
func (h *handler) adminModules(w http.ResponseWriter, r *http.Request) {
	if !h.adminOnly(w, r) {
		return
	}
	h.modulesPage(w, r, "admin-modules", "Modules", http.StatusOK, "")
}

// saveModules switches the modules of the tenant. What the hoster does not allow stays off; nothing is deleted.
func (h *handler) saveModules(w http.ResponseWriter, r *http.Request) {
	if !h.adminOnly(w, r) || !h.formOK(w, r) {
		return
	}
	ctx := r.Context()
	var on []string
	for _, s := range h.options.Modules.States(h.tenantModules(ctx)) {
		if s.Allowed && slices.Contains(r.PostForm["module"], s.ID) {
			on = append(on, s.ID)
		}
	}
	if err := h.options.DB.SetTenantModules(ctx, on); err != nil {
		h.failedPage(w, err, "modules")
		return
	}
	h.log.Info("modules switched", slog.String("by", h.me(r).Login), slog.Any("on", on))
	h.modulesPage(w, r, "admin-modules", "Modules", http.StatusOK, "Saved. Nothing was deleted: a module switched on again has everything back.")
}

// myModules shows the modules that are on, for the user to hide or show.
func (h *handler) myModules(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		h.notFound(w, r)
		return
	}
	h.modulesPage(w, r, "my-modules", "My modules", http.StatusOK, "")
}

// saveMyModules keeps what the user hides.
func (h *handler) saveMyModules(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil || !h.formOK(w, r) {
		return
	}
	var hidden []string
	for _, id := range r.PostForm["hide"] {
		if _, ok := module.Find(id); ok {
			hidden = append(hidden, id)
		}
	}
	if err := h.options.DB.SetHiddenModules(r.Context(), h.me(r).ID, hidden); err != nil && err != db.ErrNotFound {
		h.failedPage(w, err, "modules")
		return
	}
	h.modulesPage(w, r, "my-modules", "My modules", http.StatusOK, "Saved.")
}

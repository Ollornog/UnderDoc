// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Ollornog/UnderDoc/internal/auth"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/module"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// The hoster administration (ADR-23): on a host of its own, the administrators of the hoster make tenants, lock them,
// archive them, set their limits and the modules they may use. They sign in as accounts of the reserved tenant hoster,
// and they see counts, never content: the hoster host has no route into a workspace, and a session of the hoster is a
// session of its own tenant, which row-level security keeps from every other.

// hosterPath tells the paths that answer on the hoster host: its pages, and signing in.
func hosterPath(r *http.Request) bool {
	p := r.URL.Path
	return p == "/hoster" || strings.HasPrefix(p, "/hoster/") || public(r) || p == "/sign-out" ||
		strings.HasPrefix(p, "/account/") || p == "/admin/sign-in" || p == "/admin/invites"
}

func (h *handler) hosterOnly(w http.ResponseWriter, r *http.Request) bool {
	if t, _ := tenant.From(r.Context()); t != tenant.Hoster || !h.me(r).Admin {
		h.notFound(w, r)
		return false
	}
	return true
}

type hosterView struct {
	Version string
	Title   string
	Tenant  string
	Error   string
	Notice  string
	Link    string
	Domain  string
	Tenants []hosterRow
	Edit    *hosterRow
	Modules []module.Module
}

type hosterRow struct {
	db.Tenant
	Users, MaxUsers     string
	Storage, MaxStorage string
	MaxUsersValue       string
	MaxStorageMB        string
	Allowed             map[string]bool
	AllModules          bool
	Reserved            bool
}

func megabytes(n int64) string { return strconv.FormatFloat(float64(n)/1e6, 'f', 1, 64) + " MB" }

func (h *handler) hosterRow(ctx context.Context, t db.Tenant) hosterRow {
	row := hosterRow{Tenant: t, MaxUsers: "no limit", MaxStorage: "no limit", AllModules: true, Reserved: t.ID == tenant.Main}
	if u, err := h.options.DB.TenantUsage(ctx, t.ID); err == nil {
		row.Users, row.Storage = strconv.Itoa(u.Users), megabytes(u.Storage)
	} else {
		h.log.Error("usage failed", slog.String("tenant", t.ID), slog.String("error", err.Error()))
	}
	if l, err := h.options.DB.TenantLimits(ctx, t.ID); err == nil {
		if l.MaxUsers != nil {
			row.MaxUsers, row.MaxUsersValue = strconv.Itoa(*l.MaxUsers), strconv.Itoa(*l.MaxUsers)
		}
		if l.MaxStorage != nil {
			row.MaxStorage = megabytes(*l.MaxStorage)
			row.MaxStorageMB = strconv.FormatInt(*l.MaxStorage/1_000_000, 10)
		}
		row.AllModules, row.Allowed = l.Modules == nil, map[string]bool{}
		for _, m := range l.Modules {
			row.Allowed[m] = true
		}
	}
	return row
}

func (h *handler) hosterPage(w http.ResponseWriter, r *http.Request, status int, problem, notice, link, edit string) {
	ctx := r.Context()
	v := hosterView{Version: h.assetVersion, Title: "Tenants", Tenant: "Hoster", Error: problem, Notice: notice, Link: link,
		Domain: h.options.TenantDomain, Modules: module.All}
	list, err := h.options.DB.Tenants(ctx)
	if err != nil {
		h.passkeyFailed(w, err, "tenants")
		return
	}
	for _, t := range list {
		row := h.hosterRow(ctx, t)
		v.Tenants = append(v.Tenants, row)
		if t.ID == edit {
			v.Edit = &row
		}
	}
	name := "hoster"
	if edit != "" {
		if v.Edit == nil {
			h.notFound(w, r)
			return
		}
		name, v.Title = "hoster-tenant", v.Edit.Name
	}
	h.page(w, status, name, v)
}

// hoster lists the tenants with what they use.
func (h *handler) hoster(w http.ResponseWriter, r *http.Request) {
	if !h.hosterOnly(w, r) {
		return
	}
	h.hosterPage(w, r, http.StatusOK, "", "", "", "")
}

// hosterTenant shows one tenant to change.
func (h *handler) hosterTenant(w http.ResponseWriter, r *http.Request) {
	if !h.hosterOnly(w, r) {
		return
	}
	h.hosterPage(w, r, http.StatusOK, "", "", "", r.PathValue("id"))
}

// tenantAddress is where a tenant is reached: its subdomain, or its first own domain.
func (h *handler) tenantAddress(r *http.Request, t db.Tenant) string {
	scheme := strings.SplitN(requestOrigin(r), "://", 2)[0]
	switch {
	case h.options.TenantDomain != "":
		return scheme + "://" + t.ID + "." + h.options.TenantDomain
	case len(t.Hosts) > 0:
		return scheme + "://" + t.Hosts[0]
	}
	return ""
}

// inviteAdmin makes an invitation to the first administrator of a tenant and returns its link.
func (h *handler) inviteAdmin(r *http.Request, t db.Tenant, login, name string) (string, error) {
	token, hash, err := auth.NewToken()
	if err != nil {
		return "", err
	}
	if err := h.options.DB.CreateInvite(tenant.With(r.Context(), t.ID), hash, db.Invite{Login: login, Name: name, Admin: true,
		MadeBy: "", Expires: time.Now().Add(inviteLife)}); err != nil {
		return "", err
	}
	return h.tenantAddress(r, t) + "/invite/" + token, nil
}

// createTenant makes a tenant and the invitation of its first administrator.
func (h *handler) createTenant(w http.ResponseWriter, r *http.Request) {
	if !h.hosterOnly(w, r) || !h.formOK(w, r) {
		return
	}
	ctx := r.Context()
	t, err := h.options.DB.CreateTenant(ctx, strings.TrimSpace(r.PostFormValue("id")), r.PostFormValue("name"))
	if err != nil {
		h.hosterPage(w, r, http.StatusBadRequest, hosterProblem(err), "", "", "")
		return
	}
	link, err := h.inviteAdmin(r, t, r.PostFormValue("admin_login"), r.PostFormValue("admin_name"))
	if err != nil {
		h.hosterPage(w, r, http.StatusBadRequest, "The tenant is there, the invitation is not: "+registerProblem(err), "", "", t.ID)
		return
	}
	h.log.Info("tenant made", slog.String("tenant", t.ID), slog.String("by", h.me(r).Login))
	h.hosterPage(w, r, http.StatusOK, "", "Send this link to the first administrator of "+t.Name+". It works once, for seven days.", link, t.ID)
}

func hosterProblem(err error) string {
	switch {
	case errors.Is(err, tenant.ErrID):
		return capital(err.Error()) + "."
	case errors.Is(err, db.ErrTenantTaken), errors.Is(err, db.ErrHostTaken), errors.Is(err, db.ErrHost), errors.Is(err, db.ErrReserved):
		return capital(err.Error()) + "."
	case errors.Is(err, db.ErrTitle):
		return "A name has 1 to 200 characters."
	}
	return "That did not work."
}

// changeTenant locks, opens, archives or restores a tenant, sets its limits and modules, adds or removes a domain, or
// invites another administrator. Archiving is the delete of the hoster: nothing goes, and restoring brings all back.
func (h *handler) changeTenant(w http.ResponseWriter, r *http.Request) {
	if !h.hosterOnly(w, r) || !h.formOK(w, r) {
		return
	}
	ctx := r.Context()
	id := r.PathValue("id")
	t, err := h.options.DB.Tenant(ctx, id)
	if err != nil || id == tenant.Hoster {
		h.notFound(w, r)
		return
	}
	notice, link := "Saved.", ""
	switch r.PostFormValue("do") {
	case "lock":
		_, err = h.options.DB.SetTenantState(ctx, id, db.TenantLocked)
	case "open":
		_, err = h.options.DB.SetTenantState(ctx, id, db.TenantActive)
	case "archive":
		_, err = h.options.DB.SetTenantState(ctx, id, db.TenantArchived)
		notice = "Archived: nobody gets in, and everything is kept. Restore brings it back."
	case "limits":
		err = h.saveLimits(r, id)
	case "add-host":
		err = h.options.DB.AddTenantHost(ctx, id, r.PostFormValue("host"))
	case "remove-host":
		err = h.options.DB.RemoveTenantHost(ctx, id, r.PostFormValue("host"))
	case "invite":
		link, err = h.inviteAdmin(r, t, r.PostFormValue("admin_login"), r.PostFormValue("admin_name"))
		notice = "Send this link to the new administrator. It works once, for seven days."
	default:
		h.notFound(w, r)
		return
	}
	if err != nil {
		problem := hosterProblem(err)
		if errors.Is(err, db.ErrLogin) || errors.Is(err, db.ErrLoginTaken) {
			problem = registerProblem(err)
		}
		if errors.Is(err, errLimit) {
			problem = capital(err.Error()) + "."
		}
		h.hosterPage(w, r, http.StatusBadRequest, problem, "", "", id)
		return
	}
	h.log.Info("tenant changed", slog.String("tenant", id), slog.String("do", r.PostFormValue("do")), slog.String("by", h.me(r).Login))
	h.hosterPage(w, r, http.StatusOK, "", notice, link, id)
}

var errLimit = errors.New("a limit is a whole number of at least 0, or empty for none")

func (h *handler) saveLimits(r *http.Request, id string) error {
	var l db.HosterLimits
	if v := strings.TrimSpace(r.PostFormValue("max_users")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return errLimit
		}
		l.MaxUsers = &n
	}
	if v := strings.TrimSpace(r.PostFormValue("max_storage_mb")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 || n > 1<<40 {
			return errLimit
		}
		bytes := n * 1_000_000
		l.MaxStorage = &bytes
	}
	if r.PostFormValue("all_modules") != "on" {
		l.Modules = []string{}
		for _, m := range module.All {
			if slices.Contains(r.PostForm["module"], m.ID) {
				l.Modules = append(l.Modules, m.ID)
			}
		}
	}
	return h.options.DB.SetTenantLimits(r.Context(), id, l)
}

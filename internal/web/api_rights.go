// SPDX-License-Identifier: MIT

package web

import (
	"errors"
	"log/slog"
	"net/http"
	"slices"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/module"
)

// The rights of ADR-29 through the API: roles on objects, inheritance, ownership and groups. Package db decides who may
// do what; these handlers only carry it.

type apiGrant struct {
	Object string `json:"object"`
	Kind   string `json:"kind"`
	Who    string `json:"who"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	At     string `json:"at,omitempty"`
}

type apiRights struct {
	Owner     string     `json:"owner"`
	OwnerName string     `json:"ownerName"`
	Inherit   bool       `json:"inherit"`
	Own       []apiGrant `json:"own"`
	Inherited []apiGrant `json:"inherited"`
	// Role is the viewer's role on the object, Manage whether the viewer may change its rights.
	Role   string `json:"role"`
	Manage bool   `json:"manage"`
}

func grantsOut(grants []db.Grant) []apiGrant {
	out := []apiGrant{}
	for _, g := range grants {
		a := apiGrant{Object: g.Object, Kind: g.Kind, Who: g.Who, Name: g.Name, Role: g.Role}
		if !g.At.IsZero() {
			a.At = stamp(g.At)
		}
		out = append(out, a)
	}
	return out
}

func (h *handler) actor(r *http.Request) db.Actor {
	user := h.me(r)
	return db.Actor{ID: user.ID, Admin: user.Admin}
}

// apiRights returns who holds which role on an object, for whoever sees it and for administrators.
func (h *handler) apiRights(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return
	}
	actor := h.actor(r)
	rights, err := h.options.DB.RightsOf(r.Context(), r.PathValue("id"), actor)
	if h.failed(w, err, "rights") {
		return
	}
	role, err := h.options.DB.Role(r.Context(), r.PathValue("id"), actor.ID)
	if h.failed(w, err, "rights") {
		return
	}
	writeJSON(w, http.StatusOK, apiRights{Owner: rights.Owner, OwnerName: rights.OwnerName, Inherit: rights.Inherit,
		Own: grantsOut(rights.Own), Inherited: grantsOut(rights.Inherited), Role: role,
		Manage: actor.Admin || db.Rank(role) >= db.Rank(db.RoleManage)})
}

// apiSetGrant grants a role on an object, or takes it back with an empty role.
func (h *handler) apiSetGrant(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind string `json:"kind"`
		Who  string `json:"who"`
		Role string `json:"role"`
	}
	if _, ok := h.change(w, r, 1<<10, &in); !ok {
		return
	}
	if h.failed(w, h.options.DB.SetGrant(r.Context(), r.PathValue("id"), h.actor(r), in.Kind, in.Who, in.Role), "grant") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// apiSetInherit breaks the inheritance of an object, or restores it.
func (h *handler) apiSetInherit(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Inherit bool `json:"inherit"`
	}
	if _, ok := h.change(w, r, 1<<10, &in); !ok {
		return
	}
	if h.failed(w, h.options.DB.SetInherit(r.Context(), r.PathValue("id"), h.actor(r), in.Inherit), "inherit") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// apiSetOwner transfers the ownership of an object.
func (h *handler) apiSetOwner(w http.ResponseWriter, r *http.Request) {
	var in struct {
		To string `json:"to"`
	}
	if _, ok := h.change(w, r, 1<<10, &in); !ok {
		return
	}
	if h.failed(w, h.options.DB.TransferOwner(r.Context(), r.PathValue("id"), h.actor(r), in.To), "owner") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type apiGroup struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Source      string `json:"source"`
	Description string `json:"description"`
	Members     int    `json:"members"`
	// Mine tells that the viewer belongs to it.
	Mine bool `json:"mine"`
}

type apiPerson struct {
	ID    string `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
}

// apiGroups lists the groups: administrators see all of them, everyone else the groups they belong to (ADR-29).
func (h *handler) apiGroups(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return
	}
	actor := h.actor(r)
	all, err := h.options.DB.Groups(r.Context())
	if h.failed(w, err, "groups") {
		return
	}
	mine, err := h.options.DB.GroupsOf(r.Context(), actor.ID)
	if h.failed(w, err, "groups") {
		return
	}
	out := []apiGroup{}
	for _, g := range all {
		member := false
		for _, m := range mine {
			member = member || m == g.ID
		}
		if !actor.Admin && !member {
			continue
		}
		out = append(out, apiGroup{ID: g.ID, Name: g.Name, Source: g.Source, Description: g.Description, Members: g.Members, Mine: member})
	}
	writeJSON(w, http.StatusOK, out)
}

// apiPeople lists the accounts and groups of the tenant to grant roles to: what a person picks from in the rights
// dialog. It names them only, nothing of their content.
func (h *handler) apiPeople(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return
	}
	accounts, err := h.options.DB.People(r.Context())
	if h.failed(w, err, "people") {
		return
	}
	groups, err := h.options.DB.Groups(r.Context())
	if h.failed(w, err, "people") {
		return
	}
	out := struct {
		People []apiPerson `json:"people"`
		Groups []apiGroup  `json:"groups"`
	}{People: []apiPerson{}, Groups: []apiGroup{}}
	for _, a := range accounts {
		out.People = append(out.People, apiPerson{ID: a.ID, Login: a.Login, Name: a.Name})
	}
	for _, g := range groups {
		out.Groups = append(out.Groups, apiGroup{ID: g.ID, Name: g.Name, Source: g.Source})
	}
	writeJSON(w, http.StatusOK, out)
}

// adminAPI answers 403 to whoever does not administer the tenant, and reports whether they do.
func (h *handler) adminAPI(w http.ResponseWriter, r *http.Request) bool {
	if !h.me(r).Admin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only administrators"})
		return false
	}
	return true
}

// apiCreateGroup makes a local group; only administrators do.
func (h *handler) apiCreateGroup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if _, ok := h.change(w, r, 4<<10, &in); !ok || !h.adminAPI(w, r) {
		return
	}
	g, err := h.options.DB.CreateGroup(r.Context(), in.Name, in.Description)
	if h.failed(w, err, "group") {
		return
	}
	writeJSON(w, http.StatusCreated, apiGroup{ID: g.ID, Name: g.Name, Source: g.Source, Description: g.Description})
}

// apiChangeGroup renames a local group; only administrators do.
func (h *handler) apiChangeGroup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if _, ok := h.change(w, r, 4<<10, &in); !ok || !h.adminAPI(w, r) {
		return
	}
	if h.failed(w, h.options.DB.RenameGroup(r.Context(), r.PathValue("id"), in.Name, in.Description), "group") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// apiDeleteGroup deletes a local group; only administrators do.
func (h *handler) apiDeleteGroup(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.change(w, r, 0, nil); !ok || !h.adminAPI(w, r) {
		return
	}
	if h.failed(w, h.options.DB.DeleteGroup(r.Context(), r.PathValue("id")), "group") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// apiGroupMembers lists the members of a group: for administrators, and for its members.
func (h *handler) apiGroupMembers(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return
	}
	actor := h.actor(r)
	if !actor.Admin {
		mine, err := h.options.DB.GroupsOf(r.Context(), actor.ID)
		if h.failed(w, err, "members") {
			return
		}
		member := false
		for _, m := range mine {
			member = member || m == r.PathValue("id")
		}
		if !member {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
	}
	members, err := h.options.DB.GroupMembers(r.Context(), r.PathValue("id"))
	if h.failed(w, err, "members") {
		return
	}
	out := []apiPerson{}
	for _, m := range members {
		out = append(out, apiPerson{ID: m.ID, Login: m.Login, Name: m.Name})
	}
	writeJSON(w, http.StatusOK, out)
}

// apiSetMember adds an account to a local group or takes it out; only administrators do.
func (h *handler) apiSetMember(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Member bool `json:"member"`
	}
	if _, ok := h.change(w, r, 1<<10, &in); !ok || !h.adminAPI(w, r) {
		return
	}
	if h.failed(w, h.options.DB.SetMember(r.Context(), r.PathValue("id"), r.PathValue("account"), in.Member), "member") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// roleWords are the roles as the interface names them.
var roleWords = map[string]string{db.RoleRead: "read", db.RoleComment: "comment", db.RoleEdit: "edit", db.RoleManage: "manage", db.RoleOwner: "owner"}

// seenBy names who sees an object and with which role, the highest of each: the owner, the owners above, and every
// grant on it and above it while it inherits (ADR-29). Private objects are seen by their owner only.
func (h *handler) seenBy(r *http.Request, o db.Object) []string {
	if o.Private {
		return []string{"only you"}
	}
	rights, err := h.options.DB.RightsOf(r.Context(), o.ID, h.actor(r))
	if err != nil {
		h.log.Error("rights failed", slog.String("error", err.Error()))
		return nil
	}
	if rights.Private {
		return []string{"only " + rights.OwnerName}
	}
	type who struct{ kind, id, name string }
	best := map[who]string{{db.ToPerson, rights.Owner, rights.OwnerName}: db.RoleOwner}
	var order []who
	order = append(order, who{db.ToPerson, rights.Owner, rights.OwnerName})
	for _, g := range append(slices.Clone(rights.Own), rights.Inherited...) {
		k := who{g.Kind, g.Who, g.Name}
		if _, seen := best[k]; !seen {
			order = append(order, k)
		}
		if db.Rank(g.Role) > db.Rank(best[k]) {
			best[k] = g.Role
		}
	}
	slices.SortStableFunc(order, func(a, b who) int {
		rank := func(w who) int { return map[string]int{db.ToTenant: 0, db.ToGroup: 1, db.ToPerson: 2}[w.kind] }
		return rank(a) - rank(b)
	})
	out := []string{}
	for _, k := range order {
		name := k.name
		switch k.kind {
		case db.ToTenant:
			name = "everyone"
		case db.ToGroup:
			name = "group " + k.name
		}
		out = append(out, name+" ("+roleWords[best[k]]+")")
	}
	return out
}

type apiReason struct {
	Object string `json:"object"`
	Title  string `json:"title"`
	Via    string `json:"via"`
	Name   string `json:"name,omitempty"`
	Role   string `json:"role"`
}

// apiCheck answers the access check "what may X do with Y": the role of an account on an object, and why.
func (h *handler) apiCheck(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return
	}
	actor := h.actor(r)
	accountID := r.URL.Query().Get("account")
	if accountID == "" {
		accountID = actor.ID
	}
	role, reasons, err := h.options.DB.Explain(r.Context(), r.URL.Query().Get("object"), actor, accountID)
	if h.failed(w, err, "check") {
		return
	}
	out := struct {
		Role    string      `json:"role"`
		Reasons []apiReason `json:"reasons"`
	}{Role: role, Reasons: []apiReason{}}
	for _, x := range reasons {
		out.Reasons = append(out.Reasons, apiReason{Object: x.Object, Title: x.Title, Via: x.Via, Name: x.Name, Role: x.Role})
	}
	writeJSON(w, http.StatusOK, out)
}

type apiHeld struct {
	Object string `json:"object"`
	Title  string `json:"title"`
	Kind   string `json:"kind"`
	Area   string `json:"area"`
	Via    string `json:"via"`
	Name   string `json:"name,omitempty"`
	Role   string `json:"role"`
}

// apiHeld answers "what may X do": where an account holds a role of its own, through a group or as the tenant.
func (h *handler) apiHeld(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return
	}
	actor := h.actor(r)
	accountID := r.URL.Query().Get("account")
	if accountID == "" {
		accountID = actor.ID
	}
	held, err := h.options.DB.HeldBy(r.Context(), actor, accountID)
	if h.failed(w, err, "held") {
		return
	}
	out := []apiHeld{}
	for _, x := range held {
		out = append(out, apiHeld{Object: x.Object, Title: x.Title, Kind: x.Kind, Area: x.Area, Via: x.Via, Name: x.Name, Role: x.Role})
	}
	writeJSON(w, http.StatusOK, out)
}

type apiAccount struct {
	ID     string   `json:"id"`
	Login  string   `json:"login"`
	Name   string   `json:"name"`
	Source string   `json:"source"`
	Admin  bool     `json:"admin"`
	Groups []string `json:"groups"`
}

// apiUsers lists the accounts of the tenant for the administration of users; only administrators see it.
func (h *handler) apiUsers(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return
	}
	if !h.adminAPI(w, r) {
		return
	}
	everyone, err := h.options.DB.Everyone(r.Context())
	if h.failed(w, err, "users") {
		return
	}
	out := []apiAccount{}
	for _, u := range everyone {
		out = append(out, apiAccount{ID: u.ID, Login: u.Login, Name: u.Name, Source: u.Source, Admin: u.Admin, Groups: u.Groups})
	}
	writeJSON(w, http.StatusOK, out)
}

// apiSetAdmin makes a local account an administrator or takes that back. Whoever stops administering the tenant stops
// administering its organization in the time tracking at once, where UnderDoc made them do so (ADR-29).
func (h *handler) apiSetAdmin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Admin bool `json:"admin"`
	}
	if _, ok := h.change(w, r, 1<<10, &in); !ok || !h.adminAPI(w, r) {
		return
	}
	accountID := r.PathValue("id")
	err := h.options.DB.SetAdmin(r.Context(), h.actor(r), accountID, in.Admin)
	switch {
	case errors.Is(err, db.ErrSource):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "this account is an administrator through its group at the sign-in"})
		return
	case errors.Is(err, db.ErrLastAdmin):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	case h.failed(w, err, "admin"):
		return
	}
	if !in.Admin && h.moduleOn(r.Context(), module.Time) {
		if err := h.options.DB.MapTenant(r.Context(), accountID, false); err != nil {
			h.log.Error("time tracking: administrator not taken back", slog.String("error", err.Error()))
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

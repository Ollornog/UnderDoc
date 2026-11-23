// SPDX-License-Identifier: MIT

package web

import (
	"cmp"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/module"
)

// The rights of ADR-29 and ADR-36 through the API: entries per action on objects, inheritance, ownership and groups.
// Package db decides who may do what; these handlers only carry it.

type apiGrant struct {
	Object string `json:"object"`
	Kind   string `json:"kind"`
	Who    string `json:"who"`
	Name   string `json:"name"`
	// Role is the preset the entries make (read, comment, edit, manage), "custom", or empty.
	Role string `json:"role"`
	// Actions are the entries: each action with one maps to allow or deny; the others inherit.
	Actions   map[string]string `json:"actions"`
	Subgroups bool              `json:"subgroups"`
	At        string            `json:"at,omitempty"`
}

type apiRights struct {
	Owner     string     `json:"owner"`
	OwnerName string     `json:"ownerName"`
	Inherit   bool       `json:"inherit"`
	Own       []apiGrant `json:"own"`
	Inherited []apiGrant `json:"inherited"`
	// Agents are the denies on Agents above the object: a ceiling for every agent, whatever breaks the inheritance.
	Agents []apiGrant `json:"agents"`
	// Role is the viewer's role on the object, Actions what the viewer may do, Manage whether the viewer may change its
	// rights.
	Role    string   `json:"role"`
	Actions []string `json:"actions"`
	Manage  bool     `json:"manage"`
	// Samba says what the share makes of the object (ADR-30); nil where the tenant has no companion.
	Samba *apiSambaRights `json:"samba,omitempty"`
}

// apiSambaRights is what the rights dialog shows of Samba: whether the object lies in a shared library, that a file
// there has rights per folder only on the share, where the viewer finds it, and the switch of a collection's share.
type apiSambaRights struct {
	Shared bool   `json:"shared"`
	File   bool   `json:"file"`
	Path   string `json:"path,omitempty"`
	// Root is set for a collection of the view Files: whether its share is on.
	Root *bool `json:"root,omitempty"`
}

func grantsOut(grants []db.Grant) []apiGrant {
	out := []apiGrant{}
	for _, g := range grants {
		a := apiGrant{Object: g.Object, Kind: g.Kind, Who: g.Who, Name: g.Name, Role: g.Role, Actions: g.Actions, Subgroups: g.Subgroups}
		if a.Actions == nil {
			a.Actions = map[string]string{}
		}
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
	held, err := h.options.DB.ActionsOf(r.Context(), r.PathValue("id"), actor.ID)
	if h.failed(w, err, "rights") {
		return
	}
	user := h.me(r)
	out := apiRights{Owner: rights.Owner, OwnerName: rights.OwnerName, Inherit: rights.Inherit,
		Own: grantsOut(rights.Own), Inherited: grantsOut(rights.Inherited), Agents: grantsOut(rights.Agents), Role: role, Actions: held,
		Manage: !user.Agent() && (actor.Admin || slices.Contains(held, db.ActManage))}
	if h.options.Share != nil && h.options.Share.On(r.Context()) {
		out.Samba = h.sambaRights(r, r.PathValue("id"), actor.ID)
	}
	writeJSON(w, http.StatusOK, out)
}

// apiSetGrant sets what one holder is granted on an object: a preset ("role", empty takes every entry back), or the
// entries per action ("actions", each allow or deny, the others inherit) with "subgroups" for a group (ADR-36).
func (h *handler) apiSetGrant(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind      string            `json:"kind"`
		Who       string            `json:"who"`
		Role      string            `json:"role"`
		Actions   map[string]string `json:"actions"`
		Subgroups *bool             `json:"subgroups"`
	}
	if _, ok := h.change(w, r, 2<<10, &in); !ok {
		return
	}
	var err error
	detail := strings.TrimSpace(in.Kind + " " + in.Who)
	if in.Actions != nil {
		subgroups := in.Subgroups == nil || *in.Subgroups
		err = h.options.DB.SetEntries(r.Context(), r.PathValue("id"), h.actor(r), in.Kind, in.Who, in.Actions, subgroups)
		detail += " " + describeEntries(in.Actions, in.Kind == db.ToGroup && !subgroups)
	} else {
		err = h.options.DB.SetGrant(r.Context(), r.PathValue("id"), h.actor(r), in.Kind, in.Who, in.Role)
		detail += " " + cmp.Or(in.Role, "taken back")
	}
	if h.failed(w, err, "grant") {
		return
	}
	h.note(r, db.LogGrant, r.PathValue("id"), detail)
	w.WriteHeader(http.StatusNoContent)
}

// describeEntries says entries in words for the log: "read allow, write deny", or "taken back" for none.
func describeEntries(actions map[string]string, direct bool) string {
	var parts []string
	for _, a := range db.Actions {
		if v := actions[a]; v == db.Allow || v == db.Deny {
			parts = append(parts, a+" "+v)
		}
	}
	if len(parts) == 0 {
		return "taken back"
	}
	out := strings.Join(parts, ", ")
	if direct {
		out += " (without subgroups)"
	}
	return out
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
	h.note(r, db.LogInherit, r.PathValue("id"), map[bool]string{true: "restored", false: "broken"}[in.Inherit])
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
	h.note(r, db.LogOwner, r.PathValue("id"), "to "+in.To)
	w.WriteHeader(http.StatusNoContent)
}

type apiGroup struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Source      string `json:"source"`
	Description string `json:"description"`
	Members     int    `json:"members"`
	// Groups counts the groups directly inside it.
	Groups int `json:"groups"`
	// Mine tells that the viewer belongs to it, Admin that the viewer manages it (as an administrator of the tenant or an
	// admin of it or of a group above it).
	Mine  bool `json:"mine"`
	Admin bool `json:"admin"`
	// Parents are the IDs of the groups it is directly inside.
	Parents []string `json:"parents"`
}

type apiPerson struct {
	ID    string `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
}

// groupsSeen returns every group with whether the viewer belongs to it and whether they manage it.
func (h *handler) groupsSeen(r *http.Request) ([]apiGroup, error) {
	actor := h.actor(r)
	ctx := r.Context()
	all, err := h.options.DB.Groups(ctx)
	if err != nil {
		return nil, err
	}
	mine, err := h.options.DB.GroupsOf(ctx, actor.ID)
	if err != nil {
		return nil, err
	}
	administered, err := h.options.DB.AdministeredGroups(ctx, actor.ID)
	if err != nil {
		return nil, err
	}
	agent := h.me(r).Agent()
	out := []apiGroup{}
	for _, g := range all {
		parents, err := h.options.DB.ParentGroups(ctx, g.ID)
		if err != nil {
			return nil, err
		}
		ids := []string{}
		for _, p := range parents {
			ids = append(ids, p.ID)
		}
		out = append(out, apiGroup{ID: g.ID, Name: g.Name, Source: g.Source, Description: g.Description, Members: g.Members, Groups: g.Groups,
			Mine: slices.Contains(mine, g.ID), Admin: !agent && (actor.Admin || slices.Contains(administered, g.ID)), Parents: ids})
	}
	return out, nil
}

// apiGroups lists the groups: administrators see all of them, everyone else the groups they belong to and those they
// manage as a group admin (ADR-29, ADR-36).
func (h *handler) apiGroups(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return
	}
	seen, err := h.groupsSeen(r)
	if h.failed(w, err, "groups") {
		return
	}
	actor := h.actor(r)
	out := []apiGroup{}
	for _, g := range seen {
		if actor.Admin || g.Mine || g.Admin {
			out = append(out, g)
		}
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
		out.Groups = append(out.Groups, apiGroup{ID: g.ID, Name: g.Name, Source: g.Source, Groups: g.Groups, Parents: []string{}})
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

// apiCreateGroup makes a local group: administrators anywhere, the admin of a group inside it (parent).
func (h *handler) apiCreateGroup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Parent      string `json:"parent"`
	}
	if _, ok := h.change(w, r, 4<<10, &in); !ok {
		return
	}
	g, err := h.options.DB.CreateGroup(r.Context(), h.actor(r), in.Name, in.Description, in.Parent)
	if h.failed(w, err, "group") {
		return
	}
	detail := "made " + g.ID + " " + g.Name
	if in.Parent != "" {
		detail += " inside " + in.Parent
	}
	h.note(r, db.LogGroup, "", detail)
	writeJSON(w, http.StatusCreated, apiGroup{ID: g.ID, Name: g.Name, Source: g.Source, Description: g.Description, Admin: true, Parents: nonNilStrings([]string{in.Parent})})
}

func nonNilStrings(list []string) []string {
	out := []string{}
	for _, s := range list {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// apiChangeGroup renames a local group: administrators and its group admins.
func (h *handler) apiChangeGroup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if _, ok := h.change(w, r, 4<<10, &in); !ok {
		return
	}
	if h.failed(w, h.options.DB.RenameGroup(r.Context(), h.actor(r), r.PathValue("id"), in.Name, in.Description), "group") {
		return
	}
	h.note(r, db.LogGroup, "", "renamed "+r.PathValue("id")+" to "+in.Name)
	w.WriteHeader(http.StatusNoContent)
}

// apiDeleteGroup deletes a local group: administrators and its group admins.
func (h *handler) apiDeleteGroup(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.change(w, r, 0, nil); !ok {
		return
	}
	if h.failed(w, h.options.DB.DeleteGroup(r.Context(), h.actor(r), r.PathValue("id")), "group") {
		return
	}
	h.note(r, db.LogGroup, "", "deleted "+r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

// mayReadGroup tells whether the viewer sees a group's members: administrators, its members and whoever manages it.
func (h *handler) mayReadGroup(r *http.Request, groupID string) (bool, error) {
	actor := h.actor(r)
	if actor.Admin {
		return true, nil
	}
	mine, err := h.options.DB.GroupsOf(r.Context(), actor.ID)
	if err != nil || slices.Contains(mine, groupID) {
		return err == nil, err
	}
	administered, err := h.options.DB.AdministeredGroups(r.Context(), actor.ID)
	return slices.Contains(administered, groupID), err
}

// apiGroupMembers lists the persons directly in a group: for administrators, its members and its group admins.
func (h *handler) apiGroupMembers(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return
	}
	ok, err := h.mayReadGroup(r, r.PathValue("id"))
	if h.failed(w, err, "members") {
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	members, err := h.options.DB.GroupMembers(r.Context(), r.PathValue("id"))
	if h.failed(w, err, "members") {
		return
	}
	writeJSON(w, http.StatusOK, peopleOut(members))
}

func peopleOut(list []db.Member) []apiPerson {
	out := []apiPerson{}
	for _, m := range list {
		out = append(out, apiPerson{ID: m.ID, Login: m.Login, Name: m.Name})
	}
	return out
}

// apiGroup answers one group with its persons, the groups inside it, the groups it is inside and its admins.
func (h *handler) apiGroup(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return
	}
	groupID := r.PathValue("id")
	ok, err := h.mayReadGroup(r, groupID)
	if h.failed(w, err, "group") {
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	ctx := r.Context()
	g, err := h.options.DB.GroupOf(ctx, groupID)
	if h.failed(w, err, "group") {
		return
	}
	members, err := h.options.DB.GroupMembers(ctx, groupID)
	if h.failed(w, err, "group") {
		return
	}
	inner, err := h.options.DB.SubGroups(ctx, groupID)
	if h.failed(w, err, "group") {
		return
	}
	outer, err := h.options.DB.ParentGroups(ctx, groupID)
	if h.failed(w, err, "group") {
		return
	}
	admins, err := h.options.DB.GroupAdmins(ctx, groupID)
	if h.failed(w, err, "group") {
		return
	}
	manages, err := h.options.DB.MayAdministerGroup(ctx, h.actor(r), groupID)
	if h.failed(w, err, "group") {
		return
	}
	groupsOut := func(list []db.Group) []apiGroup {
		out := []apiGroup{}
		for _, x := range list {
			out = append(out, apiGroup{ID: x.ID, Name: x.Name, Source: x.Source, Members: x.Members, Groups: x.Groups, Parents: []string{}})
		}
		return out
	}
	writeJSON(w, http.StatusOK, struct {
		Group   apiGroup    `json:"group"`
		Members []apiPerson `json:"members"`
		Groups  []apiGroup  `json:"groups"`
		Parents []apiGroup  `json:"parents"`
		Admins  []apiPerson `json:"admins"`
	}{
		Group:   apiGroup{ID: g.ID, Name: g.Name, Source: g.Source, Description: g.Description, Members: g.Members, Groups: g.Groups, Admin: manages, Parents: []string{}},
		Members: peopleOut(members), Groups: groupsOut(inner), Parents: groupsOut(outer), Admins: peopleOut(admins),
	})
}

// apiSetMember adds an account to a local group or takes it out: administrators and its group admins.
func (h *handler) apiSetMember(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Member bool `json:"member"`
	}
	if _, ok := h.change(w, r, 1<<10, &in); !ok {
		return
	}
	if h.failed(w, h.options.DB.SetMember(r.Context(), h.actor(r), r.PathValue("id"), r.PathValue("account"), in.Member), "member") {
		return
	}
	h.note(r, db.LogMember, "", r.PathValue("account")+map[bool]string{true: " into ", false: " out of "}[in.Member]+r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

// apiSetSubgroup puts a group inside a local group or takes it out: administrators and its group admins. A cycle is
// refused (409).
func (h *handler) apiSetSubgroup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Member bool `json:"member"`
	}
	if _, ok := h.change(w, r, 1<<10, &in); !ok {
		return
	}
	err := h.options.DB.SetSubgroup(r.Context(), h.actor(r), r.PathValue("id"), r.PathValue("group"), in.Member)
	if errors.Is(err, db.ErrGroupCycle) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	if h.failed(w, err, "subgroup") {
		return
	}
	h.note(r, db.LogMember, "", "group "+r.PathValue("group")+map[bool]string{true: " into ", false: " out of "}[in.Member]+r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

// apiSetGroupAdmin makes an account an admin of a local group or takes that back: administrators and its group admins.
func (h *handler) apiSetGroupAdmin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Admin bool `json:"admin"`
	}
	if _, ok := h.change(w, r, 1<<10, &in); !ok {
		return
	}
	if h.failed(w, h.options.DB.SetGroupAdmin(r.Context(), h.actor(r), r.PathValue("id"), r.PathValue("account"), in.Admin), "group admin") {
		return
	}
	h.note(r, db.LogGroup, "", r.PathValue("account")+map[bool]string{true: " made an admin of ", false: " no longer an admin of "}[in.Admin]+r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

// apiAgentLimit answers what the viewer lets their own agents do, and what agents may do at most.
func (h *handler) apiAgentLimit(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return
	}
	actions, err := h.options.DB.AgentLimit(r.Context(), h.me(r).ID)
	if h.failed(w, err, "agents") {
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"actions": actions, "possible": db.AgentActions})
}

// apiSetAgentLimit sets what the viewer lets their own agents do. An agent cannot change it.
func (h *handler) apiSetAgentLimit(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Actions []string `json:"actions"`
	}
	if _, ok := h.change(w, r, 1<<10, &in); !ok {
		return
	}
	if h.failed(w, h.options.DB.SetAgentLimit(r.Context(), h.me(r).ID, in.Actions), "agents") {
		return
	}
	h.note(r, db.LogAgents, "", "own agents may: "+strings.Join(in.Actions, ", "))
	w.WriteHeader(http.StatusNoContent)
}

// roleWords are the roles as the interface names them.
var roleWords = map[string]string{db.RoleRead: "read", db.RoleComment: "comment", db.RoleEdit: "edit", db.RoleManage: "manage", db.RoleOwner: "owner",
	db.PresetCustom: "custom"}

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
	denied := map[who]bool{}
	for _, g := range append(slices.Clone(rights.Own), rights.Inherited...) {
		if g.Kind == db.ToAgents {
			continue
		}
		k := who{g.Kind, g.Who, g.Name}
		if g.Actions[db.ActRead] == db.Deny {
			denied[k] = true
		}
		if _, seen := best[k]; !seen {
			order = append(order, k)
			best[k] = g.Role
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
		word := roleWords[best[k]]
		if denied[k] {
			word = "read denied somewhere"
		}
		out = append(out, name+" ("+word+")")
	}
	return out
}

type apiDecision struct {
	Action  string `json:"action"`
	Allowed bool   `json:"allowed"`
	// Via is owner, owner above, entry, link, private, none, agents, agents default, agents own; Object and Title
	// where the deciding entry is; Kind, Who and Name its holder; Value allow or deny.
	Via    string `json:"via"`
	Object string `json:"object,omitempty"`
	Title  string `json:"title,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Who    string `json:"who,omitempty"`
	Name   string `json:"name,omitempty"`
	Value  string `json:"value,omitempty"`
}

// apiCheck answers the access check "what may X do with Y": for each action whether it is allowed, and the deciding
// entry with where it comes from (ADR-36). ?agent=1 asks it for an agent of that account.
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
	agent := r.URL.Query().Get("agent") == "1"
	role, decisions, err := h.options.DB.Explain(r.Context(), r.URL.Query().Get("object"), actor, accountID, agent)
	if h.failed(w, err, "check") {
		return
	}
	out := struct {
		Role    string        `json:"role"`
		Agent   bool          `json:"agent"`
		Actions []apiDecision `json:"actions"`
	}{Role: role, Agent: agent, Actions: []apiDecision{}}
	for _, d := range decisions {
		out.Actions = append(out.Actions, apiDecision{Action: d.Action, Allowed: d.Allowed, Via: d.Via, Object: d.Object, Title: d.Title,
			Kind: d.Kind, Who: d.Who, Name: d.Name, Value: d.Value})
	}
	writeJSON(w, http.StatusOK, out)
}

type apiHeld struct {
	Object  string            `json:"object"`
	Title   string            `json:"title"`
	Kind    string            `json:"kind"`
	Area    string            `json:"area"`
	Via     string            `json:"via"`
	Name    string            `json:"name,omitempty"`
	Role    string            `json:"role"`
	Actions map[string]string `json:"actions"`
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
		out = append(out, apiHeld{Object: x.Object, Title: x.Title, Kind: x.Kind, Area: x.Area, Via: x.Via, Name: x.Name, Role: x.Role, Actions: x.Actions})
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
	h.note(r, db.LogAdmin, "", accountID+map[bool]string{true: " made an administrator", false: " no longer an administrator"}[in.Admin])
	if !in.Admin && h.moduleOn(r.Context(), module.Time) {
		if err := h.options.DB.MapTenant(r.Context(), accountID, false); err != nil {
			h.log.Error("time tracking: administrator not taken back", slog.String("error", err.Error()))
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

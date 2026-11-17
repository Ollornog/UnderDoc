// SPDX-License-Identifier: MIT

// Package module knows the modules of UnderDoc and which of them are on (ADR-25). Every module can be switched: the
// hoster sets what is allowed and on by default (UNDERDOC_MODULES) and what each tenant may use, the administrator of a
// tenant switches among that, and each user can hide what is on. Switching off never deletes data: a module that is off
// leaves no entry, no route and no job behind, and is whole again when it comes back.
package module

import (
	"fmt"
	"slices"
	"strings"
)

// Module is one module.
type Module struct {
	ID          string
	Name        string
	Description string
	// On tells whether the module is on for a tenant that has not switched it, where the hoster allows it.
	On bool
}

// The modules there are.
const (
	// Documents are collections, folders and pages with everything that belongs to them.
	Documents = "documents"
	// Files is the view Files: its roots, files with versions, and uploads (ADR-28).
	Files = "files"
)

// All lists every module there is, in the order the settings show them. A new module adds itself here, guards its
// routes and jobs with the state of its tenant (web: h.module), and gets tests with itself on and off.
var All = []Module{
	{ID: Documents, Name: "Documents", Description: "Collections, folders and pages, with comments, embeddings, tags, the trash and the folder source.", On: true},
	{ID: Files, Name: "Files", Description: "The view Files: the folder of the workspace, a folder of each user and collections, with versions and uploads.", On: true},
}

// Find returns a module by its ID.
func Find(id string) (Module, bool) {
	i := slices.IndexFunc(All, func(m Module) bool { return m.ID == id })
	if i < 0 {
		return Module{}, false
	}
	return All[i], true
}

// Policy is what the hoster set for the instance: which modules are allowed, and which of them are on by default.
type Policy struct {
	allowed map[string]bool
	on      map[string]bool
}

// ParsePolicy reads UNDERDOC_MODULES: module IDs separated by commas, each allowed and on by default, or with ":off"
// allowed but off by default. A module not named is not allowed. Empty allows every module, with its own default.
func ParsePolicy(value string) (Policy, error) {
	p := Policy{allowed: map[string]bool{}, on: map[string]bool{}}
	if strings.TrimSpace(value) == "" {
		for _, m := range All {
			p.allowed[m.ID], p.on[m.ID] = true, m.On
		}
		return p, nil
	}
	for _, part := range strings.Split(value, ",") {
		id, state, _ := strings.Cut(strings.TrimSpace(part), ":")
		if id == "" {
			continue
		}
		if _, ok := Find(id); !ok {
			return Policy{}, fmt.Errorf("UNDERDOC_MODULES: no module %q (there are: %s)", id, strings.Join(IDs(), ", "))
		}
		switch state {
		case "", "on":
			p.on[id] = true
		case "off":
			p.on[id] = false
		default:
			return Policy{}, fmt.Errorf("UNDERDOC_MODULES: %q is neither on nor off", part)
		}
		p.allowed[id] = true
	}
	return p, nil
}

// IDs returns the IDs of all modules.
func IDs() []string {
	ids := make([]string, len(All))
	for i, m := range All {
		ids[i] = m.ID
	}
	return ids
}

// State is a module as one user of one tenant has it.
type State struct {
	Module
	// Allowed: the hoster lets the tenant use it. On: the tenant has it switched on. Hidden: the user hides it.
	Allowed, On, Hidden bool
}

// Tenant is what a tenant and its user set: allowed is the choice of the hoster for the tenant, nil for everything the
// instance allows; on is the choice of the administrator, nil for the defaults; hidden is the choice of the user.
type Tenant struct {
	Allowed []string
	On      []string
	Hidden  []string
}

// States returns every module with its state for a tenant and a user.
func (p Policy) States(t Tenant) []State {
	out := make([]State, 0, len(All))
	for _, m := range All {
		s := State{Module: m}
		s.Allowed = p.allowed[m.ID] && (t.Allowed == nil || slices.Contains(t.Allowed, m.ID))
		if t.On == nil {
			s.On = s.Allowed && p.on[m.ID]
		} else {
			s.On = s.Allowed && slices.Contains(t.On, m.ID)
		}
		s.Hidden = s.On && slices.Contains(t.Hidden, m.ID)
		out = append(out, s)
	}
	return out
}

// On tells whether a module is on for a tenant.
func (p Policy) On(t Tenant, id string) bool {
	for _, s := range p.States(t) {
		if s.ID == id {
			return s.On
		}
	}
	return false
}

// IsZero tells a policy that was never read, which allows nothing.
func (p Policy) IsZero() bool { return p.allowed == nil }

// Allows tells whether the instance allows a module at all.
func (p Policy) Allows(id string) bool { return p.allowed[id] }

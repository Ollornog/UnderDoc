// SPDX-License-Identifier: MIT

package module

import (
	"testing"
)

func TestPolicy(t *testing.T) {
	all, err := ParsePolicy("")
	if err != nil || !all.Allows(Documents) || !all.On(Tenant{}, Documents) {
		t.Fatalf("empty allows everything with its defaults: %v", err)
	}
	off, err := ParsePolicy(" documents:off ")
	if err != nil || !off.Allows(Documents) || off.On(Tenant{}, Documents) {
		t.Errorf("allowed, off by default: %v", err)
	}
	if !off.On(Tenant{On: []string{Documents}}, Documents) {
		t.Error("a tenant switches on what is allowed")
	}
	none, err := ParsePolicy(",")
	if err != nil || none.Allows(Documents) || none.On(Tenant{On: []string{Documents}}, Documents) {
		t.Errorf("not named, not allowed, not even when a tenant switches it on: %v", err)
	}
	for _, bad := range []string{"mail", "documents:maybe"} {
		if _, err := ParsePolicy(bad); err == nil {
			t.Errorf("%q", bad)
		}
	}
}

func TestStates(t *testing.T) {
	p, _ := ParsePolicy("documents")
	for _, c := range []struct {
		name                string
		tenant              Tenant
		allowed, on, hidden bool
	}{
		{"defaults", Tenant{}, true, true, false},
		{"switched off", Tenant{On: []string{}}, true, false, false},
		{"not allowed for the tenant", Tenant{Allowed: []string{}, On: []string{Documents}}, false, false, false},
		{"hidden by the user", Tenant{Hidden: []string{Documents}}, true, true, true},
		{"hidden but off", Tenant{On: []string{}, Hidden: []string{Documents}}, true, false, false},
	} {
		s := p.States(c.tenant)[0]
		if s.ID != Documents || s.Allowed != c.allowed || s.On != c.on || s.Hidden != c.hidden {
			t.Errorf("%s: %+v", c.name, s)
		}
	}
	if _, ok := Find("nothing"); ok {
		t.Error("an unknown module")
	}
	if m, ok := Find(Documents); !ok || m.Name != "Documents" {
		t.Errorf("%+v", m)
	}
}

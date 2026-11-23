// SPDX-License-Identifier: MIT

package pagekind

import (
	"slices"
	"strings"
	"testing"
)

func codes(fs []Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Code)
	}
	return out
}

func TestKindsAndAudiences(t *testing.T) {
	for kind, want := range map[string]string{
		"": AudiencePeople, Overview: AudienceStructure, Register: AudienceStructure, Record: AudienceStructure,
		Plan: AudienceStructure, Source: AudienceStructure, Reference: AudienceAgents, AgentNotes: AudienceAgents,
		HowTo: AudiencePeople, Explanation: AudiencePeople, Tutorial: AudiencePeople,
	} {
		if got := Audience(kind); got != want {
			t.Errorf("Audience(%q) = %q, want %q", kind, got, want)
		}
		if !Valid(kind) {
			t.Errorf("%q is valid", kind)
		}
	}
	for _, bad := range []string{"notes", "page", "How-To", "secret"} {
		if Valid(bad) {
			t.Errorf("%q is not a kind", bad)
		}
	}
	for in, want := range map[string]string{"How-to": HowTo, "howto": HowTo, "Agent notes": AgentNotes, "agent_notes": AgentNotes,
		" REFERENCE ": Reference, "Anleitung": HowTo, "erklärung": Explanation, "Erklaerung": Explanation, "Referenz": Reference,
		"Datensatz": Record, "Quelle": Source, "": ""} {
		if got, ok := Parse(in); !ok || got != want {
			t.Errorf("Parse(%q) = %q %v, want %q", in, got, ok, want)
		}
	}
	if _, ok := Parse("notes"); ok {
		t.Error("notes (a board of quick notes) is no kind of page")
	}
	if Name(HowTo) != "How-to" || Name(AgentNotes) != "Agent notes" || Name(Reference) != "Reference" || Name("") != "No kind" {
		t.Error("names of kinds")
	}
}

// TestTeamWiki: the two axes of the team wiki come to one kind (T-28).
func TestTeamWiki(t *testing.T) {
	for _, c := range []struct{ typ, art, want string }{
		{"Register", "", Register}, {"Register", "anleitung", Register}, {"Plan", "", Plan}, {"Quelle", "referenz", Source},
		{"Regeln", "anleitung", HowTo}, {"Thema", "referenz", Reference}, {"Thema", "erklärung", Explanation},
		{"Thema", "tutorial", Tutorial}, {"Regeln", "", AgentNotes}, {"Thema", "", ""}, {"", "", ""}, {"Thema", "unbekannt", ""},
	} {
		if got := FromTeamWiki(c.typ, c.art); got != c.want {
			t.Errorf("FromTeamWiki(%q, %q) = %q, want %q", c.typ, c.art, got, c.want)
		}
	}
}

// TestRefuse: front matter in a block and a part ID used twice block a write; anything else does not.
func TestRefuse(t *testing.T) {
	for body, want := range map[string][]string{
		"---\nkind: how-to\ndescription: x\n---\nText.":                         {CodeFrontMatter},
		"---\ntitle: A\ntags:\n  - a\n---\n":                                    {CodeFrontMatter},
		"---\nNote: this is a rule\n---\n":                                      nil, // a rule, a line, a rule: no keys of front matter
		"Text\n\n---\n\nkind: of a line\n\n---":                                 nil,
		"```yaml\n---\nkind: how-to\n---\n```":                                  nil,
		"<!-- part t1 -->a<!-- /part t1 --> <!-- part t1 -->b":                  {CodeID},
		"<!-- part t1 -->a<!-- /part t1 --> <!-- part t2 -->b<!-- /part t2 -->": nil,
	} {
		got := codes(Refuse("b1", body))
		if !slices.Equal(got, want) {
			t.Errorf("Refuse(%q) = %v, want %v", body, got, want)
		}
		for _, f := range Refuse("b1", body) {
			if f.Level != Error || f.Block != "b1" || f.Fix == "" {
				t.Errorf("an error with its block and a fix: %+v", f)
			}
		}
	}
}

func page(kind string, bodies ...string) Page {
	p := Page{Kind: kind}
	for i, b := range bodies {
		p.Blocks = append(p.Blocks, Block{ID: "b" + string(rune('1'+i)), Body: b})
	}
	return p
}

// TestCheckRules: each rule of ADR-33 finds what breaks it, only for the kinds it is meant for, and stays quiet otherwise.
func TestCheckRules(t *testing.T) {
	values := []Value{{Text: "192.0.2.12", Address: "ref0000001#b1", Title: "Network"}, {Text: "/etc/app/config.yaml", Address: "ref0000001#b2", Title: "Network"},
		{Text: "backup.example.com", Address: "ref0000001#b3", Title: "Network"}}
	cases := []struct {
		name string
		p    Page
		want []string
	}{
		{"empty page says nothing", page(HowTo), nil},
		{"no kind", page("", "Some text for people here."), []string{CodeKindMissing}},
		{"no kind, no START", page("", "- a list first"), []string{CodeKindMissing}},
		{"how-to starts with a list", page(HowTo, "- first\n- second", "1. Do it."), []string{CodeStart}},
		{"how-to starts with a heading", page(HowTo, "## Steps\n\n1. Do it."), []string{CodeStart}},
		{"how-to without steps", page(HowTo, "This guide restarts the app."), []string{CodeSteps}},
		{"how-to fine", page(HowTo, "This guide restarts the app.", "1. Stop it.\n2. Start it."), nil},
		{"explanation starts with too short a line", page(Explanation, "Why so"), []string{CodeStart}},
		{"tutorial fine", page(Tutorial, "In this tutorial you build a page."), nil},
		{"reference may start with a list", page(Reference, "## Ports\n\n- 443"), nil},
		{"structure says nothing of style", page(Register, "| Error | Meaning |\n|---|---|\n| E1 | a |\n| E2 | b |\n| E3 | c |", "[map](plan.png)"), nil},
		{"image only linked", page(Explanation, "The plan is shown here: [the plan](images/plan.png)."), []string{CodeImageLink}},
		{"image shown", page(Explanation, "The plan is shown here.\n\n![the plan](images/plan.png)"), nil},
		{"a link to a page is no image", page(Explanation, "See the plan in [the guide](/o/abcdefghjk)."), nil},
		{"image link in a page without a kind", page("", "See it: [shot](a.JPG)"), []string{CodeKindMissing, CodeImageLink}},
		{"error table", page(Explanation, "These errors come up often.", "| Error | What to do |\n|---|---|\n| E1 | a |\n| E2 | b |\n| E3 | c |"), []string{CodeErrorTable}},
		{"a short table", page(Explanation, "These errors come up often.", "| Error | What to do |\n|---|---|\n| E1 | a |"), nil},
		{"a table of ports", page(Explanation, "These ports are open.", "| Port | Use |\n|---|---|\n| 1 | a |\n| 2 | b |\n| 3 | c |"), nil},
		{"reference without headings", page(Reference, "443 for the web.", "22 for ssh."), []string{CodeStable}},
		{"reference with a heading twice", page(Reference, "## Ports\n\n443", "## Ports\n\n22"), []string{CodeStable}},
		{"reference with parts", page(Reference, "<!-- part t1 -->443<!-- /part t1 -->", "22"), nil},
		{"reference of one block", page(Reference, "443 for the web."), nil},
		{"copied address", page(HowTo, "Connect to the server at 192.0.2.12 first.", "1. Open /etc/app/config.yaml and change it."), []string{CodeCopy, CodeCopy}},
		{"copied host", page(Explanation, "The backups go to backup.example.com every night."), []string{CodeCopy}},
		{"embedded, not copied", page(HowTo, "Connect to the server at :embed{ref=\"ref0000001#b1.t1\"} first.", "1. Go."), nil},
		{"copies in a tutorial are not checked", page(Tutorial, "Connect to the server at 192.0.2.12 first."), nil},
		{"copies in a reference are fine", page(Reference, "Server 192.0.2.12."), nil},
		{"a value not in any reference", page(HowTo, "Connect to the server at 203.0.113.9 first.", "1. Go."), nil},
		{"front matter as a finding", page(HowTo, "---\nkind: how-to\n---\nThis page is for people.", "1. Go."), []string{CodeStart, CodeFrontMatter}},
	}
	for _, c := range cases {
		got := codes(Check(c.p, values))
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	copied := Check(page(HowTo, "Connect to 192.0.2.12.", "1. Go."), values)
	if len(copied) != 1 || copied[0].Source != "ref0000001#b1" || !strings.Contains(copied[0].Message, "Network") ||
		!strings.Contains(copied[0].Fix, "Embed instead of copying") || copied[0].Block != "b1" || copied[0].Level != Warning {
		t.Errorf("COPY names the source and says to embed: %+v", copied)
	}
}

func TestPageRules(t *testing.T) {
	if got := codes(Check(Page{Kind: "manual"}, nil)); !slices.Equal(got, []string{CodeKind}) {
		t.Errorf("an unknown kind: %v", got)
	}
	long := strings.Repeat("a", MaxDescription+1)
	if got := codes(Check(Page{Kind: Reference, Description: long}, nil)); !slices.Equal(got, []string{CodeDescription}) {
		t.Errorf("too long a description: %v", got)
	}
	if got := codes(Check(Page{Kind: Reference, Description: strings.Repeat("ä", MaxDescription)}, nil)); got != nil {
		t.Errorf("a description of %d characters: %v", MaxDescription, got)
	}
	if New(CodeKind, "", "x").Level != Error || New(CodeCopy, "", "a", "b").Level != Warning || New(CodeID, "", "t1").Level != Error {
		t.Error("levels")
	}
	if b := Blocking(Check(page(HowTo, "---\ntitle: x\n---"), nil)); len(b) != 1 || b[0].Code != CodeFrontMatter {
		t.Errorf("Blocking keeps the errors: %+v", b)
	}
}

func TestValues(t *testing.T) {
	got := Values("Open https://wiki.example.com/a?b=1, mail ops@example.com, ssh 192.0.2.20:2222 or 198.51.100.0/24, edit /srv/app/data/ and " +
		"C:\\Tools\\app.exe, ~/.config/app; db.example.com:5432 at app.example.org. Not: README.md, v1.2.3, 1.2.3, e.g. this, /o/abcdefghjk/x, /tmp.")
	want := []string{"https://wiki.example.com/a?b=1", "ops@example.com", "192.0.2.20:2222", "198.51.100.0/24", "/srv/app/data/",
		"C:\\Tools\\app.exe", "~/.config/app", "db.example.com:5432", "app.example.org"}
	if !slices.Equal(got, want) {
		t.Errorf("Values:\n got %q\nwant %q", got, want)
	}
}

// TestParseChecked reads the mark of a check of a team wiki for the import (T-28).
func TestParseChecked(t *testing.T) {
	who, day, ok := ParseChecked(" AB 2026-11-20 ")
	if !ok || who != "AB" || day.Format("2006-01-02") != "2026-11-20" {
		t.Errorf("a mark: %q %v %v", who, day, ok)
	}
	for _, bad := range []string{"", "AB", "AB 20.11.2026", "A B 2026-11-20", strings.Repeat("x", 41) + " 2026-11-20"} {
		if _, _, ok := ParseChecked(bad); ok {
			t.Errorf("%q is no mark", bad)
		}
	}
}

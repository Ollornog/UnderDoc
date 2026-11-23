// SPDX-License-Identifier: MIT

// Package pagekind holds the kinds of pages and their writing rules (T-20, ADR-33): which kinds there are, which
// audience each sets, and the check every door runs when a page is written. The rules sit here once; the database runs
// them at write time (db.Findings, and the errors that block a write), the list of problems runs them over every page,
// and the MCP access (T-21) and the import (T-28) call the same functions.
package pagekind

import (
	"strings"
	"unicode"
)

// Kinds of pages. The empty kind is a prose page without a kind: it counts as human documentation (ADR-33).
const (
	// Structure and records: no kind required, no hint of AI, the check looks at structure only.
	Overview = "overview"
	Register = "register"
	Record   = "record"
	Plan     = "plan"
	Source   = "source"
	// Agent documentation and reference: agents write and change it freely.
	Reference  = "reference"
	AgentNotes = "agent-notes"
	// Human documentation: style comes first, values are embedded, never copied.
	HowTo       = "how-to"
	Explanation = "explanation"
	Tutorial    = "tutorial"
)

// Audiences a kind sets. There is no field of its own for it: it follows from the kind.
const (
	AudienceStructure = "structure"
	AudienceAgents    = "agents"
	AudiencePeople    = "people"
)

// Kinds are all kinds, grouped by audience in the order of ADR-33.
var Kinds = []string{Overview, Register, Record, Plan, Source, Reference, AgentNotes, HowTo, Explanation, Tutorial}

// Valid tells whether a kind can be stored: one of Kinds, or none.
func Valid(kind string) bool {
	if kind == "" {
		return true
	}
	for _, k := range Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// Audience is who a page of this kind is written for. A page without a kind is human documentation.
func Audience(kind string) string {
	switch kind {
	case Overview, Register, Record, Plan, Source:
		return AudienceStructure
	case Reference, AgentNotes:
		return AudienceAgents
	}
	return AudiencePeople
}

// ForPeople tells whether pages of this kind are human documentation, which carries the hints of AI changes.
func ForPeople(kind string) bool { return Audience(kind) == AudiencePeople }

// Parse reads a kind as people and programs write it: the English names in any case, with spaces, underscores or
// hyphens, a few common other spellings, and the German names of FromGerman. ok is false for anything else. An empty
// value is no kind.
func Parse(s string) (kind string, ok bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "", true
	}
	if k, ok := FromGerman(s); ok {
		return k, true
	}
	norm := strings.Map(func(r rune) rune {
		if r == ' ' || r == '_' {
			return '-'
		}
		return r
	}, s)
	switch norm {
	case "howto", "how-to-guide", "guide":
		return HowTo, true
	case "agent-note", "agentnotes", "notes-for-agents":
		return AgentNotes, true
	case "records":
		return Record, true
	}
	if Valid(norm) {
		return norm, true
	}
	return "", false
}

// german maps the values of `art:` and `type:` of a team wiki written in German to kinds. English stays the base; this
// is what an import (T-28) reads.
var german = map[string]string{
	"anleitung":      HowTo,
	"erklärung":      Explanation,
	"erklaerung":     Explanation,
	"tutorial":       Tutorial,
	"referenz":       Reference,
	"agentennotizen": AgentNotes,
	"agent-notizen":  AgentNotes,
	"übersicht":      Overview,
	"uebersicht":     Overview,
	"register":       Register,
	"datensatz":      Record,
	"plan":           Plan,
	"quelle":         Source,
}

// FromGerman reads a German name of a kind.
func FromGerman(s string) (string, bool) {
	k, ok := german[strings.ToLower(strings.TrimSpace(s))]
	return k, ok
}

// FromTeamWiki maps the two axes of a team wiki, `type:` (Regeln, Thema, Register, Plan, Quelle) and `art:` (anleitung,
// referenz, erklärung, tutorial), onto the one kind of UnderDoc, for the import (T-28):
//
//   - Register, Plan and Quelle are structure: register, plan, source, whatever art says.
//   - A page of Regeln or Thema takes the kind its art names.
//   - Regeln without an art become agent notes: rules there are the conventions and instructions agents follow, which
//     is what agent notes are, so they keep no hint per change while people may still edit them.
//   - Thema without an art, and anything unknown, get no kind and count as human documentation, the stricter side.
func FromTeamWiki(typ, art string) string {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "register":
		return Register
	case "plan":
		return Plan
	case "quelle":
		return Source
	}
	if k, ok := Parse(art); ok && k != "" {
		return k
	}
	if strings.EqualFold(strings.TrimSpace(typ), "regeln") {
		return AgentNotes
	}
	return ""
}

// Name is how the interface names a kind.
func Name(kind string) string {
	switch kind {
	case "":
		return "No kind"
	case AgentNotes:
		return "Agent notes"
	case HowTo:
		return "How-to"
	}
	r := []rune(kind)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// SPDX-License-Identifier: MIT

package pagekind

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Levels of a finding. An error blocks the write; a warning comes back with a hint how to fix it and blocks nothing
// (ADR-33: by default nothing waits).
const (
	Error   = "error"
	Warning = "warning"
)

// Codes of the rules. They are English; docs/page-kinds.md maps the codes of the German team wiki onto them.
const (
	// Errors: something is broken, and the one who writes can always fix it in the same write.
	CodeKind        = "KIND"
	CodeFrontMatter = "FRONT-MATTER"
	CodeID          = "ID"
	// Warnings.
	CodeKindMissing  = "KIND-MISSING"
	CodeStart        = "START"
	CodeSteps        = "STEPS"
	CodeCopy         = "COPY"
	CodeImageLink    = "IMAGE-LINK"
	CodeErrorTable   = "ERROR-TABLE"
	CodeStable       = "STABLE-HEADINGS"
	CodeEmbed        = "EMBED"
	CodeDescription  = "DESCRIPTION"
	CodeOverview     = "OVERVIEW"
	CodeFolderSize   = "FOLDER-SIZE"
	MaxDescription   = 120
	MaxPagesInFolder = 25
)

// Finding is one rule a page breaks: its code and level, the block it is in (empty for the page), what is wrong and how
// to fix it. Source names the address a copied value comes from (COPY) or that does not resolve (EMBED).
type Finding struct {
	Code    string `json:"code"`
	Level   string `json:"level"`
	Block   string `json:"block,omitempty"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
	Source  string `json:"source,omitempty"`
}

// texts are the words of every rule in one place, so a translation (T-32) finds them: what is wrong, and how to fix it.
// %s stands for the detail of a finding.
var texts = map[string][2]string{
	CodeKind:        {"%q is not a kind of page.", "Choose one of: " + strings.Join(Kinds, ", ") + ", or none."},
	CodeFrontMatter: {"The block starts with front matter.", "Set the kind, title, description and tags of the page through their fields, not in the text."},
	CodeID:          {"The part ID %s is used twice in this block.", "Give each part its own ID, or mark the text again to get a new one."},
	CodeKindMissing: {"The page has no kind, so it counts as human documentation.", "Choose its kind: how-to, explanation, tutorial or reference, or a structure kind such as overview or record."},
	CodeStart:       {"The page does not start with a sentence for people.", "Begin with one or two sentences that say what the page is for, before any list, table or heading."},
	CodeSteps:       {"A how-to without numbered steps.", "Write the steps as a numbered list (1. 2. 3.), each with what to do and why."},
	CodeCopy:        {"The value %s is copied from %s.", "Embed instead of copying: mark the value as a part there and embed it with :embed{ref=…}."},
	CodeImageLink:   {"The image %s is only linked.", "Show the image at its step with ![what it shows](…) instead of a link."},
	CodeErrorTable:  {"Errors are listed in a table.", "Give each error a heading of its own, with what it means and what to do, so it can be found and linked."},
	CodeStable:      {"The reference has %s.", "Give every section a heading of its own, once in the page, so it can be embedded elsewhere."},
	CodeEmbed:       {"The embedding %s finds nothing.", "Point it at a page, block or part that exists, or remove it."},
	CodeDescription: {"The description has %d characters.", fmt.Sprintf("Keep it to one sentence of at most %d characters; it shows on top as the summary.", MaxDescription)},
	CodeOverview:    {"The folder has no overview page.", "Add a page of the kind overview that says what the folder holds and where to start."},
	CodeFolderSize:  {"The folder holds %d pages.", fmt.Sprintf("Keep at most %d pages in a folder; group them into folders below it.", MaxPagesInFolder)},
}

// New makes a finding of a rule with its words, filled in with the details.
func New(code, block string, args ...any) Finding {
	t := texts[code]
	msg := t[0]
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	level := Warning
	if code == CodeKind || code == CodeFrontMatter || code == CodeID {
		level = Error
	}
	return Finding{Code: code, Level: level, Block: block, Message: msg, Fix: t[1]}
}

// Block is one block of a page as the check sees it.
type Block struct {
	ID   string
	Body string
}

// Page is a page as the check sees it.
type Page struct {
	Kind        string
	Description string
	Blocks      []Block
}

// Value is a value that stands in a reference or a record, with the address of where it stands.
type Value struct {
	Text    string
	Address string
	Title   string
}

var (
	fence       = regexp.MustCompile("(?s)(?m)^[ \t]*(```|~~~).*?^[ \t]*(```|~~~)[ \t]*$")
	frontMatter = regexp.MustCompile(`\A---[ \t]*\r?\n((?:[A-Za-z_][A-Za-z0-9_-]*:[^\n]*\r?\n|[ \t]+[^\n]*\r?\n|-[^\n]*\r?\n)+)---[ \t]*(?:\r?\n|\z)`)
	frontKey    = regexp.MustCompile(`(?m)^(id|title|kind|art|type|description|tags|status|created|timestamp):`)
	partOpen    = regexp.MustCompile(`<!-- part (t[1-9][0-9]{0,8})(?: name="[^"<>\n\\]{1,80}")? -->`)
	ordered     = regexp.MustCompile(`(?m)^[ \t]{0,3}[0-9]{1,9}[.)][ \t]+\S`)
	imageLink   = regexp.MustCompile(`(^|[^!\]])\[[^\]\n]*\]\(<?([^)\s>]+\.(?i:png|jpe?g|gif|webp|svg|avif|bmp))(?:[?#][^)\s>]*)?>?(?:[ \t]+"[^"\n]*")?\)`)
	tableRow    = regexp.MustCompile(`(?m)^[ \t]*\|.*\|[ \t]*$`)
	errorHead   = regexp.MustCompile(`(?i)\b(error|errors|fault|faults|problem|problems|symptom|symptoms|message|code|fehler|fehlermeldung|meldung|störung)\b`)
	headingLine = regexp.MustCompile(`(?m)^[ \t]{0,3}(#{1,6})[ \t]+(.+?)[ \t#]*$`)
	nonSentence = regexp.MustCompile(`^[ \t]*(?:[#>|*+-]|[0-9]{1,9}[.)][ \t]|!\[|::|:embed|<!--|<|---|===|\[[^\]]*\]\([^)]*\)[ \t]*$)`)
)

// Refuse runs the rules that block a write on one block: front matter in the text and a part ID used twice. It is what
// every door checks before a block is stored.
func Refuse(blockID, body string) []Finding {
	var out []Finding
	if m := frontMatter.FindStringSubmatch(body); m != nil && frontKey.MatchString(m[1]) {
		out = append(out, New(CodeFrontMatter, blockID))
	}
	seen := map[string]bool{}
	for _, m := range partOpen.FindAllStringSubmatch(body, -1) {
		if seen[m[1]] {
			out = append(out, New(CodeID, blockID, m[1]))
			continue
		}
		seen[m[1]] = true
	}
	return out
}

// Check runs every rule of the kind of a page on it, values being what references and records hold for COPY. The
// findings come in the order of the blocks, the page's own first.
func Check(p Page, values []Value) []Finding {
	var out []Finding
	if !Valid(p.Kind) {
		return []Finding{New(CodeKind, "", p.Kind)}
	}
	if n := len([]rune(strings.TrimSpace(p.Description))); n > MaxDescription {
		out = append(out, New(CodeDescription, "", n))
	}
	written := slices.ContainsFunc(p.Blocks, func(b Block) bool { return strings.TrimSpace(b.Body) != "" })
	if !written {
		return out
	}
	audience := Audience(p.Kind)
	if p.Kind == "" {
		out = append(out, New(CodeKindMissing, ""))
	}
	if p.Kind != "" && audience == AudiencePeople {
		if first := firstBlock(p.Blocks); first != nil && !opensWithSentence(first.Body) {
			out = append(out, New(CodeStart, first.ID))
		}
	}
	if p.Kind == HowTo && !slices.ContainsFunc(p.Blocks, func(b Block) bool { return ordered.MatchString(fence.ReplaceAllString(b.Body, "")) }) {
		out = append(out, New(CodeSteps, ""))
	}
	if p.Kind == Reference {
		out = append(out, stable(p.Blocks)...)
	}
	for _, b := range p.Blocks {
		out = append(out, Refuse(b.ID, b.Body)...)
		prose := fence.ReplaceAllString(b.Body, "")
		if audience == AudiencePeople {
			for _, m := range imageLink.FindAllStringSubmatch(prose, -1) {
				out = append(out, New(CodeImageLink, b.ID, m[2]))
			}
			if errorTable(prose) {
				out = append(out, New(CodeErrorTable, b.ID))
			}
		}
		if p.Kind == HowTo || p.Kind == Explanation {
			out = append(out, copies(b, values)...)
		}
	}
	return out
}

// Blocking returns the errors among findings.
func Blocking(findings []Finding) []Finding {
	var out []Finding
	for _, f := range findings {
		if f.Level == Error {
			out = append(out, f)
		}
	}
	return out
}

func firstBlock(blocks []Block) *Block {
	for i := range blocks {
		if strings.TrimSpace(blocks[i].Body) != "" {
			return &blocks[i]
		}
	}
	return nil
}

// opensWithSentence tells whether the first thing of a block is a paragraph of at least three words, not a heading, a
// list, a table, a quote, an image, an embedding or code.
func opensWithSentence(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if nonSentence.MatchString(line) || strings.HasPrefix(strings.TrimSpace(line), "```") || strings.HasPrefix(strings.TrimSpace(line), "~~~") {
			return false
		}
		return len(strings.Fields(line)) >= 3
	}
	return false
}

// stable checks that a reference can be embedded part by part: it has headings, and none of them twice.
func stable(blocks []Block) []Finding {
	seen := map[string]bool{}
	any := false
	var out []Finding
	for _, b := range blocks {
		if partOpen.MatchString(b.Body) {
			any = true
		}
		for _, m := range headingLine.FindAllStringSubmatch(fence.ReplaceAllString(b.Body, ""), -1) {
			any = true
			key := strings.ToLower(strings.TrimSpace(m[2]))
			if seen[key] {
				out = append(out, New(CodeStable, b.ID, fmt.Sprintf("the heading %q twice", strings.TrimSpace(m[2]))))
			}
			seen[key] = true
		}
	}
	if !any && len(blocks) > 1 {
		out = append(out, New(CodeStable, "", "no headings"))
	}
	return out
}

// errorTable tells whether a block holds a table of errors: a header that names errors, and at least three rows below.
func errorTable(prose string) bool {
	rows := tableRow.FindAllString(prose, -1)
	return len(rows) >= 5 && errorHead.MatchString(rows[0])
}

// copies finds values of references and records written down again in a block.
func copies(b Block, values []Value) []Finding {
	if len(values) == 0 {
		return nil
	}
	text := embedRef.ReplaceAllString(b.Body, "")
	var out []Finding
	reported := map[string]bool{}
	for _, v := range Values(text) {
		if reported[v] {
			continue
		}
		for _, src := range values {
			if src.Text == v {
				f := New(CodeCopy, b.ID, v, src.Title)
				f.Source = src.Address
				out = append(out, f)
				reported[v] = true
				break
			}
		}
	}
	return out
}

var (
	embedRef = regexp.MustCompile(`:{1,2}embed\{[^}\n]*\}`)
	// valuePattern finds what counts as a value that changes (T-20): addresses, e-mail, URLs, paths, hosts with a port.
	valuePattern = regexp.MustCompile(`(?i)` +
		`https?://[^\s<>()\[\]"'` + "`" + `]+` + // a URL
		`|[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}` + // an e-mail address
		`|\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}(?:/[0-9]{1,2})?(?::[0-9]{1,5})?\b` + // an IPv4 address, a network or with a port
		`|(?:^|[\s(` + "`" + `'"])((?:~|[a-z]:)?(?:[/\\][a-z0-9._-]+){2,}[/\\]?)` + // a path of two levels or more
		`|\b[a-z0-9-]+(?:\.[a-z0-9-]+)+:[0-9]{2,5}\b` + // a host with a port
		`|\b[a-z0-9-]{2,}(?:\.[a-z0-9-]{2,})*\.[a-z]{2,24}\b`) // a host name
	// notHosts are endings of names of files, which look like host names.
	notHosts = map[string]bool{"md": true, "js": true, "mjs": true, "ts": true, "go": true, "py": true, "rs": true, "sh": true,
		"txt": true, "json": true, "yaml": true, "yml": true, "toml": true, "html": true, "css": true, "png": true, "jpg": true,
		"jpeg": true, "gif": true, "svg": true, "pdf": true, "csv": true, "xlsx": true, "docx": true, "sql": true, "conf": true,
		"log": true, "zip": true, "service": true, "timer": true, "env": true, "lock": true, "mod": true, "sum": true}
)

// Values returns the values a text holds, as COPY compares them: addresses, e-mail, URLs, paths and host names, each once
// and without punctuation at its end.
func Values(text string) []string {
	var out []string
	for _, m := range valuePattern.FindAllStringSubmatch(text, -1) {
		v := m[0]
		if m[1] != "" {
			v = m[1]
		}
		v = strings.TrimRight(v, ".,;:!?)")
		// A link to a page of UnderDoc is no value: it is how pages point at each other.
		if len(v) < 6 || strings.HasPrefix(v, "/o/") {
			continue
		}
		if !strings.ContainsAny(v, "/@:\\") {
			dot := strings.LastIndexByte(v, '.')
			if dot < 0 || notHosts[strings.ToLower(v[dot+1:])] || isVersion(v) {
				continue
			}
		}
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// isVersion tells a version such as 1.2.3 from a host name. Four numbers are an address of IPv4.
func isVersion(v string) bool {
	return strings.Count(v, ".") != 3 && strings.IndexFunc(v, func(r rune) bool { return r != '.' && (r < '0' || r > '9') && r != 'v' }) < 0
}

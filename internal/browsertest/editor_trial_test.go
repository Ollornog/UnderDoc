//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// trialEditors are the candidates on /dev/editor-trial, in the order of the page.
var trialEditors = []string{"milkdown", "tiptap", "codemirror"}

// shortcut is Markdown typed into an empty editor with real key input, and where the editor shows what it made of it.
type shortcut struct {
	Name, Typed, Selector string
}

// trialTodo finds a to-do: TipTap's task item, the checkbox label of Crepe's list item, the line class of CodeMirror.
const trialTodo = `input[type=checkbox], li[data-type="taskItem"], li[data-item-type="task"], .milkdown-list-item-block .checked, .milkdown-list-item-block .unchecked, .cm-lp-task`

// The selectors name the rendered element of a WYSIWYG editor and the class the live preview of CodeMirror gives a line.
var trialShortcuts = []shortcut{
	{"heading", "## Title", "h2, .cm-lp-h2"},
	{"bold", "**b**", "strong, .cm-lp-strong"},
	{"italic", "*i*", "em, .cm-lp-em"},
	{"inline code", "`c`", "code, .cm-lp-icode"},
	{"bullet list", "- item", "ul li, .cm-lp-bullet"},
	{"numbered list", "1. item", "ol li, .cm-lp-ordered"},
	{"quote", "> q", "blockquote, .cm-lp-quote"},
	{"to-do", "[ ] todo", trialTodo},
	{"to-do in list", "- [ ] todo", trialTodo},
	{"code fence + Enter", "```\ncode", "pre, .milkdown-code-block, .cm-lp-code"},
	{"code fence + space", "``` code", "pre, .milkdown-code-block, .cm-lp-code"},
	{"link", "[a](/o/x)", "a[href], .cm-lp-link"},
	{"rule", "---", "hr, .cm-lp-hr"},
}

type shortcutResult struct {
	Name      string `json:"name"`
	Typed     string `json:"typed"`
	Converted bool   `json:"converted"`
	Markdown  string `json:"markdown"`
	// Shown is the text the editor shows once it has lost the focus: markers that are still there were not rendered.
	Shown string `json:"shown"`
}

type roundTrip struct {
	Identical bool     `json:"identical"`
	Diff      []string `json:"diff"`
	Markdown  string   `json:"markdown"`
}

type trialEditor struct {
	State      string            `json:"state"`
	Violations []json.RawMessage `json:"cspViolations"`
	Errors     []json.RawMessage `json:"errors"`
	RoundTrip  roundTrip         `json:"roundTrip"`
	Shortcuts  []shortcutResult  `json:"shortcuts"`
	SlashMenu  bool              `json:"slashMenu"`
}

type trialReport struct {
	Page struct {
		ForeignRequests []string `json:"foreignRequests"`
		Failed          []string `json:"failedRequests"`
		CSP             []string `json:"cspConsole"`
		Errors          []string `json:"consoleErrors"`
		PageViolations  []string `json:"unattributedViolations"`
	} `json:"page"`
	Editors map[string]*trialEditor `json:"editors"`
}

// typeKeys types key by key, unlike typeText: every character is a keyDown with its text, a newline is Enter. Input
// rules of an editor see what a keyboard would send.
func (b *browser) typeKeys(text string) {
	b.t.Helper()
	for _, r := range text {
		if r == '\n' {
			for _, kind := range []string{"keyDown", "keyUp"} {
				params := map[string]any{"type": kind, "key": "Enter", "code": "Enter", "windowsVirtualKeyCode": 13}
				if kind == "keyDown" {
					params["text"] = "\r"
				}
				b.call("Input.dispatchKeyEvent", params, nil)
			}
			continue
		}
		ch := string(r)
		down := map[string]any{"type": "keyDown", "key": ch, "text": ch, "unmodifiedText": ch}
		up := map[string]any{"type": "keyUp", "key": ch}
		if r == ' ' {
			for _, p := range []map[string]any{down, up} {
				p["code"], p["windowsVirtualKeyCode"] = "Space", 32
			}
		}
		b.call("Input.dispatchKeyEvent", down, nil)
		b.call("Input.dispatchKeyEvent", up, nil)
		// A quick typist, not a machine: some editors finish their work on a key in a later task.
		time.Sleep(15 * time.Millisecond)
	}
}

// drain reads the events that came after the page loaded, without waiting.
func (b *browser) drain() (csp, errors []string) {
	for {
		select {
		case m, open := <-b.events:
			if !open {
				return
			}
			switch m.Method {
			case "Log.entryAdded":
				var p struct {
					Entry struct{ Source, Level, Text string }
				}
				_ = json.Unmarshal(m.Params, &p)
				if p.Entry.Source == "security" || strings.Contains(p.Entry.Text, "Content Security Policy") {
					csp = append(csp, p.Entry.Text)
				} else if p.Entry.Level == "error" {
					errors = append(errors, p.Entry.Source+": "+p.Entry.Text)
				}
			case "Runtime.exceptionThrown":
				errors = append(errors, "exception: "+string(m.Params))
			}
		default:
			return
		}
	}
}

func (b *browser) shotClip(name, selector string) {
	b.t.Helper()
	folder := os.Getenv("BROWSER_SHOTS")
	if folder == "" {
		return
	}
	r := b.rect(selector)
	var result struct{ Data string }
	b.call("Page.captureScreenshot", map[string]any{"format": "png", "captureBeyondViewport": true,
		"clip": map[string]any{"x": r.Left, "y": r.Top + b.scrollY(), "width": r.Width, "height": r.Bottom - r.Top, "scale": 1}}, &result)
	b.save(folder, name, result.Data)
}

func (b *browser) scrollY() float64 {
	y, _ := strconv.ParseFloat(b.eval(`scrollY`), 64)
	return y
}

func (b *browser) save(folder, name, data string) {
	b.t.Helper()
	picture, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		b.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, name+".png"), picture, 0o644); err != nil {
		b.t.Fatal(err)
	}
}

// TestEditorTrial loads the three editor candidates of M-8 side by side and records how each one does: mounting under
// the strict CSP, Markdown typed with real keys, and the round trip of the fixture. Shortcomings of an editor are the
// result, not a failure; the test fails only when the page or the harness breaks. The comparison goes to the log and,
// as JSON, to a file whose path the log names.
func TestEditorTrial(t *testing.T) {
	b, base := setup(t)
	b.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": 1680, "height": 1300, "deviceScaleFactor": 1, "mobile": false}, nil)
	f := b.open(base + "/dev/editor-trial")
	if b.eval(`typeof window.editorTrial`) != "object" {
		t.Fatalf("the trial page did not start: %v %v %v", f.failed, f.csp, f.errors)
	}
	for _, name := range trialEditors {
		if b.eval(`!!document.querySelector('[data-editor="`+name+`"]')`) != "true" {
			t.Fatalf("the column of %s is missing", name)
		}
	}
	// Mounting may load chunks; give every editor its time.
	settled := `[...document.querySelectorAll('[data-editor]')].every(c => c.dataset.state === 'ready' || c.dataset.state === 'failed')`
	deadline := time.Now().Add(20 * time.Second)
	for b.eval(settled) != "true" {
		if time.Now().After(deadline) {
			t.Logf("not every editor settled: %s", b.eval(`[...document.querySelectorAll('[data-editor]')].map(c => c.dataset.editor + '=' + c.dataset.state).join(' ')`))
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	b.waitFor(`[...document.querySelectorAll('[data-note]')].every(n => n.textContent.length > 0)`)
	time.Sleep(300 * time.Millisecond)

	report := trialReport{Editors: map[string]*trialEditor{}}
	own, _ := url.Parse(base)
	for _, request := range f.requests {
		if u, err := url.Parse(request); err != nil || (u.Scheme != "data" && u.Host != own.Host) {
			report.Page.ForeignRequests = append(report.Page.ForeignRequests, request)
		}
	}
	report.Page.Failed, report.Page.CSP, report.Page.Errors = f.failed, f.csp, f.errors

	b.shot("editor-trial")
	// Each editor with its whole document: the hosts grow to their content for these pictures.
	b.eval(`document.body.classList.add('et-tall')`)
	time.Sleep(300 * time.Millisecond)
	for _, name := range trialEditors {
		b.shotClip("editor-trial-"+name, `[data-editor="`+name+`"]`)
	}
	b.eval(`document.body.classList.remove('et-tall')`)
	b.eval(`scrollTo(0, 0)`)
	// The live preview of CodeMirror shows a table as Markdown while the cursor is in it. The editor is in a shadow root.
	cell := `window.editorTrial.editors.codemirror?.scope.querySelector('.cm-lp-table-widget td')`
	if b.eval(`!!`+cell) == "true" {
		b.eval(cell + `.scrollIntoView({block: 'center'})`)
		var r box
		if err := json.Unmarshal([]byte(b.eval(`JSON.stringify((() => { const r = `+cell+`.getBoundingClientRect(); return {X: r.left + r.width / 2, Y: r.top + r.height / 2}; })())`)), &r); err != nil {
			t.Fatal(err)
		}
		b.mouse("mouseMoved", r.X, r.Y, 0, 0)
		b.mouse("mousePressed", r.X, r.Y, 1, 0)
		b.mouse("mouseReleased", r.X, r.Y, 1, 0)
		time.Sleep(200 * time.Millisecond)
		b.shotClip("editor-trial-codemirror-cursor-in-table", `[data-editor="codemirror"] [data-host]`)
		b.eval(`window.editorTrial.editors.codemirror.scope.blur()`)
		b.eval(`scrollTo(0, 0)`)
	}

	for _, name := range trialEditors {
		e := &trialEditor{State: b.eval(`document.querySelector('[data-editor="` + name + `"]').dataset.state`)}
		report.Editors[name] = e
		if e.State != "ready" {
			continue
		}
		raw := b.eval(`JSON.stringify(window.editorTrial.editors.` + name + `.initial)`)
		if err := json.Unmarshal([]byte(raw), &e.RoundTrip); err != nil {
			t.Fatalf("%s: the round trip is not readable: %s", name, raw)
		}
		scope := `window.editorTrial.editors.` + name + `.scope`
		for _, s := range trialShortcuts {
			b.eval(`window.editorTrial.editors.` + name + `.reset('')`)
			time.Sleep(80 * time.Millisecond)
			b.typeKeys(s.Typed)
			time.Sleep(150 * time.Millisecond)
			r := shortcutResult{Name: s.Name, Typed: s.Typed}
			r.Converted = b.eval(`!!`+scope+`.querySelector(`+strconv.Quote(s.Selector)+`)`) == "true"
			r.Markdown = strings.TrimSpace(b.eval(`window.editorTrial.editors.` + name + `.getMarkdown()`))
			b.eval(scope + `.blur()`)
			time.Sleep(80 * time.Millisecond)
			r.Shown = strings.TrimSpace(b.eval(scope + `.innerText`))
			e.Shortcuts = append(e.Shortcuts, r)
		}
		// A slash on an empty line: only Crepe brings a menu for it; the others would need one built.
		b.eval(`window.editorTrial.editors.` + name + `.reset('')`)
		time.Sleep(80 * time.Millisecond)
		b.typeKeys("/")
		time.Sleep(300 * time.Millisecond)
		e.SlashMenu = b.eval(`!!`+scope+`.getRootNode().querySelector('.milkdown-slash-menu[data-show="true"]')`) == "true"
		if e.SlashMenu {
			b.shotClip("editor-trial-"+name+"-slash-menu", `[data-editor="`+name+`"]`)
		}
		b.eval(`window.editorTrial.editors.` + name + `.reset(window.editorTrial.fixture)`)
		b.eval(scope + `.blur()`)
	}

	csp, errors := b.drain()
	report.Page.CSP = append(report.Page.CSP, csp...)
	report.Page.Errors = append(report.Page.Errors, errors...)
	for _, list := range []string{"violations", "errors"} {
		raw := b.eval(`JSON.stringify(window.editorTrial.` + list + `)`)
		var entries []json.RawMessage
		if err := json.Unmarshal([]byte(raw), &entries); err != nil {
			t.Fatalf("%s: %s", list, raw)
		}
		for _, entry := range entries {
			var head struct{ Bundle string }
			_ = json.Unmarshal(entry, &head)
			e, ok := report.Editors[head.Bundle]
			switch {
			case !ok:
				report.Page.PageViolations = append(report.Page.PageViolations, list+": "+string(entry))
			case list == "violations":
				e.Violations = append(e.Violations, entry)
			default:
				e.Errors = append(e.Errors, entry)
			}
		}
	}

	b.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": 420, "height": 900, "deviceScaleFactor": 1, "mobile": false}, nil)
	b.eval(`scrollTo(0, 0)`)
	time.Sleep(200 * time.Millisecond)
	b.shot("editor-trial-narrow")
	if got := b.eval(`getComputedStyle(document.querySelector('.et-grid')).gridTemplateColumns.split(' ').length`); got != "1" {
		t.Errorf("at narrow width the editors should stack, the grid has %s columns", got)
	}

	logTrial(t, report)
	folder, err := os.MkdirTemp("", "underdoc-editor-trial-")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(folder, "editor-trial.json")
	encoded, _ := json.MarshalIndent(report, "", "  ")
	if err := os.WriteFile(out, encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("the comparison as JSON: %s", out)
}

func mark(ok bool) string {
	if ok {
		return "✅"
	}
	return "❌"
}

func logTrial(t *testing.T, r trialReport) {
	t.Helper()
	var s strings.Builder
	row := func(cells ...string) {
		s.WriteString("| " + strings.Join(cells, " | ") + " |\n")
	}
	s.WriteString("\n")
	row(append([]string{"Editor trial"}, trialEditors...)...)
	row("---", "---", "---", "---")
	cells := func(f func(e *trialEditor) string) []string {
		out := []string{}
		for _, name := range trialEditors {
			out = append(out, f(r.Editors[name]))
		}
		return out
	}
	row(append([]string{"mounted"}, cells(func(e *trialEditor) string { return mark(e.State == "ready") + " " + e.State })...)...)
	row(append([]string{"CSP violations"}, cells(func(e *trialEditor) string {
		return mark(len(e.Violations) == 0) + " " + strconv.Itoa(len(e.Violations))
	})...)...)
	row(append([]string{"errors"}, cells(func(e *trialEditor) string { return mark(len(e.Errors) == 0) + " " + strconv.Itoa(len(e.Errors)) })...)...)
	row(append([]string{"slash menu on `/`"}, cells(func(e *trialEditor) string { return mark(e.SlashMenu) })...)...)
	row(append([]string{"fixture round trip"}, cells(func(e *trialEditor) string {
		if e.RoundTrip.Identical {
			return "✅ identical"
		}
		return fmt.Sprintf("❌ %d lines differ", len(e.RoundTrip.Diff))
	})...)...)
	for i, sc := range trialShortcuts {
		row(append([]string{"typed `" + strings.ReplaceAll(sc.Typed, "\n", "⏎") + "`"}, cells(func(e *trialEditor) string {
			if i >= len(e.Shortcuts) {
				return "–"
			}
			res := e.Shortcuts[i]
			return mark(res.Converted) + " " + strconv.Quote(res.Markdown) + " shows " + strconv.Quote(res.Shown)
		})...)...)
	}
	t.Log(s.String())
	for _, name := range trialEditors {
		e := r.Editors[name]
		if len(e.RoundTrip.Diff) > 0 {
			t.Logf("%s round trip, - fixture / + editor:\n  %s", name, strings.Join(e.RoundTrip.Diff, "\n  "))
		}
		for _, v := range e.Violations {
			t.Logf("%s CSP: %s", name, v)
		}
		for _, v := range e.Errors {
			t.Logf("%s error: %s", name, v)
		}
	}
	for _, list := range [][]string{r.Page.ForeignRequests, r.Page.Failed, r.Page.CSP, r.Page.Errors, r.Page.PageViolations} {
		for _, entry := range list {
			t.Logf("page: %s", entry)
		}
	}
}

//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/web"
)

// diagramText reads the text of the drawn diagram out of its image.
const diagramText = `(() => { const img = document.querySelector('figure[data-mermaid] img[data-diagram]');
	if (!img) return '';
	const svg = new DOMParser().parseFromString(decodeURIComponent(img.src.split(',')[1]), 'image/svg+xml');
	return svg.documentElement.textContent.replace(/\s+/g, ' ').trim(); })()`

// A Mermaid diagram draws under the production policy of the workspace (T-3): no inline style and no inline script for
// the app. Mermaid draws in a sandboxed frame without an origin of its own, the workspace shows the result as an image, and
// a value embedded in the diagram is drawn with its text.
func TestADiagramDrawsUnderTheStrictPolicy(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	rates := makePage(b, "Parts", `The router is the <!-- part t1 name="router" -->edge router<!-- /part t1 -->.`)
	page := makePage(b, "Diagram", "```mermaid\nflowchart LR\n  A[:embed{ref=\""+rates+"#b1.t1\"}] --> B[core switch]\n"+
		"  B --> C[\"<img src=x onerror=parent.injected=1>\"]\n  click B call parent.injected()\n```")
	b.open(base + "/o/" + page)
	b.ready("Diagram")
	if !b.waitFor(`document.querySelector('figure[data-mermaid] img[data-diagram]')?.naturalWidth > 0`) {
		t.Fatalf("no diagram is drawn: %s", b.eval(`document.querySelector('figure[data-mermaid]')?.outerHTML.slice(0, 600)`))
	}
	text := b.eval(diagramText)
	for _, want := range []string{"edge router", "core switch"} {
		if !strings.Contains(text, want) {
			t.Errorf("the diagram does not show %q: %s", want, text)
		}
	}
	if b.eval(`document.querySelector('figure[data-mermaid] pre').hidden`) != "true" {
		t.Error("the source still shows next to the drawn diagram")
	}
	// The frame has no origin of its own: the workspace cannot reach into it, nor it into the workspace.
	if got := b.eval(`(() => { const f = document.querySelector('iframe[data-diagrams]'); return f.getAttribute('sandbox') + ' ' + (f.contentDocument === null); })()`); got != "allow-scripts true" {
		t.Errorf("the frame of the diagrams is not sandboxed: %s", got)
	}
	if b.eval(`typeof window.injected`) != "undefined" {
		t.Error("the diagram ran code in the workspace")
	}
	if csp, errors := b.drain(); len(csp) > 0 || len(errors) > 0 {
		t.Errorf("csp %v, errors %v", csp, errors)
	}
	b.shot("diagram")

	// The workspace keeps its policy; only the frame gets one of its own, sandboxed and only for UnderDoc to hold.
	for path, want := range map[string]string{"/o/" + page: web.CSP, "/assets/app/mermaid/frame.html": "sandbox allow-scripts"} {
		r, err := http.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = r.Body.Close()
		policy := r.Header.Get("Content-Security-Policy")
		if !strings.Contains(policy, want) {
			t.Errorf("%s: %q", path, policy)
		}
		if path != "/o/"+page && (!strings.Contains(policy, "frame-ancestors 'self'") || strings.Contains(policy, "allow-same-origin") || r.Header.Get("X-Frame-Options") != "") {
			t.Errorf("the frame may be held by others or gets an origin: %q, X-Frame-Options %q", policy, r.Header.Get("X-Frame-Options"))
		}
	}
	if strings.Contains(web.CSP, "unsafe-inline") {
		t.Errorf("the workspace allows inline code: %s", web.CSP)
	}
}

// A diagram Mermaid cannot read says so and keeps its source in sight.
func TestABrokenDiagramSaysSo(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	page := makePage(b, "Broken diagram", "```mermaid\nflowchart LR\n  A --> \n  ]]] nonsense [[[\n```")
	b.open(base + "/o/" + page)
	b.ready("Broken diagram")
	if !b.waitFor(`/This diagram cannot be drawn/.test(document.querySelector('figure[data-mermaid] [data-diagram]')?.textContent || '')`) {
		t.Fatalf("a broken diagram does not say so: %s", b.eval(`document.querySelector('figure[data-mermaid]')?.outerHTML.slice(0, 600)`))
	}
	if b.eval(`document.querySelector('figure[data-mermaid] pre').hidden`) != "false" {
		t.Error("the source of a broken diagram is hidden")
	}
}

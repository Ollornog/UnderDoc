//go:build werkzeuge

// Package werkzeuge pinnt die Prüfwerkzeuge der Suite (staticcheck, govulncheck) in einem EIGENEN
// Modul: ihre Abhängigkeiten geraten so nicht in den Modulgraphen des Servers.
//
// Die Blank-Imports machen die Werkzeuge zu DIREKTEN Abhängigkeiten. Dependabot hebt nur solche;
// die `tool`-Direktive allein erkennt es (Stand 2026-10) nicht — dependabot-core PR #13655 offen.
// Ausgeführt wird über `go tool` (Direktiven unten in go.mod) bzw. gebaut von scripts/_go_check.sh.
package werkzeuge

import (
	_ "golang.org/x/vuln/cmd/govulncheck"
	_ "honnef.co/go/tools/cmd/staticcheck"
)

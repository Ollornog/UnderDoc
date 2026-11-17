// SPDX-License-Identifier: MIT

package search

import (
	"slices"
	"testing"
)

func TestFindCodes(t *testing.T) {
	cases := []struct {
		text  string
		codes []string
	}{
		{"Fehler E-1042 an Kasse 3", []string{"e1042"}},
		{"Update scheitert mit 0x80070643", []string{"0x80070643"}},
		{"ORA-00942: Tabelle fehlt", []string{"ora00942"}},
		{"Der Shop meldet HTTP 503.", []string{"http503"}},
		{"KB5034441 und KB 5034441", []string{"kb5034441", "kb5034441"}},
		{"Druckerfehler 79 im Büro", []string{"fehler79"}},
		{"VPN-Fehler 809", []string{"fehler809"}},
		{"Event ID 4625 häuft sich", []string{"id4625"}},
		{"E_ACCESSDENIED und max_connections", []string{"eaccessdenied", "maxconnections"}},
		{"NET::ERR_CERT_DATE_INVALID", []string{"errcertdateinvalid"}},
		{"550 5.7.1 Relaying denied", []string{"550", "571"}},
		{"SQLSTATE 40P01 und SQLSTATE 23505", []string{"40p01", "sqlstate23505"}},
		{"Seite 12 von 30, Windows 11, Kapitel 2", nil},
		{"Kartenzahlung und Druckerwarteschlange", nil},
	}
	for _, c := range cases {
		var got []string
		for _, token := range FindCodes(c.text) {
			got = append(got, token.Code)
		}
		if !slices.Equal(got, c.codes) {
			t.Errorf("%q: %v, want %v", c.text, got, c.codes)
		}
	}
}

func TestCodeParts(t *testing.T) {
	cases := map[string][]string{
		"ORA-00942":  {"00942", "942", "ora942"},
		"0x80070005": {"80070005"},
		"KB5034441":  {"5034441"},
		"HTTP 503":   {"503"},
	}
	for text, want := range cases {
		tokens := FindCodes(text)
		if len(tokens) != 1 {
			t.Fatalf("%q: %d codes", text, len(tokens))
		}
		for _, part := range want {
			if !slices.Contains(tokens[0].Parts, part) {
				t.Errorf("%q: parts %v lack %s", text, tokens[0].Parts, part)
			}
		}
	}
}

func TestParse(t *testing.T) {
	q := Parse("Kartenterminal E-1042 antwortet nicht")
	if !slices.Equal(q.Codes, []string{"e1042"}) || !slices.Equal(q.Words, []string{"Kartenterminal", "antwortet", "nicht"}) {
		t.Errorf("codes and words apart: %+v", q)
	}
	if q := Parse("0x80070015"); len(q.Words) != 0 {
		t.Errorf("a code is never a word, so it never takes a typing error: %+v", q)
	}
}

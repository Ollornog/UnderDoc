// SPDX-License-Identifier: MIT

package sheet

import "testing"

// TestNumberFormats holds number formats to what Excel shows, in English and German (ADR-31).
func TestNumberFormats(t *testing.T) {
	cases := []struct {
		v      float64
		code   string
		en, de string
	}{
		{1234.5, "General", "1234.5", "1234,5"},
		{1.0 / 3, "General", "0.333333333", "0,333333333"},
		{123456789012, "General", "1.23457E+11", "1,23457E+11"},
		{0.1 + 0.2, "General", "0.3", "0,3"},
		{-5, "", "-5", "-5"},
		{1e-10, "General", "1E-10", "1E-10"},
		{0, "General", "0", "0"},
		{2.345, "0.00", "2.35", "2,35"},
		{2.5, "0", "3", "3"},
		{-2.5, "0", "-3", "-3"},
		{1234567.8, "#,##0", "1,234,568", "1.234.568"},
		{-1234.5, "#,##0.00", "-1,234.50", "-1.234,50"},
		{0.256, "0%", "26%", "26%"},
		{0.256, "0.00%", "25.60%", "25,60%"},
		{12345, "0.00E+00", "1.23E+04", "1,23E+04"},
		{0.000123, "0.00E+00", "1.23E-04", "1,23E-04"},
		{12345, "##0.0E+0", "12.3E+3", "12,3E+3"},
		{1.5, "# ?/?", "1 1/2", "1 1/2"},
		{3.14159, "# ??/??", "3 14/99", "3 14/99"},
		{0.3, "?/8", "2/8", "2/8"},
		{2, "# ?/?", "2", "2"},
		{45000, "yyyy-mm-dd", "2023-03-15", "2023-03-15"},
		{45000, "d-mmm-yy", "15-Mar-23", "15-Mär-23"},
		{45000, "mmmm yyyy", "March 2023", "März 2023"},
		{45000, "dddd", "Wednesday", "Mittwoch"},
		{45000, "DD.MM.YYYY", "15.03.2023", "15.03.2023"},
		{45000.75, "dd.mm.yyyy hh:mm", "15.03.2023 18:00", "15.03.2023 18:00"},
		{0.75, "h:mm AM/PM", "6:00 PM", "6:00 PM"},
		{0.25, "h:mm am/pm", "6:00 am", "6:00 am"},
		{1.5, "[h]:mm:ss", "36:00:00", "36:00:00"},
		{1.0627 / 86400, "mm:ss.0", "00:01.1", "00:01,1"},
		{0.5 / 86400, "h:mm:ss", "0:00:01", "0:00:01"},
		{60, "yyyy-mm-dd", "1900-02-29", "1900-02-29"},
		{61, "yyyy-mm-dd", "1900-03-01", "1900-03-01"},
		{-1, "yyyy-mm-dd", "#######", "#######"},
		{1234.5, `#,##0.00\ "€"`, "1,234.50 €", "1.234,50 €"},
		{1234.5, `[$€-407] #,##0.00`, "€ 1,234.50", "€ 1.234,50"},
		{1234.5, `[$-407]#,##0.00`, "1,234.50", "1.234,50"},
		{-5, "0.00;[Red]0.00", "-5.00", "-5,00"},
		{-5, "0.00;(0.00)", "(5.00)", "(5,00)"},
		{-5, "0.00;[Red](0.00)", "(5.00)", "(5,00)"},
		{0, `0;-0;"zero"`, "zero", "zero"},
		{1500, `[>=1000]#,##0,"K";0`, "2K", "2K"},
		{999, `[>=1000]#,##0,"K";0`, "999", "999"},
		{1500000, `#,##0.0,,"M"`, "1.5M", "1,5M"},
		{123456789, "000-00-0000", "123-45-6789", "123-45-6789"},
		{0.5, "#.00", ".50", ",50"},
		{7, "000", "007", "007"},
		{7.1, "0.0#", "7.1", "7,1"},
		{7.125, "0.0#", "7.13", "7,13"},
		{7, "0.##", "7.", "7,"},
		{5, `"Total: "0`, "Total: 5", "Total: 5"},
		{5, `0_);\(0\)`, "5 ", "5 "},
		{12, "0 \"pcs\"", "12 pcs", "12 pcs"},
	}
	for _, c := range cases {
		if got := FormatNumber(c.v, c.code, English, false); got != c.en {
			t.Errorf("en %v %q: %q, want %q", c.v, c.code, got, c.en)
		}
		if got := FormatNumber(c.v, c.code, German, false); got != c.de {
			t.Errorf("de %v %q: %q, want %q", c.v, c.code, got, c.de)
		}
	}
}

func TestDateSystem1904(t *testing.T) {
	if got := FormatNumber(0, "yyyy-mm-dd", English, true); got != "1904-01-01" {
		t.Errorf("1904: %q", got)
	}
	if got := FormatNumber(45000-1462, "yyyy-mm-dd", English, true); got != "2023-03-15" {
		t.Errorf("1904 later: %q", got)
	}
}

func TestBuiltInFormats(t *testing.T) {
	for id, want := range map[int][2]string{14: {"3/15/2023", "15.03.2023"}, 22: {"3/15/2023 18:00", "15.03.2023 18:00"}} {
		en, _ := BuiltInCode(id, English)
		de, _ := BuiltInCode(id, German)
		if got := FormatNumber(45000.75, en, English, false); got != want[0] {
			t.Errorf("%d en: %q", id, got)
		}
		if got := FormatNumber(45000.75, de, German, false); got != want[1] {
			t.Errorf("%d de: %q", id, got)
		}
	}
	if got, _ := BuiltInCode(8, German); FormatNumber(-1234.5, got, German, false) != "-1.234,50 €" {
		t.Errorf("currency de: %q", FormatNumber(-1234.5, got, German, false))
	}
	if got, _ := BuiltInCode(7, English); FormatNumber(-1234.5, got, English, false) != "($1,234.50)" {
		t.Errorf("currency en: %q", FormatNumber(-1234.5, got, English, false))
	}
	if _, ok := BuiltInCode(99, English); ok {
		t.Error("99 is no built-in format")
	}
}

func TestTextFormats(t *testing.T) {
	for _, c := range [][3]string{
		{"abc", "@", "abc"},
		{"abc", `"Name: "@`, "Name: abc"},
		{"abc", `0;-0;0;"<"@">"`, "<abc>"},
		{"abc", "0.00", "abc"},
	} {
		if got := FormatText(c[0], c[1]); got != c[2] {
			t.Errorf("%q %q: %q", c[0], c[1], got)
		}
	}
}

func TestLangOf(t *testing.T) {
	for header, want := range map[string]string{"de-DE,de;q=0.9,en;q=0.8": "de", "de": "de", "en-US,de;q=0.5": "en", "": "en", "fr": "en", "DE-AT": "de"} {
		if got := LangOf(header).Code; got != want {
			t.Errorf("%q: %s", header, got)
		}
	}
}

// FuzzNumberFormats: no code and no number makes the formatter panic or run away.
func FuzzNumberFormats(f *testing.F) {
	for _, s := range []string{"0.00", "#,##0", "[h]:mm:ss", "# ??/??", "0.00E+00", `[$€-407] #,##0.00;[Red]-#,##0.00`, `"a";"b";"c";@`, "[>100]0;[<0]0", "\\", "\"", "[", "E+", "?/"} {
		f.Add(s, 1234.5)
		f.Add(s, -0.001)
	}
	f.Fuzz(func(t *testing.T, code string, v float64) {
		if len(code) > MaxFormatCode {
			return
		}
		out := FormatNumber(v, code, German, false)
		if len(out) > 4096 {
			t.Fatalf("%q %v: %d bytes", code, v, len(out))
		}
		_ = FormatText("x", code)
	})
}

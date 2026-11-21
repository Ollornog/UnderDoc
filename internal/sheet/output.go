// SPDX-License-Identifier: MIT

package sheet

import (
	"encoding/csv"
	"html"
	"io"
	"strconv"
	"strings"
)

// HTML writes a grid as a table of the workspace: every text escaped, numbers aligned to the right, with the first row
// as the head when head is set. caption, if any, is escaped too.
func HTML(g Grid, head bool, caption string) string {
	var b strings.Builder
	b.WriteString(`<div class="table-container overflow-x-auto"><table class="table">`)
	if caption != "" {
		b.WriteString(`<caption>` + html.EscapeString(caption) + `</caption>`)
	}
	rows := g.Rows
	if head && len(rows) > 0 {
		b.WriteString("<thead>")
		writeRow(&b, rows[0], "th")
		b.WriteString("</thead>")
		rows = rows[1:]
	}
	b.WriteString("<tbody>")
	for _, r := range rows {
		writeRow(&b, r, "td")
	}
	b.WriteString("</tbody></table></div>")
	return b.String()
}

func writeRow(b *strings.Builder, r GridRow, tag string) {
	b.WriteString("<tr>")
	for _, c := range r.Cells {
		b.WriteString("<" + tag)
		if c.RowSpan > 1 {
			b.WriteString(` rowspan="` + strconv.Itoa(c.RowSpan) + `"`)
		}
		if c.ColSpan > 1 {
			b.WriteString(` colspan="` + strconv.Itoa(c.ColSpan) + `"`)
		}
		switch c.Kind {
		case Number, ShownNumber:
			b.WriteString(` class="text-right tabular-nums"`)
		case Uncomputed:
			b.WriteString(` title="This formula was never calculated: open and save the file in Excel or LibreOffice"`)
		}
		b.WriteString(">")
		b.WriteString(strings.ReplaceAll(html.EscapeString(c.Text), "\n", "<br>"))
		b.WriteString("</" + tag + ">")
	}
	b.WriteString("</tr>")
}

// WriteCSV writes a grid as CSV. A text that a spreadsheet program would read as a formula (it starts with =, +, -, @,
// a tab or a carriage return) gets an apostrophe in front; a number is written as the plain number it holds, which never
// starts a formula. Merged cells are written once, in their first cell.
func WriteCSV(w io.Writer, wb *Workbook, s *Sheet, rect Rect) error {
	out := csv.NewWriter(w)
	hiddenRow, hiddenCol := toSet(s.HiddenRows), toSet(s.HiddenCols)
	// The rectangle is written whole, empty cells too: one far out to the right and down would be billions of them.
	rows, cols := rect.R2-rect.R1+1, rect.C2-rect.C1+1
	for _, n := range s.HiddenRows {
		if n >= rect.R1 && n <= rect.R2 {
			rows--
		}
	}
	for _, c := range s.HiddenCols {
		if c >= rect.C1 && c <= rect.C2 {
			cols--
		}
	}
	if rows*cols > MaxArea {
		return &LimitError{What: "cells in the range written out", Limit: MaxArea}
	}
	cells := map[int][]Cell{}
	for _, r := range s.Rows {
		if r.N >= rect.R1 && r.N <= rect.R2 {
			cells[r.N] = r.Cells
		}
	}
	for r := rect.R1; r <= rect.R2; r++ {
		if hiddenRow[r] {
			continue
		}
		var record []string
		for c := rect.C1; c <= rect.C2; c++ {
			if hiddenCol[c] {
				continue
			}
			field := ""
			if cell := cellAt(cells[r], c); cell != nil {
				field = csvField(wb, *cell)
			}
			record = append(record, field)
		}
		if err := out.Write(record); err != nil {
			return err
		}
	}
	out.Flush()
	return out.Error()
}

func csvField(wb *Workbook, c Cell) string {
	switch c.Kind {
	case Number:
		if f, err := strconv.ParseFloat(c.Value, 64); err == nil {
			return strconv.FormatFloat(f, 'f', -1, 64)
		}
	case Bool:
		return wb.Display(c, English)
	case Uncomputed:
		return ""
	}
	return Defuse(c.Value)
}

// Defuse puts an apostrophe before a text that a spreadsheet program would read as a formula. A plain number, such as
// -89.90, starts no formula and stays as it is.
func Defuse(text string) string {
	if text == "" || !strings.ContainsRune("=+-@\t\r", rune(text[0])) {
		return text
	}
	if _, err := strconv.ParseFloat(text, 64); err == nil && !strings.ContainsAny(text, "\t\r") {
		return text
	}
	return "'" + text
}

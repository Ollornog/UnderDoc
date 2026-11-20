#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
"""Make the spreadsheet fixtures of internal/sheet with LibreOffice headless (ADR-31).

    make.py make <out>       build the workbook and save it as budget.xlsx, budget.ods and budget.csv in <out>
    make.py show <out> <tag> load budget.xlsx and budget.ods and write what LibreOffice displays in each visible cell to
                             <out>/display-<tag>.json, in the locale of the process (LC_ALL)

Run through scripts/sheets-libreoffice.sh, never by hand on a host.
"""
import json
import os
import subprocess
import sys
import time

import uno  # noqa: F401  (python3-uno)
from com.sun.star.beans import PropertyValue
from com.sun.star.table import CellRangeAddress


def prop(name, value):
    p = PropertyValue()
    p.Name, p.Value = name, value
    return p


def connect():
    office = subprocess.Popen(["soffice", "-env:UserInstallation=file:///tmp/lo", "--headless", "--invisible", "--norestore", "--nologo", "--nodefault",
                               "--accept=socket,host=127.0.0.1,port=2002;urp;"])
    local = uno.getComponentContext()
    resolver = local.ServiceManager.createInstanceWithContext("com.sun.star.bridge.UnoUrlResolver", local)
    for _ in range(120):
        try:
            ctx = resolver.resolve("uno:socket,host=127.0.0.1,port=2002;urp;StarOffice.ComponentContext")
            return office, ctx.ServiceManager.createInstanceWithContext("com.sun.star.frame.Desktop", ctx)
        except Exception:  # LibreOffice is still starting
            time.sleep(0.5)
    raise SystemExit("LibreOffice does not answer")


def fmt(doc, code):
    """The key of a number format with an invariant (en-US) code."""
    formats = doc.NumberFormats
    locale = uno.createUnoStruct("com.sun.star.lang.Locale")
    locale.Language, locale.Country = "en", "US"
    key = formats.queryKey(code, locale, False)
    if key == -1:
        key = formats.addNew(code, locale)
    return key


# Each row: item, amount, share, date, done, the format of the amount.
ROWS = [
    ("Rent", 1234.5, 0.256, 45000, True, "#,##0.00 [$€-407]"),
    ("Power", -89.9, 0.031, 45031.75, False, "#,##0.00;[RED]-#,##0.00"),
    ("Coffee & <tea>", 1500000, 0.5, 45292, True, "#,##0.0,,\"M\""),
    ("Ümlaut ✓", 0.125, 1.5, 61, False, "# ?/?"),
]


def build(desktop, out):
    doc = desktop.loadComponentFromURL("private:factory/scalc", "_blank", 0, (prop("Hidden", True),))
    sheets = doc.Sheets
    budget = sheets.getByIndex(0)
    sheets.getByIndex(0).Name = "Budget"
    head = ["Item", "Amount", "Share", "Date", "Done"]
    for c, text in enumerate(head):
        budget.getCellByPosition(c, 0).setString(text)
    for r, (item, amount, share, date, done, amount_fmt) in enumerate(ROWS, start=1):
        budget.getCellByPosition(0, r).setString(item)
        cell = budget.getCellByPosition(1, r)
        cell.setValue(amount)
        cell.NumberFormat = fmt(doc, amount_fmt)
        cell = budget.getCellByPosition(2, r)
        cell.setValue(share)
        cell.NumberFormat = fmt(doc, "0.0%")
        cell = budget.getCellByPosition(3, r)
        cell.setValue(date)
        cell.NumberFormat = fmt(doc, "DD.MM.YYYY" if r != 2 else "YYYY-MM-DD HH:MM")
        budget.getCellByPosition(4, r).setFormula("=TRUE()" if done else "=FALSE()")
    # Formulas with their results, an error, a time over a day, a scientific number, a text that looks like a formula.
    budget.getCellByPosition(0, 5).setString("Total")
    budget.getCellByPosition(1, 5).setFormula("=SUM(B2:B5)")
    budget.getCellByPosition(1, 5).NumberFormat = fmt(doc, "#,##0.00")
    budget.getCellByPosition(2, 5).setFormula("=1/0")
    cell = budget.getCellByPosition(3, 5)
    cell.setValue(1.5)
    cell.NumberFormat = fmt(doc, "[HH]:MM")
    cell = budget.getCellByPosition(4, 5)
    cell.setValue(12345.678)
    cell.NumberFormat = fmt(doc, "0.00E+00")
    # Row 7 is hidden, column G is hidden.
    budget.getCellByPosition(0, 6).setString("hidden row")
    budget.Rows.getByIndex(6).IsVisible = False
    budget.getCellByPosition(6, 1).setString("hidden column")
    budget.Columns.getByIndex(6).IsVisible = False
    # A merged area A9:C9, text after it in F9 so the used range reaches beyond.
    budget.getCellByPosition(0, 8).setString("Merged over three")
    budget.getCellRangeByName("A9:C9").merge(True)
    budget.getCellByPosition(5, 8).setString("=cmd|' /C calc'!A0")
    budget.getCellByPosition(5, 9).setString("@SUM(1+1)")
    budget.getCellByPosition(5, 10).setString("-2+3")
    budget.getCellByPosition(5, 11).setString("<script>alert(1)</script>")
    # Names: a range of amounts, a single cell as a value (VAT), and a database range as a table.
    cell = budget.getCellByPosition(7, 0)
    cell.setValue(0.19)
    cell.NumberFormat = fmt(doc, "0%")
    names = doc.NamedRanges
    origin = uno.createUnoStruct("com.sun.star.table.CellAddress")
    names.addNewByName("Amounts", "$Budget.$B$2:$B$5", origin, 0)
    names.addNewByName("VAT", "$Budget.$H$1", origin, 0)
    area = CellRangeAddress()
    area.Sheet, area.StartColumn, area.StartRow, area.EndColumn, area.EndRow = 0, 0, 0, 4, 4
    doc.DatabaseRanges.addNewByName("Expenses", area)
    doc.DatabaseRanges.getByName("Expenses").ContainsHeader = True
    doc.DatabaseRanges.getByName("Expenses").AutoFilter = True
    # A hidden sheet, and a sheet whose name needs escaping.
    sheets.insertNewByName("Secret", 1)
    sheets.getByName("Secret").getCellByPosition(0, 0).setString("top secret")
    sheets.getByName("Secret").IsVisible = False
    sheets.insertNewByName("Odd <&> it's", 2)
    odd = sheets.getByName("Odd <&> it's")
    odd.getCellByPosition(0, 0).setString("Name")
    odd.getCellByPosition(1, 0).setString("Value")
    odd.getCellByPosition(0, 1).setString("Längerer Text mit Umlauten äöüß")
    odd.getCellByPosition(1, 1).setValue(42)
    for ext, filt, opts in ((".xlsx", "Calc MS Excel 2007 XML", None), (".ods", "calc8", None),
                            (".csv", "Text - txt - csv (StarCalc)", "59,34,76,1")):
        args = [prop("FilterName", filt)]
        if opts:
            args.append(prop("FilterOptions", opts))
        doc.storeToURL(uno.systemPathToFileUrl(os.path.join(out, "budget" + ext)), tuple(args))
    doc.close(True)


def show(desktop, out, tag):
    shown = {}
    for name in ("budget.xlsx", "budget.ods"):
        doc = desktop.loadComponentFromURL(uno.systemPathToFileUrl(os.path.join(out, name)), "_blank", 0,
                                           (prop("Hidden", True),))
        sheets = {}
        for i in range(doc.Sheets.Count):
            sh = doc.Sheets.getByIndex(i)
            if not sh.IsVisible:
                continue
            cursor = sh.createCursor()
            cursor.gotoEndOfUsedArea(False)
            cells = {}
            for r in range(cursor.RangeAddress.EndRow + 1):
                for c in range(cursor.RangeAddress.EndColumn + 1):
                    cell = sh.getCellByPosition(c, r)
                    if cell.getString() != "":
                        cells[col_name(c + 1) + str(r + 1)] = cell.getString()
            sheets[sh.Name] = cells
        shown[name] = sheets
        doc.close(True)
    with open(os.path.join(out, "display-" + tag + ".json"), "w", encoding="utf-8") as f:
        json.dump(shown, f, ensure_ascii=False, indent=1, sort_keys=True)
        f.write("\n")


def col_name(c):
    s = ""
    while c > 0:
        c, rest = divmod(c - 1, 26)
        s = chr(65 + rest) + s
    return s


def main():
    office, desktop = connect()
    try:
        if sys.argv[1] == "make":
            build(desktop, sys.argv[2])
        else:
            show(desktop, sys.argv[2], sys.argv[3])
    finally:
        try:
            desktop.terminate()
        except Exception:  # LibreOffice is gone already
            pass
        office.wait(timeout=60)


if __name__ == "__main__":
    main()

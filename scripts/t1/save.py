#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
"""Open a file on a share of the Samba companion in LibreOffice, change it and save it, as a person would (T-1).

    save.py [--make] //server/share file [file ...]

The share is mounted with the kernel's SMB client (SMB 3.1.1) as SMB_USER with SMB_PASS from the environment. Each file
is loaded, its first cell (Calc) or its text (Writer) changed, and saved with Save, in the format it has. With --make,
each file is made new with Save As first, in the format its extension names, so the save over an existing file is what
the next call measures.
"""
import os
import subprocess
import sys
import time

import uno  # noqa: F401  (python3-uno)
from com.sun.star.beans import PropertyValue

MOUNT = "/mnt/share"


def prop(name, value):
    p = PropertyValue()
    p.Name, p.Value = name, value
    return p


def connect():
    office = subprocess.Popen(["soffice", "--headless", "--invisible", "--norestore", "--nologo", "--nodefault",
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


def change(doc, text):
    if doc.supportsService("com.sun.star.sheet.SpreadsheetDocument"):
        doc.Sheets.getByIndex(0).getCellByPosition(0, 0).setString(text)
    else:
        doc.Text.setString(text)


FILTERS = {".xlsx": ("scalc", "Calc MS Excel 2007 XML"), ".ods": ("scalc", "calc8"),
           ".docx": ("swriter", "MS Word 2007 XML"), ".odt": ("swriter", "writer8")}


def main():
    args = sys.argv[1:]
    make = args[:1] == ["--make"]
    if make:
        args = args[1:]
    share, names = args[0], args[1:]
    os.makedirs(MOUNT, exist_ok=True)
    subprocess.run(["mount", "-t", "cifs", share, MOUNT, "-o",
                    "username=%s,password=%s,vers=3.1.1,noperm,cache=strict,actimeo=0" % (os.environ["SMB_USER"], os.environ["SMB_PASS"])],
                   check=True)
    office, desktop = connect()
    try:
        for name in names:
            url = uno.systemPathToFileUrl(os.path.join(MOUNT, name))
            hidden = (prop("Hidden", True),)
            if make:
                kind, filt = FILTERS[os.path.splitext(name)[1]]
                doc = desktop.loadComponentFromURL("private:factory/" + kind, "_blank", 0, hidden)
                change(doc, "made by LibreOffice")
                doc.storeAsURL(url, (prop("FilterName", filt),))
                doc.close(True)
                print("made", name, flush=True)
                continue
            doc = desktop.loadComponentFromURL(url, "_blank", 0, hidden)
            change(doc, "saved by LibreOffice at %s" % time.strftime("%H:%M:%S"))
            doc.store()
            doc.close(True)
            print("saved", name, flush=True)
    finally:
        try:
            desktop.terminate()
        except Exception:
            pass
        office.wait(timeout=60)
        subprocess.run(["umount", MOUNT], check=False)


if __name__ == "__main__":
    main()

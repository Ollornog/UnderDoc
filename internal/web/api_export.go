// SPDX-License-Identifier: MIT

package web

import (
	"archive/zip"
	"bytes"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/transfer"
)

// apiExport answers a collection, folder or page as a zip of normal files, written as `underdoc export` writes them,
// with only what the viewer sees (docs/export.md). The zip is made in memory first, so a failure is still an error.
func (h *handler) apiExport(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return
	}
	user, _ := account.From(r.Context())
	tree, err := h.options.DB.Subtree(r.Context(), r.PathValue("id"), user.ID)
	if err == nil && db.IsDivider(tree.Kind) {
		err = db.ErrNotFound
	}
	if h.failed(w, err, "export") {
		return
	}
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	now := time.Now()
	_, err = transfer.Write(transfer.Tree{Shared: []*db.Node{tree}}, func(name string, data []byte) error {
		f, err := archive.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: now})
		if err != nil {
			return err
		}
		_, err = f.Write(data)
		return err
	})
	if err == nil {
		err = archive.Close()
	}
	if h.failed(w, err, "export") {
		return
	}
	name := strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, tree.Title) + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	_, _ = w.Write(buf.Bytes())
}

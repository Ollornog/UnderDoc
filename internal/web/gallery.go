// SPDX-License-Identifier: MIT

package web

import (
	"html/template"
	"net/http"

	"github.com/Ollornog/UnderDoc/internal/library"
)

type galleryView struct {
	shellView
	Broken       template.HTML
	Results      []searchResult
	Problems     []library.Problem
	BacklinkList []linkItem
	Folders      []linkItem
	Documents    []linkItem
	Contents     []contentsItem
}

// gallery shows the components of this application in every state, so a redesign can work against one page.
func (h *handler) gallery(w http.ResponseWriter, _ *http.Request) {
	view := galleryView{
		shellView: shellView{
			Title:        "Components",
			AssetVersion: h.assetVersion,
			ProblemCount: 2,
			Crumbs:       []crumb{{Label: "Home", Href: "#"}, {Label: "Folder", Href: "#"}},
			Tree: []treeItem{
				{Label: "Open folder", IsFolder: true, Open: true, Href: "#", Children: []treeItem{
					{Label: "Current page", Href: "#", Current: true},
					{Label: "Nested folder", IsFolder: true, Href: "#", Children: []treeItem{{Label: "Hidden page", Href: "#"}}},
				}},
				{Label: "Page with a very long title that does not fit into the sidebar at all", Href: "#"},
			},
		},
		Broken: `<span class="badge" data-variant="destructive">Missing page: no such page</span>`,
		Results: []searchResult{{
			linkItem: linkItem{Label: "Network plan", Href: "#", Detail: "operations/network.md"},
			Snippet:  []library.Fragment{{Text: "… the "}, {Text: "router", Match: true}, {Text: " in the tower …"}},
		}},
		Problems:     []library.Problem{{Source: "notes/a.md", Message: "[[Missing]]: no such page"}},
		BacklinkList: []linkItem{{Label: "Other page", Href: "#", Detail: "notes/other.md"}},
		Folders:      []linkItem{{Label: "Subfolder", Href: "#"}},
		Documents:    []linkItem{{Label: "A page", Href: "#", Detail: "2026-11-06 12:00"}},
		Contents:     []contentsItem{{Label: "Main heading", Href: "#", Level: 2}, {Label: "Subheading", Href: "#", Level: 3}},
	}
	h.render(w, "gallery", http.StatusOK, view)
}

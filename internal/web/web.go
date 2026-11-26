// SPDX-License-Identifier: MIT

// Package web serves the workspace and the API it reads. Everyone who gets past the sign-in gate in front of it can read
// what is not private.
package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"sync"

	"github.com/Ollornog/UnderDoc/docs"
	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/auth"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/document"
	"github.com/Ollornog/UnderDoc/internal/files"
	"github.com/Ollornog/UnderDoc/internal/id"
	"github.com/Ollornog/UnderDoc/internal/library"
	"github.com/Ollornog/UnderDoc/internal/module"
	"github.com/Ollornog/UnderDoc/internal/route"
	"github.com/Ollornog/UnderDoc/internal/search"
	"github.com/Ollornog/UnderDoc/internal/share"
	"github.com/Ollornog/UnderDoc/internal/sheet"
	"github.com/Ollornog/UnderDoc/internal/store"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/vault"
)

//go:embed templates/*.html
var templateFiles embed.FS

//go:embed all:assets
var assetFiles embed.FS

const (
	maxSearchResults = 50
	// homeLabel names the root of the folder source in the workspace.
	homeLabel = "Home"
)

var imageTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".avif": "image/avif",
	".svg":  "image/svg+xml",
}

// Options configures the handler.
type Options struct {
	// DevUI turns on /dev/ui, the workspace with only the made-up content of the mockup, every component in use.
	DevUI bool
	// DevUser stands in for the gate when the application runs on its own, for development only.
	DevUser *account.User
	// GateClient checks what the sign-in gate passes on (ADR-32), the only way in; its role of the app makes an
	// administrator. Nil is the gate in header mode with its default names.
	GateClient *account.Gate
	// Vault seals secrets such as the passwords in the text and the app passwords of Samba. Without a key, these are not
	// offered.
	Vault *vault.Vault
	// Modules is what the instance allows of the modules and has on by default (UNDERDOC_MODULES); the zero value
	// allows every module with its own default.
	Modules module.Policy
	// HosterHost is the host name of the hoster administration, with many tenants (ADR-23).
	HosterHost string
	// DB keeps accounts and the objects made in the web (ADR-19). Without it, the folder source is all there is.
	DB *db.DB
	// Tenants is the mode of the instance (ADR-23): tenant.ModeOne, the default, or tenant.ModeMany, which needs DB.
	Tenants string
	// TenantDomain is the domain of the hoster in the mode many tenants: acme.<domain> leads to the tenant acme.
	TenantDomain string
	// Store is the content store of the files (ADR-28). Without it, or without DB, there are no files.
	Store *store.Store
	// Sheets reads spreadsheets of the store as tables and keeps them (ADR-31); nil makes one over Store.
	Sheets *sheet.Cache
	// Office converts Office files to PDF for their preview, an optional module (docs/files.md); nil for none.
	Office files.OfficeConverter
	// TextWays are the ways of reading the text of files the hoster allows (ADR-24); the tenant and each user narrow them.
	TextWays db.Ways
	// OCR reads the text of images on the server, as part of its way, and AIOCR through a model (M-13); nil for none.
	OCR, AIOCR files.OCR
	// Search answers /api/search when there is a database (ADR-24). Without it, the folder is searched in memory.
	Search *search.Service
	// Share keeps the working copy of the libraries shared over Samba (ADR-30); nil without a companion.
	Share *share.Manager
	// Gate is what UnderDoc may do in the gate itself (T-33): accounts through its account API and approvals for
	// deleting for good. Nil offers neither.
	Gate *GateAPI
}

type handler struct {
	store        *library.Store
	log          *slog.Logger
	pages        *template.Template
	assetVersion string
	options      Options
	// bound keeps the account each lasting ID of the gate was bound to, so not every request writes it.
	boundMu sync.Mutex
	bound   map[string]boundAccount
	// noFolder is the empty library that tenants other than Main see in place of the folder source.
	noFolder *library.Library
	// uploads are the uploads a request is writing right now.
	uploads uploadBusy
	// throttle is the limit against guessing the passwords of share links.
	throttle *auth.Throttle
	// reveals is the limit on revealing passwords of the text (ADR-7).
	reveals *auth.Rate
}

// New returns the complete handler with logging and security headers.
func New(store *library.Store, log *slog.Logger, options Options) (http.Handler, error) {
	if options.DB != nil && options.Search == nil {
		options.Search = search.NewService(nil, TitleFallback(options.DB, store), tenant.Main, log)
	}
	h := &handler{store: store, log: log, options: options, bound: map[string]boundAccount{}, noFolder: library.Build(library.Input{}),
		throttle: auth.NewThrottle(), reveals: auth.NewRate(db.RevealsPerWindow, db.RevealWindow)}
	if h.options.Modules.IsZero() {
		h.options.Modules, _ = module.ParsePolicy("")
	}
	if h.options.Sheets == nil && h.options.Store != nil {
		h.options.Sheets = sheet.NewCache(h.options.Store, log)
	}
	if h.options.GateClient == nil {
		gate, err := account.NewGate(account.GateConfig{})
		if err != nil {
			return nil, err
		}
		h.options.GateClient = gate
	}
	switch options.Tenants {
	case "":
		h.options.Tenants = tenant.ModeOne
	case tenant.ModeOne:
	case tenant.ModeMany:
		if options.DB == nil {
			return nil, errors.New("many tenants need the database")
		}
	default:
		return nil, fmt.Errorf("unknown mode of tenants %q: %s or %s", options.Tenants, tenant.ModeOne, tenant.ModeMany)
	}
	if h.approvalsOn() {
		approver, err := newApprover(h.options.Gate, h.options.DB)
		if err != nil {
			return nil, err
		}
		h.options.DB.SetApprover(approver)
	}
	var err error
	funcs := template.FuncMap{
		// meterOf hands the template "meter" a use against its limit and the label of its bar.
		"meterOf": func(m *meter, label string) map[string]any { return map[string]any{"M": m, "Label": label} },
	}
	if h.pages, err = template.New("").Funcs(funcs).ParseFS(templateFiles, "templates/*.html"); err != nil {
		return nil, err
	}
	if h.assetVersion, err = hashAssets(); err != nil {
		return nil, err
	}
	assets, err := fs.Sub(assetFiles, "assets")
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	// The workspace renders itself in the browser, as the approved mockup does, and reads its data from the API (ADR-16).
	// Everything of the module Documents answers only while it is on for the tenant (ADR-25).
	doc := func(pattern string, f http.HandlerFunc) { mux.HandleFunc(pattern, h.guard(module.Documents, f)) }
	fil := func(pattern string, f http.HandlerFunc) { mux.HandleFunc(pattern, h.guard(module.Files, f)) }
	for _, pattern := range []string{"GET /{$}", "GET /profile"} {
		mux.HandleFunc(pattern, h.app)
	}
	for _, pattern := range []string{"GET /o/{id...}", "GET /f/{path...}"} {
		doc(pattern, h.app)
	}
	mux.HandleFunc("GET /api/state", h.apiState)
	doc("GET /api/doc/{id...}", h.apiDocument)
	doc("GET /api/search", h.apiSearch)
	// The MCP server for agents (ADR-34). Its transport takes POST; it answers GET and DELETE itself, that it does not.
	mcpServer := h.mcpHandler().ServeHTTP
	for _, method := range []string{"POST ", "GET ", "DELETE "} {
		doc(method+MCPPath, mcpServer)
	}
	doc("POST /api/collections", h.apiCreateCollection)
	doc("PATCH /api/objects/{id}", h.apiRename)
	doc("PUT /api/objects/{id}/tags", h.apiTags)
	doc("PUT /api/objects/{id}/view", h.apiView)
	doc("PUT /api/objects/{id}/template", h.apiTemplate)
	doc("GET /api/templates", h.apiTemplates)
	doc("GET /api/objects/{id}/uses", h.apiUses)
	doc("POST /api/pages", h.apiCreatePage)
	doc("POST /api/notes", h.apiAddNote)
	doc("POST /api/folders", h.apiCreateFolder)
	mux.HandleFunc("PUT /api/settings", h.apiSettings)
	doc("POST /api/objects/{id}/access", h.apiAccess)
	// The rights of ADR-29 and ADR-36 hold for documents and files alike.
	mux.HandleFunc("GET /api/objects/{id}/rights", h.apiRights)
	mux.HandleFunc("PUT /api/objects/{id}/grants", h.apiSetGrant)
	mux.HandleFunc("PUT /api/objects/{id}/inherit", h.apiSetInherit)
	mux.HandleFunc("PUT /api/objects/{id}/owner", h.apiSetOwner)
	// The archive of spaces (T-34), of the documents and of Files alike.
	mux.HandleFunc("PUT /api/objects/{id}/archive", h.apiSetArchived)
	// The mark of agents' writes after reading narrower content, per space (T-38).
	mux.HandleFunc("PUT /api/objects/{id}/agent-mark", h.apiSetAgentMark)
	mux.HandleFunc("GET /api/people", h.apiPeople)
	mux.HandleFunc("GET /api/groups", h.apiGroups)
	mux.HandleFunc("POST /api/groups", h.apiCreateGroup)
	mux.HandleFunc("PATCH /api/groups/{id}", h.apiChangeGroup)
	mux.HandleFunc("DELETE /api/groups/{id}", h.apiArchiveGroup)
	mux.HandleFunc("POST /api/groups/{id}/restore", h.apiRestoreGroup)
	mux.HandleFunc("GET /api/groups/{id}/members", h.apiGroupMembers)
	mux.HandleFunc("PUT /api/groups/{id}/members/{account}", h.apiSetMember)
	// Nested groups, group admins and what each person lets their agents do (ADR-36).
	mux.HandleFunc("GET /api/groups/{id}", h.apiGroup)
	mux.HandleFunc("PUT /api/groups/{id}/groups/{group}", h.apiSetSubgroup)
	mux.HandleFunc("PUT /api/groups/{id}/admins/{account}", h.apiSetGroupAdmin)
	mux.HandleFunc("GET /api/me/agents", h.apiAgentLimit)
	// The rules page for agents (B-5): named by an administrator in the browser.
	doc("GET /api/rules-page", h.apiRulesPage)
	doc("PUT /api/rules-page", h.apiSetRulesPage)
	mux.HandleFunc("PUT /api/me/agents", h.apiSetAgentLimit)
	mux.HandleFunc("PUT /api/me/agent-reads", h.apiSetAgentReadHours)
	mux.HandleFunc("GET /api/rights/check", h.apiCheck)
	mux.HandleFunc("GET /api/rights/held", h.apiHeld)
	mux.HandleFunc("GET /api/users", h.apiUsers)
	// Accounts through the gate's account API and approvals of the gate (T-33); they answer only where configured.
	mux.HandleFunc("GET /api/accounts/{id}", h.apiAccountInfo)
	mux.HandleFunc("POST /api/groups/{id}/invite", h.apiInvite)
	mux.HandleFunc("POST /api/accounts/{id}/signin-link", h.apiSigninLink)
	mux.HandleFunc("PUT /api/accounts/{id}/lock", h.apiSetLocked)
	mux.HandleFunc("POST /api/accounts/{id}/reset", h.apiReset)
	mux.HandleFunc("POST /api/approvals/{approval}/wait", h.apiWaitApproval)
	mux.HandleFunc("GET "+approvalReturn, h.approvalBack)
	mux.HandleFunc("PUT /api/users/{id}/admin", h.apiSetAdmin)
	mux.HandleFunc("GET /api/log", h.apiLog)
	mux.HandleFunc("GET /api/log/keep", h.apiLogKeep)
	mux.HandleFunc("PUT /api/log/keep", h.apiSetLogKeep)
	// Share links (ADR-29): made and listed in the workspace, opened at /s/<token> without signing in.
	mux.HandleFunc("GET /api/objects/{id}/shareables", h.apiShareables)
	mux.HandleFunc("POST /api/objects/{id}/links", h.apiCreateLink)
	mux.HandleFunc("GET /api/links", h.apiLinks)
	mux.HandleFunc("DELETE /api/links/{id}", h.apiRevokeLink)
	mux.HandleFunc("POST /api/links/{id}/renew", h.apiRenewLink)
	mux.HandleFunc("GET /s/{token}", h.share)
	mux.HandleFunc("GET /link/{token}", h.takeLink)
	mux.HandleFunc("POST /s/{token}/password", h.sharePassword)
	mux.HandleFunc("GET /s/{token}/f/{file}", h.shareFile)
	mux.HandleFunc("GET /s/{token}/p/{page}/attachments/{attachment}", h.shareAttachment)
	mux.HandleFunc("POST /s/{token}/upload", h.shareUpload)
	// One-time upload links of agents (ADR-34): open at the gate like share links, the token says what it may do.
	mux.HandleFunc("PUT "+uploadLinkPath+"{token}", h.uploadThroughOneTimeLink)
	doc("POST /api/objects/{id}/kind", h.apiConvert)
	doc("POST /api/pages/{id}/blocks", h.apiAddBlock)
	doc("PUT /api/pages/{id}/blocks/{block}", h.apiEditBlock)
	doc("DELETE /api/pages/{id}/blocks/{block}", h.apiDeleteBlock)
	doc("POST /api/pages/{id}/blocks/{block}/move", h.apiMoveBlock)
	doc("PUT /api/pages/{id}/blocks/{block}/align", h.apiAlignBlock)
	doc("POST /api/pages/{id}/blocks/{block}/parts", h.apiMarkPart)
	doc("POST /api/pages/{id}/blocks/{block}/excerpt", h.apiExcerpt)
	doc("POST /api/pages/{id}/blocks/{block}/lost", h.apiLost)
	doc("POST /api/removed/{id}/restore", h.apiRestoreRemoved)
	doc("POST /api/removed/{id}/copy", h.apiKeepCopy)
	doc("GET /api/redirect", h.apiRedirect)
	doc("GET /api/notices", h.apiNotices)
	doc("GET /api/values", h.apiValues)
	doc("GET /api/snippets", h.apiSnippets)
	doc("GET /api/pages/{id}/versions", h.apiVersions)
	// Page kinds and their writing rules (T-20, ADR-33); the raw page with its front matter for agents.
	doc("PUT /api/pages/{id}/kind", h.apiPageKind)
	doc("GET /api/pages/{id}/raw", h.apiRawPage)
	// Changes by agents on human documentation: the diff, checking it and taking it back, one block at a time (ADR-33).
	doc("GET /api/pages/{id}/blocks/{block}/change", h.apiChange)
	doc("POST /api/pages/{id}/blocks/{block}/check", h.apiCheckChange)
	doc("POST /api/pages/{id}/blocks/{block}/takeback", h.apiTakeBack)
	// Passwords in the text (ADR-7): the value only through reveal.
	doc("GET /api/pages/{id}/secrets", h.apiPageSecrets)
	doc("POST /api/pages/{id}/secrets", h.apiCreateSecret)
	doc("GET /api/secrets/{id}", h.apiSecret)
	doc("PUT /api/secrets/{id}", h.apiChangeSecret)
	doc("DELETE /api/secrets/{id}", h.apiDeleteSecret)
	doc("POST /api/secrets/{id}/reveal", h.apiRevealSecret)
	doc("PUT /api/secrets/{id}/readers", h.apiSecretReaders)
	doc("POST /api/secrets/{id}/versions/{n}/restore", h.apiRestoreSecret)
	doc("GET /api/pages/{id}/comments", h.apiComments)
	doc("POST /api/pages/{id}/comments", h.apiAddComment)
	doc("PATCH /api/comments/{id}", h.apiChangeComment)
	doc("DELETE /api/comments/{id}", h.apiDeleteComment)
	doc("POST /api/dividers", h.apiCreateDivider)
	doc("DELETE /api/objects/{id}", h.apiDeleteObject)
	doc("POST /api/objects/{id}/move", h.apiMove)
	doc("POST /api/objects/{id}/copy", h.apiCopy)
	doc("GET /api/objects/{id}/export", h.apiExport)
	doc("GET /api/trash", h.apiTrash)
	doc("GET /api/problems", h.apiProblems)
	doc("POST /api/trash/{id}/restore", h.apiRestore)
	doc("DELETE /api/trash/{id}", h.apiPurge)
	doc("DELETE /api/trash", h.apiEmptyTrash)
	// The view Files (ADR-28).
	fil("GET /api/files", h.apiFileRoots)
	fil("POST /api/files/collections", h.apiCreateFileCollection)
	fil("PUT /api/files/roots/{id}/quota", h.apiSetQuota)
	fil("PUT /api/files/writers", h.apiSetWriters)
	fil("GET /api/files/{id}/list", h.apiFileList)
	fil("GET /api/files/{id}", h.apiFile)
	fil("GET /api/files/{id}/content", h.apiFileContent)
	fil("GET /api/files/{id}/sheets", h.apiSheets)
	fil("GET /api/files/{id}/sheets/grid", h.apiSheetGrid)
	fil("GET /api/files/{id}/sheets/csv", h.apiSheetCSV)
	fil("GET /api/files/{id}/tables", h.apiFileTables)
	fil("POST /api/files/{id}/tables", h.apiChooseTable)
	fil("POST /api/files/{id}/tables/{table}/keep", h.apiKeepTable)
	fil("GET /api/files/{id}/thumb", h.apiFileThumb)
	fil("GET /api/files/{id}/text", h.apiFileText)
	fil("PUT /api/files/{id}/text", h.apiPutFileText)
	fil("GET /api/files/text-settings", h.apiTextSettings)
	fil("PUT /api/files/text-settings", h.apiSetTextSettings)
	fil("GET /api/files/{id}/pdf", h.apiFilePDF)
	doc("GET /api/pages/{id}/attachments/{attachment}/thumb", h.apiAttachmentThumb)
	fil("POST /api/files/{id}/versions/{n}/restore", h.apiRestoreFileVersion)
	fil("POST /api/uploads", h.apiStartUpload)
	fil("GET /api/uploads/{id}", h.apiUploadState)
	fil("PATCH /api/uploads/{id}", h.apiUploadChunk)
	fil("POST /api/uploads/{id}/finish", h.apiFinishUpload)
	fil("DELETE /api/uploads/{id}", h.apiCancelUpload)
	doc("GET /api/pages/{id}/attachments", h.apiAttachments)
	doc("GET /api/pages/{id}/attachments/{attachment}", h.apiAttachmentContent)
	doc("POST /api/pages/{id}/attachments/{attachment}/move", h.apiMoveAttachment)
	// Samba (ADR-30): the app password in the profile, the share of a collection, the Samba users for administrators.
	mux.HandleFunc("GET /api/samba", h.apiSamba)
	mux.HandleFunc("POST /api/samba/password", h.apiNewAppPassword)
	mux.HandleFunc("DELETE /api/samba/password", h.apiRemoveAppPassword)
	mux.HandleFunc("POST /api/samba/domain", h.apiSambaDomain)
	mux.HandleFunc("GET /api/samba/accounts", h.apiSambaAccounts)
	mux.HandleFunc("DELETE /api/samba/accounts/{id}", h.apiRemoveAppPassword)
	fil("PUT /api/files/roots/{id}/samba", h.apiSetRootSamba)
	fil("GET /api/spaces/{id}/files", h.apiSpaceFiles)
	fil("PUT /api/spaces/{id}/files", h.apiLinkSpaceFiles)
	// Links from before the object addresses of ADR-18 keep working.
	doc("GET /d/{id...}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, route.Object(r.PathValue("id")), http.StatusMovedPermanently)
	})
	doc("GET /files/{path...}", h.file)
	// Modules (ADR-25) and the hoster administration (ADR-23).
	mux.HandleFunc("GET /admin/modules", h.adminModules)
	mux.HandleFunc("POST /admin/modules", h.saveModules)
	mux.HandleFunc("GET /account/modules", h.myModules)
	mux.HandleFunc("POST /account/modules", h.saveMyModules)
	mux.HandleFunc("GET /hoster", h.hoster)
	mux.HandleFunc("POST /hoster/tenants", h.createTenant)
	mux.HandleFunc("GET /hoster/tenants/{id}", h.hosterTenant)
	mux.HandleFunc("POST /hoster/tenants/{id}", h.changeTenant)
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", staticAssets(h.assetVersion, http.FileServerFS(assets))))
	mux.HandleFunc("GET /assets/logo.svg", serveBytes("image/svg+xml", docs.Logo))
	mux.HandleFunc("GET "+mermaidFrame, h.mermaidFrame)
	mux.HandleFunc("GET /favicon.ico", serveBytes("image/vnd.microsoft.icon", docs.Favicon))
	if options.DevUI {
		mux.HandleFunc("GET /dev/ui", h.app)
	}
	// The time tracking (EightPerDay) answers below /time while its module is on (docs/time-tracking.md).
	if options.DB != nil {
		timeTracking, err := h.timeTracking()
		if err != nil {
			return nil, err
		}
		for _, pattern := range []string{"GET " + timeBase, "GET " + timeBase + "/", "POST " + timeBase + "/"} {
			mux.Handle(pattern, timeTracking)
		}
		mux.HandleFunc("GET /api/time/absences", h.guard(module.Time, h.apiTimeAbsences))
	}
	mux.HandleFunc("GET /", h.notFound)
	return accessLog(log, secure(h.tenanted(h.signedIn(h.logged(mux))))), nil
}

// open reports whether a request needs neither tenant nor sign-in: the assets, so a page that says why nothing else
// answers still has its styles.
func open(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/assets/") || r.URL.Path == "/favicon.ico"
}

// tenanted finds the tenant of a request and carries it in the context, where the database takes it from (ADR-23). With
// one tenant every request belongs to Main. With many, the host name decides; an unknown host finds nothing, and a
// locked tenant lets nobody in.
func (h *handler) tenanted(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if open(r) {
			next.ServeHTTP(w, r)
			return
		}
		if h.options.Tenants != tenant.ModeMany {
			next.ServeHTTP(w, r.WithContext(tenant.With(r.Context(), tenant.Main)))
			return
		}
		if h.options.HosterHost != "" && tenant.Host(r.Host) == tenant.Host(h.options.HosterHost) {
			// The hoster administration answers on its own host, and nothing of a workspace does.
			switch {
			case r.URL.Path == "/":
				http.Redirect(w, r, "/hoster", http.StatusSeeOther)
			case hosterPath(r):
				next.ServeHTTP(w, r.WithContext(tenant.With(r.Context(), tenant.Hoster)))
			default:
				h.notFound(w, r)
			}
			return
		}
		t, err := h.options.DB.TenantByHost(r.Context(), r.Host, h.options.TenantDomain)
		switch {
		case errors.Is(err, db.ErrNotFound):
			h.message(w, http.StatusNotFound, message{
				Title: "Nothing here",
				Text:  "No workspace answers at this address. Check the address you were given.",
				Link:  "Try again",
			})
			return
		case err != nil:
			h.log.Error("tenant not found", slog.String("error", err.Error()))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		case t.State == db.TenantLocked:
			h.message(w, http.StatusForbidden, message{
				Title: "Locked",
				Text:  "This workspace is locked for now. Nothing in it is lost. Ask whoever runs it.",
				Link:  "Try again",
			})
			return
		}
		next.ServeHTTP(w, r.WithContext(tenant.With(r.Context(), t.ID)))
	})
}

// library returns the folder source as the tenant of the request sees it: whole for Main, and empty for every other
// tenant, since the folder belongs to the instance. The folder is for everyone signed in, which a service account never
// is (B-3): it sees it empty.
func (h *handler) library(ctx context.Context) *library.Library {
	if u, ok := account.From(ctx); ok && u.Kind == account.KindService {
		return h.noFolder
	}
	if t, _ := tenant.From(ctx); t == tenant.Main {
		return h.store.Library()
	}
	return h.noFolder
}

// signedIn lets a request through only with someone the gate signed in (ADR-32). Assets and share links stay open; a
// request without the gate gets the page that says so.
func (h *handler) signedIn(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if open(r) || public(r) {
			next.ServeHTTP(w, r)
			return
		}
		user, err := h.who(r)
		switch {
		case errors.Is(err, db.ErrQuota):
			h.message(w, http.StatusForbidden, message{Title: "Workspace full", Text: quotaText, Link: "Try again"})
			return
		case errors.Is(err, errNotSignedIn):
			h.notSignedIn(w)
			return
		case errors.Is(err, errRefused), errors.Is(err, account.ErrService), errors.Is(err, errLocked):
			h.refused(w, err)
			return
		case err != nil:
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(w, r.WithContext(account.With(r.Context(), user)))
	})
}

// sameOrigin lets a change through only from a page of this application. The gate signs in with a cookie, so a page on
// another site could otherwise send changes here in someone's name.
func sameOrigin(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		return site == "same-origin"
	}
	origin := r.Header.Get("Origin")
	return origin != "" && strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://") == r.Host
}

func hashAssets() (string, error) {
	hash := sha256.New()
	err := fs.WalkDir(assetFiles, "assets", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, err := assetFiles.ReadFile(p)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(hash, "%s %d\n", p, len(content))
		_, _ = hash.Write(content)
		return nil
	})
	return hex.EncodeToString(hash.Sum(nil))[:12], err
}

// staticAssets lets the browser keep an asset for a year when its URL names the version of this release (?v=…): the URL
// changes with every release, so nothing stale is kept, and the editor, once loaded, comes from the cache (2026-11-09).
// Without the current version a day.
//
// The worker of Tesseract.js is the one script that may compile WebAssembly (ocrWorkerCSP): a dedicated worker takes the
// policy of its own script, so the pages keep theirs. The script of the frame of the diagrams may be loaded from a
// document without an origin.
func staticAssets(version string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == ocrWorker {
			w.Header().Set("Content-Security-Policy", ocrWorkerCSP)
		}
		// The frame of the diagrams has no origin of its own (T-3), so its script is fetched from no origin, which
		// same-origin forbids. It holds nothing but Mermaid, which anyone may read.
		if r.URL.Path == mermaidScript {
			w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
		}
		if version != "" && r.URL.Query().Get("v") == version {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=86400")
		}
		next.ServeHTTP(w, r)
	})
}

// mermaidFrame answers the page of the frame Mermaid draws in (T-3), with the policy of its own and the script of this
// release. Only this page may be framed, and only by UnderDoc itself.
func (h *handler) mermaidFrame(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Security-Policy", mermaidFrameCSP)
	w.Header().Del("X-Frame-Options")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = fmt.Fprintf(w, "<!doctype html>\n<html lang=\"en\"><head><meta charset=\"utf-8\"><title>Diagrams</title>"+
		"<script src=\"/assets/app/mermaid/frame.js?v=%s\"></script></head><body></body></html>\n", template.HTMLEscapeString(h.assetVersion))
}

func serveBytes(contentType string, content []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=86400")
		if contentType == "image/svg+xml" {
			w.Header().Set("Content-Security-Policy", imageCSP)
		}
		_, _ = w.Write(content)
	}
}

// appView fills the page of the workspace. Demo shows only the made-up content of the mockup, for /dev/ui.
type appView struct {
	Version string
	Demo    bool
}

// app answers every address of the workspace with the same page; the browser shows the object the address names.
func (h *handler) app(w http.ResponseWriter, r *http.Request) {
	if !h.exists(r) {
		h.notFound(w, r)
		return
	}
	var out bytes.Buffer
	if err := h.pages.ExecuteTemplate(&out, "app", appView{Version: h.assetVersion, Demo: r.URL.Path == "/dev/ui"}); err != nil {
		h.log.Error("template failed", slog.String("template", "app"), slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(out.Bytes())
}

// exists tells whether the address of the workspace names something the viewer may see, so an unknown address is a 404
// and not an empty workspace.
func (h *handler) exists(r *http.Request) bool {
	switch {
	case strings.HasPrefix(r.URL.Path, "/o/"):
		key := r.PathValue("id")
		if _, ok := h.library(r.Context()).Document(document.ID(key)); ok {
			return true
		}
		if h.options.DB == nil || !id.Valid(key) {
			return false
		}
		user, _ := account.From(r.Context())
		_, err := h.options.DB.Object(r.Context(), key, user.ID)
		return err == nil
	case strings.HasPrefix(r.URL.Path, "/f/"):
		_, ok := h.library(r.Context()).Tree().Folder(r.PathValue("path"))
		return ok
	}
	return true
}

// message is a page of its own for what the workspace cannot show: no sign-in, nothing at this address.
type message struct {
	Title, Text, Link string
	Version           string
}

func (h *handler) message(w http.ResponseWriter, status int, m message) {
	m.Version = h.assetVersion
	var out bytes.Buffer
	if err := h.pages.ExecuteTemplate(&out, "message", m); err != nil {
		h.log.Error("template failed", slog.String("template", "message"), slog.String("error", err.Error()))
		http.Error(w, m.Title, status)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(out.Bytes())
}

func (h *handler) notFound(w http.ResponseWriter, _ *http.Request) {
	h.message(w, http.StatusNotFound, message{
		Title: "Page not found",
		Text:  "It may have moved. Pages without an id in their front matter get a new address when they move.",
		Link:  "To the start",
	})
}

// withoutFirstHeading drops the heading a page opens with: the title stands in the head of the window already (ADR-16).
func withoutFirstHeading(body string, startsWithTitle bool) string {
	if !startsWithTitle {
		return body
	}
	trimmed := strings.TrimLeft(body, " \t\r\n")
	if !strings.HasPrefix(trimmed, "<h1") {
		return body
	}
	end := strings.Index(trimmed, "</h1>")
	if end < 0 {
		return body
	}
	return trimmed[end+len("</h1>"):]
}

func (h *handler) file(w http.ResponseWriter, r *http.Request) {
	imagePath := r.PathValue("path")
	contentType, known := imageTypes[strings.ToLower(path.Ext(imagePath))]
	if !known || !h.library(r.Context()).HasImage(imagePath) {
		h.notFound(w, r)
		return
	}
	file, modified, err := h.store.OpenImage(imagePath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			h.log.Error("image failed", slog.String("path", imagePath), slog.String("error", err.Error()))
		}
		h.notFound(w, r)
		return
	}
	defer func() { _ = file.Close() }()
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Security-Policy", imageCSP)
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "", modified, file)
}

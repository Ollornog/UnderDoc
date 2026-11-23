// SPDX-License-Identifier: MIT

package web

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	tinysesam "github.com/Ollornog/TinySesam/clients/go"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// Sign-in only through the gate (ADR-32). TinySesam signs people in, and its Go client checks every request that comes
// through it (account.Gate). UnderDoc keeps no credentials of a person: no passwords, no passkeys, no sessions of its
// own. Sign-out, the ways in and the security of an account are the gate's.

// formLimit is the most a form of the administration may carry.
const formLimit = 16 << 10

// errNotSignedIn is a request without anyone signed in.
var errNotSignedIn = errors.New("not signed in")

// errRefused is a request the gate passed on in a way UnderDoc does not take: a Remote header or a claim of the token the
// gate's client refuses, such as a missing or unknown Remote-Kind while it is required (ADR-32, B-3).
var errRefused = errors.New("refused")

// who finds who sent a request: the person the gate's client checked, bound to their account, else the user that stands
// in for development. It fails with errNotSignedIn, or with db.ErrQuota for a person new to a tenant that has as many
// accounts as its hoster allows.
func (h *handler) who(r *http.Request) (account.User, error) {
	user, err := h.whoAny(r)
	// An agent administers nothing, also for an administrator (ADR-36): agents never manage rights.
	if err == nil && user.Agent() {
		user.Admin, user.AdminRole = false, false
	}
	return user, err
}

func (h *handler) whoAny(r *http.Request) (account.User, error) {
	ctx := r.Context()
	// Who someone is, how they signed in and what kind of account they have come only from what the gate's client
	// checked, never raw from a header. The role of the app makes an administrator (ADR-35).
	user, err := h.options.GateClient.Identify(r)
	if err == nil {
		user.AdminRole = user.Admin
		return h.bindGate(ctx, user)
	}
	// A service account no administrator allowed, and what the gate passed on wrongly, are refused, not taken for
	// nobody signed in: the gate did sign someone in (B-3).
	if errors.Is(err, account.ErrService) {
		return account.User{}, account.ErrService
	}
	if errors.Is(err, tinysesam.ErrHeader) || errors.Is(err, tinysesam.ErrClaims) {
		return account.User{}, errRefused
	}
	if h.options.DevUser != nil {
		user := *h.options.DevUser
		user.Admin = h.options.GateClient.HasAdminRole(user.Groups)
		user.AdminRole = user.Admin
		user.Spaces = h.options.GateClient.HasSpacesRole(user.Groups)
		user.AuthKind = account.AuthSession
		if user.RemoteID == "" {
			return user, nil
		}
		return h.bindGate(ctx, user)
	}
	return account.User{}, errNotSignedIn
}

// bindGate turns a person of the gate into the account of the tenant bound to their lasting ID (ADR-35), made at first
// contact. The account is written at most every few minutes; in between, what was bound is kept in memory. An account
// marked as an administrator (the emergency access) administers like one with the role.
func (h *handler) bindGate(ctx context.Context, user account.User) (account.User, error) {
	if h.options.DB == nil {
		return user, nil
	}
	t, _ := tenant.From(ctx)
	key := t + "\x00" + user.RemoteID
	h.boundMu.Lock()
	b, ok := h.bound[key]
	h.boundMu.Unlock()
	if !ok || time.Since(b.at) >= 5*time.Minute {
		a, err := h.options.DB.BindGate(ctx, user)
		if err != nil {
			if !errors.Is(err, db.ErrQuota) {
				h.log.Error("account not bound", slog.String("error", err.Error()))
			}
			return account.User{}, err
		}
		b = boundAccount{id: a.ID, admin: a.Admin, at: time.Now()}
		h.boundMu.Lock()
		h.bound[key] = b
		h.boundMu.Unlock()
	}
	user.ID = b.id
	user.Admin = user.Admin || b.admin
	return user, nil
}

// boundAccount is the account a lasting ID of the gate was bound to, and when that was read.
type boundAccount struct {
	id    string
	admin bool
	at    time.Time
}

// public tells the paths that answer without anyone signed in: those of share links, which carry a token of their own.
func public(r *http.Request) bool { return isShare(r) }

// notSignedIn answers a request without anyone signed in: UnderDoc is reached through its gate, which signs people in.
func (h *handler) notSignedIn(w http.ResponseWriter) {
	h.message(w, http.StatusUnauthorized, message{
		Title: "Not signed in",
		Text:  "UnderDoc is reached through its sign-in gate, which tells it who you are. This request came without it.",
		Link:  "Try again",
	})
}

// refused answers a request the gate passed on but UnderDoc does not take (B-3).
func (h *handler) refused(w http.ResponseWriter, err error) {
	text := "The sign-in gate passed this request on in a way UnderDoc does not take. Whoever runs the gate can tell why."
	if errors.Is(err, account.ErrService) {
		text = "This service account may not use UnderDoc. An administrator allows it by its ID in the gate."
	}
	h.message(w, http.StatusForbidden, message{Title: "Refused", Text: text, Link: "Try again"})
}

// quotaText says that a tenant has as many accounts as its hoster allows.
const quotaText = "This workspace has as many accounts as its hoster allows. Ask its administrator."

func (h *handler) page(w http.ResponseWriter, status int, name string, view any) {
	var out bytes.Buffer
	if err := h.pages.ExecuteTemplate(&out, name, view); err != nil {
		h.log.Error("template failed", slog.String("template", name), slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(out.Bytes())
}

func (h *handler) tenantName(ctx context.Context) string {
	if h.options.Tenants != tenant.ModeMany {
		return "UnderDoc"
	}
	t, _ := tenant.From(ctx)
	if found, err := h.options.DB.Tenant(ctx, t); err == nil {
		return found.Name
	}
	return "UnderDoc"
}

// formOK reads a form sent from a page of this application, and refuses one from anywhere else.
func (h *handler) formOK(w http.ResponseWriter, r *http.Request) bool {
	if !sameOrigin(r) {
		h.message(w, http.StatusForbidden, message{Title: "Not from here", Text: "This form came from another site.", Link: "To the start"})
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, formLimit)
	if err := r.ParseForm(); err != nil {
		h.message(w, http.StatusBadRequest, message{Title: "Bad request", Text: "The form could not be read.", Link: "To the start"})
		return false
	}
	return true
}

// failedPage logs what failed and answers with an error that names nothing of it.
func (h *handler) failedPage(w http.ResponseWriter, err error, what string) {
	h.log.Error(what+" failed", slog.String("error", err.Error()))
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
}

// me returns who is signed in.
func (h *handler) me(r *http.Request) account.User {
	user, _ := account.From(r.Context())
	return user
}

// adminOnly lets an administrator of the tenant through, and answers anyone else that there is nothing.
func (h *handler) adminOnly(w http.ResponseWriter, r *http.Request) bool {
	if h.options.DB == nil || !h.me(r).Admin {
		h.notFound(w, r)
		return false
	}
	return true
}

func capital(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// requestOrigin is the origin the browser sees: https, but http on the own machine.
func requestOrigin(r *http.Request) string {
	if host := tenant.Host(r.Host); host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return "http://" + r.Host
	}
	return "https://" + r.Host
}

// isAdmin tells whether the viewer administers the tenant.
func (h *handler) isAdmin(r *http.Request) bool { return h.me(r).Admin }

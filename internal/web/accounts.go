// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	tinysesam "github.com/Ollornog/TinySesam/clients/go"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/gateapi"
)

// Accounts through the gate's account API, and approvals of the gate (ADR-35, T-33). The gate keeps the accounts; UnderDoc
// asks it, with the key of its own service account there, to invite a person, to issue a new sign-in link, to lock or
// unlock an account and to reset its access, and it asks the gate to have deleting for good confirmed. The gate takes
// the person UnderDoc names as the one it acts for on trust (TinySesam-On-Behalf-Of), so UnderDoc checks itself who may
// do what, before it calls: everything here only in a browser session, never for an agent, a key, MCP or a service
// account. Without the account API configured, none of it is offered, and nothing changes.

// GateAPI is what UnderDoc may do in the gate itself. Nil turns all of it off.
type GateAPI struct {
	// Client calls the gate with the automation key of UnderDoc's service account.
	Client *gateapi.Client
	// Roles are the roles of the app UnderDoc may give a person it invites: its allowlist, never a role of an
	// administrator. Empty turns the accounts off.
	Roles []string
	// ApproverRole is the role whose holders confirm a reset of the access of an account in the gate (its approver_role);
	// empty where the key may not reset access.
	ApproverRole string
	// App is the id of UnderDoc in the gate (apps.<id>), with which the types of its approvals begin.
	App string
	// Approvals makes deleting for good wait for a signed one-time approval of the gate.
	Approvals bool
}

// accountsOn tells whether the account API is configured.
func (h *handler) accountsOn() bool {
	return h.options.Gate != nil && h.options.Gate.Client != nil && len(h.options.Gate.Roles) > 0 && h.options.DB != nil
}

// approvalsOn tells whether deleting for good waits for an approval of the gate.
func (h *handler) approvalsOn() bool {
	return h.options.Gate != nil && h.options.Gate.Client != nil && h.options.Gate.Approvals && h.options.DB != nil
}

// mayReset tells whether a user may ask the gate to reset the access of an account: the gate lets only a holder of its
// approver role confirm it (ADR-35: UnderDoc's administrators), so it is offered to them alone.
func (h *handler) mayReset(user account.User) bool {
	return h.accountsOn() && h.options.Gate.ApproverRole != "" && personAtWork(user) &&
		slices.Contains(user.Groups, h.options.Gate.ApproverRole)
}

// personAtWork tells whether a request is a person in a browser session: never an agent, a key, MCP or a service account.
func personAtWork(user account.User) bool {
	return !user.Agent() && user.AuthKind == account.AuthSession && user.Kind != account.KindService
}

// Refusals of the accounts.
var (
	errAccountsOff   = errors.New("accounts are not managed through the gate here")
	errAccountsAgent = errors.New("accounts are managed by a person in the browser, never by an agent or a key")
	errNoGateID      = errors.New("this needs a person the gate signed in")
	errAccountSelf   = errors.New("nobody does this to their own account")
	errAccountOther  = errors.New("only an administrator, a delegated administrator or an admin of a group the person is in does this")
	errAccountAdmin  = errors.New("an administrator's or a delegated administrator's account is managed by administrators only")
	errAccountSvc    = errors.New("a service account is not managed here")
	errNotBound      = errors.New("this account is bound to no account of the gate")
	errRoleAllowlist = errors.New("a role that UnderDoc may not give")
	errAccountLocked = errors.New("this account is locked: unlock it first")
	errAccountLink   = errors.New("a new sign-in link signs in as the person: only who invited them, an administrator or a delegated administrator gives one")
	errAccountUnlock = errors.New("a group admin locks nobody, they take the person out of their groups: put them into the group again instead")
)

// gateRefusal says a refusal of the gate in words a person can act on, with the status UnderDoc answers.
func gateRefusal(err error) (int, string) {
	switch {
	case errors.Is(err, gateapi.ErrNotFound), errors.Is(err, gateapi.ErrShared):
		return http.StatusConflict, "the gate does not let UnderDoc manage this account: its operator manages it"
	case errors.Is(err, gateapi.ErrSignedIn):
		return http.StatusConflict, "this person has signed in already: a new sign-in link is only for before the first sign-in; reset the access instead"
	case errors.Is(err, gateapi.ErrDisabled):
		return http.StatusConflict, "this account is locked in the gate: unlock it first"
	case errors.Is(err, gateapi.ErrOperator):
		return http.StatusConflict, "the gate's operator locked this account; only the operator unlocks it"
	case errors.Is(err, gateapi.ErrNameTaken):
		return http.StatusConflict, "this login is taken in the gate"
	case errors.Is(err, gateapi.ErrEmailTaken):
		return http.StatusConflict, "this mail address is taken in the gate"
	case errors.Is(err, gateapi.ErrRoleNotAllowed):
		return http.StatusBadRequest, "the gate does not let UnderDoc give this role"
	case errors.Is(err, gateapi.ErrLimit):
		return http.StatusTooManyRequests, "the gate allows no more of this for now: try again later"
	case errors.Is(err, &gateapi.Error{Code: "name_invalid"}):
		return http.StatusBadRequest, "the gate does not take this login"
	case errors.Is(err, &gateapi.Error{Code: "email_invalid"}):
		return http.StatusBadRequest, "the gate does not take this mail address"
	case errors.Is(err, &gateapi.Error{Code: "quota_reached"}):
		return http.StatusConflict, "the gate has as many accounts of UnderDoc as it allows"
	}
	return http.StatusBadGateway, "the gate did not do it"
}

// accountsGuard answers for whoever may not use the accounts at all, and returns the person and the gate ID UnderDoc names
// to the gate as the one it acts for.
func (h *handler) accountsGuard(w http.ResponseWriter, r *http.Request, into any) (account.User, bool) {
	if !h.accountsOn() {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": errAccountsOff.Error()})
		return account.User{}, false
	}
	user, ok := h.change(w, r, 2<<10, into)
	if !ok {
		return account.User{}, false
	}
	if !personAtWork(user) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": errAccountsAgent.Error()})
		return account.User{}, false
	}
	if !gateapi.ID(user.RemoteID) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": errNoGateID.Error()})
		return account.User{}, false
	}
	return user, true
}

// manager is how a person manages the account of another (T-33, B-7).
type manager int

const (
	// asAdministrator: every account.
	asAdministrator manager = iota + 1
	// asDelegate: every account but an administrator's or a delegated administrator's.
	asDelegate
	// asGroupAdmin: a person in none but the actor's groups. Locking takes them out of those groups, nothing more.
	asGroupAdmin
	// asInviter: a group admin who invited the person, who also gives them a new sign-in link.
	asInviter
)

// mayManageAccount decides whether and how a person manages the account of another (T-33, B-7): administrators every
// account, delegated administrators every account but an administrator's or another delegated administrator's, a group
// admin only the persons in none but their groups or whom they invited. Nobody their own, and no service account. Whether
// the account is an administrator's or a delegate's, the gate says where it manages the account (its roles now), besides
// the mark and the roles of its last sign-in (B-7): an account made by add or adopt, or promoted since, has signed in
// with neither. A gate that does not answer makes the account count as privileged, except for locking (lock), which
// must work while the gate is away.
func (h *handler) mayManageAccount(ctx context.Context, user account.User, target db.Account, lock bool) (manager, error) {
	switch {
	case target.ID == user.ID:
		return 0, errAccountSelf
	case target.Service:
		return 0, errAccountSvc
	}
	if user.Has(account.PermAdminister) {
		return asAdministrator, nil
	}
	privileged, known := h.privileged(ctx, target)
	if privileged || !known && !lock {
		return 0, errAccountAdmin
	}
	if user.Has(account.PermAccounts) {
		return asDelegate, nil
	}
	m, err := h.options.DB.MayManagePerson(ctx, user.ID, target.ID)
	switch {
	case err != nil:
		return 0, err
	case m.Invited:
		return asInviter, nil
	case m.Manages:
		return asGroupAdmin, nil
	}
	return 0, errAccountOther
}

// privileged tells whether an account is an administrator's or a delegated administrator's: by the mark of the command
// line, the roles of its last sign-in, or the roles the gate gives it now where the gate lets UnderDoc see the account
// (B-7). known is false when the gate did not answer.
func (h *handler) privileged(ctx context.Context, target db.Account) (privileged, known bool) {
	gate := h.options.GateClient
	if target.Admin || gate.HasAdminRole(target.Groups) || gate.HasDelegateRole(target.Groups) {
		return true, true
	}
	if !h.accountsOn() || target.RemoteID == "" {
		return false, true
	}
	a, err := h.options.Gate.Client.Get(ctx, target.RemoteID)
	switch {
	case errors.Is(err, gateapi.ErrNotFound):
		return false, true
	case err != nil:
		h.log.Warn("the gate did not answer for the roles of an account", slog.String("error", err.Error()))
		return false, false
	}
	return gate.HasAdminRole(a.Roles) || gate.HasDelegateRole(a.Roles), true
}

// accountFailed answers an error of the accounts and reports whether there was one.
func (h *handler) accountFailed(w http.ResponseWriter, err error, what string) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, errAccountSelf), errors.Is(err, errAccountOther), errors.Is(err, errAccountAdmin), errors.Is(err, errAccountSvc),
		errors.Is(err, errAccountsAgent), errors.Is(err, errAccountLink), errors.Is(err, errAccountUnlock):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
	case errors.Is(err, errNotBound), errors.Is(err, errAccountLocked):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, errRoleAllowlist), errors.Is(err, db.ErrLogin):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.As(err, new(*gateapi.Error)):
		status, text := gateRefusal(err)
		writeJSON(w, status, map[string]string{"error": text})
	default:
		return h.failed(w, err, what)
	}
	return true
}

// targetAccount returns the account of the path a person may manage, and how; lock for locking it.
func (h *handler) targetAccount(r *http.Request, user account.User, lock bool) (db.Account, manager, error) {
	target, err := h.options.DB.AccountByID(r.Context(), r.PathValue("id"))
	if err != nil {
		return db.Account{}, 0, err
	}
	how, err := h.mayManageAccount(r.Context(), user, target, lock)
	return target, how, err
}

// apiAccountView is an account as the group page and the users show it to whoever manages it.
type apiAccountView struct {
	ID     string `json:"id"`
	Login  string `json:"login"`
	Name   string `json:"name"`
	Locked bool   `json:"locked"`
	// Gate is the state of the account in the gate: active, disabled, disabled_operator, deleting; "unmanaged" where
	// the gate does not let UnderDoc manage it, "unbound" for an account bound to none, "unknown" when it did not answer.
	Gate     string `json:"gate"`
	SignedIn bool   `json:"signedIn"`
	// What the viewer may do with it. A group admin removes a person from their groups instead of locking them (B-7).
	Link   bool `json:"link"`
	Lock   bool `json:"lock"`
	Remove bool `json:"remove"`
	Reset  bool `json:"reset"`
}

// apiAccountInfo shows an account and what the viewer may do with it in the gate.
func (h *handler) apiAccountInfo(w http.ResponseWriter, r *http.Request) {
	if !h.accountsOn() {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": errAccountsOff.Error()})
		return
	}
	user := h.me(r)
	if !personAtWork(user) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": errAccountsAgent.Error()})
		return
	}
	target, how, err := h.targetAccount(r, user, false)
	if h.accountFailed(w, err, "account") {
		return
	}
	group := how == asGroupAdmin || how == asInviter
	out := apiAccountView{ID: target.ID, Login: target.Login, Name: target.Name, Locked: target.Locked, Gate: "unbound", Lock: !group, Remove: group}
	if target.RemoteID != "" {
		a, err := h.options.Gate.Client.Get(r.Context(), target.RemoteID)
		switch {
		case err == nil:
			out.Gate, out.SignedIn = a.State, a.SignedIn()
			out.Link = !a.SignedIn() && a.State == gateapi.StateActive && !target.Locked && how != asGroupAdmin
			out.Reset = a.SignedIn() && a.State == gateapi.StateActive && !target.Locked && h.mayReset(user)
		case errors.Is(err, gateapi.ErrNotFound):
			out.Gate = "unmanaged"
		default:
			h.log.Warn("the gate did not answer for an account", slog.String("error", err.Error()))
			out.Gate = "unknown"
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// apiInvite invites a person into a local group (T-33): the gate makes their account with roles of UnderDoc's
// allowlist and its first sign-in link, and UnderDoc binds a profile to the new lasting ID at once and puts it into the
// group, so the person finds what was shared with the group at their first sign-in. The link is in this answer only.
func (h *handler) apiInvite(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Login string   `json:"login"`
		Name  string   `json:"name"`
		Email string   `json:"email"`
		Roles []string `json:"roles"`
	}
	user, ok := h.accountsGuard(w, r, &in)
	if !ok {
		return
	}
	ctx := r.Context()
	groupID := r.PathValue("id")
	g, err := h.options.DB.GroupOf(ctx, groupID)
	if h.failed(w, err, "invite") {
		return
	}
	actor := h.actor(r)
	// Whether the group takes the person is decided before the gate makes anything.
	switch may, err := h.options.DB.MayInvite(ctx, actor, groupID); {
	case h.failed(w, err, "invite"):
		return
	case g.Source != db.GroupLocal:
		h.failed(w, db.ErrGroup, "invite")
		return
	case g.Archived:
		h.failed(w, db.ErrGroupArchived, "invite")
		return
	case !may:
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only an administrator, a delegated administrator or an admin of this group invites into it"})
		return
	}
	roles := h.options.Gate.Roles
	if len(in.Roles) > 0 {
		for _, role := range in.Roles {
			if !slices.Contains(h.options.Gate.Roles, role) {
				h.accountFailed(w, errRoleAllowlist, "invite")
				return
			}
		}
		roles = in.Roles
	}
	in.Login, in.Name, in.Email = strings.TrimSpace(in.Login), strings.TrimSpace(in.Name), strings.TrimSpace(in.Email)
	if in.Login == "" || len([]rune(in.Login)) > 200 || len([]rune(in.Name)) > 200 || len([]rune(in.Email)) > 320 {
		h.accountFailed(w, db.ErrLogin, "invite")
		return
	}
	made, err := h.options.Gate.Client.Create(ctx, user.RemoteID, gateapi.NewAccount{Username: in.Login, DisplayName: in.Name,
		Email: in.Email, Roles: slices.Clone(roles)})
	if h.accountFailed(w, err, "invite") {
		return
	}
	name := made.Account.DisplayName
	if name == "" {
		name = made.Account.Username
	}
	a, created, err := h.options.DB.AddAccount(ctx, made.Account.ID, made.Account.Username, name, made.Account.Email)
	if err == nil && created {
		err = h.options.DB.MarkInvited(ctx, a.ID, user.ID)
	}
	if err == nil {
		err = h.options.DB.InviteMember(ctx, actor, groupID, a.ID)
	}
	if err != nil {
		// The account is made in the gate and its link is valid: hand it over all the same, and say what is missing.
		h.log.Error("an invited account was not prepared", slog.String("error", err.Error()))
		h.note(r, db.LogInvited, "", fmt.Sprintf("%s (gate %s) invited, but not put into group %s; on behalf of %s",
			made.Account.Username, made.Account.ID, groupID, user.RemoteID))
		writeJSON(w, http.StatusOK, map[string]any{"account": "", "link": made.Link.URL, "expires": made.Link.ExpiresAt,
			"warning": "the person is invited, but UnderDoc could not put them into the group: add them once they signed in"})
		return
	}
	h.note(r, db.LogInvited, "", fmt.Sprintf("%s (gate %s) into group %s with %s; on behalf of %s",
		made.Account.Username, made.Account.ID, groupID, strings.Join(roles, ", "), user.RemoteID))
	writeJSON(w, http.StatusOK, map[string]any{"account": a.ID, "link": made.Link.URL, "expires": made.Link.ExpiresAt})
}

// apiSigninLink issues a new sign-in link for an account that has not signed in yet; older links die (T-33).
func (h *handler) apiSigninLink(w http.ResponseWriter, r *http.Request) {
	user, ok := h.accountsGuard(w, r, nil)
	if !ok {
		return
	}
	target, how, err := h.targetAccount(r, user, false)
	switch {
	case h.accountFailed(w, err, "sign-in link"):
		return
	case how == asGroupAdmin:
		// A new link signs in as the person: only who invited them, an administrator or a delegate gives one (B-7).
		h.accountFailed(w, errAccountLink, "sign-in link")
		return
	case target.RemoteID == "":
		h.accountFailed(w, errNotBound, "sign-in link")
		return
	case target.Locked:
		h.accountFailed(w, errAccountLocked, "sign-in link")
		return
	}
	link, err := h.options.Gate.Client.SigninLink(r.Context(), user.RemoteID, target.RemoteID)
	if h.accountFailed(w, err, "sign-in link") {
		return
	}
	h.note(r, db.LogSigninLink, "", fmt.Sprintf("new sign-in link for %s (gate %s); on behalf of %s", target.ID, target.RemoteID, user.RemoteID))
	writeJSON(w, http.StatusOK, map[string]any{"link": link.URL, "expires": link.ExpiresAt})
}

// apiSetLocked locks an account or unlocks it (T-33): in the gate, where the gate lets UnderDoc, and in UnderDoc always,
// which holds on its own (migration 039).
func (h *handler) apiSetLocked(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Locked bool `json:"locked"`
	}
	user, ok := h.accountsGuard(w, r, &in)
	if !ok {
		return
	}
	target, how, err := h.targetAccount(r, user, in.Locked)
	if h.accountFailed(w, err, "lock") {
		return
	}
	if how == asGroupAdmin || how == asInviter {
		h.removeFromMyGroups(w, r, user, target, in.Locked)
		return
	}
	// Locked in UnderDoc first, which holds on its own, then in the gate; unlocked in the gate first, so a failure on the
	// way leaves the account locked rather than open (B-7).
	var locked db.Account
	if in.Locked {
		if locked, err = h.options.DB.SetLocked(r.Context(), target.ID, true, user.ID); h.failed(w, err, "lock") {
			return
		}
		h.forget(target.RemoteID)
	}
	gate := "unbound"
	if target.RemoteID != "" {
		if in.Locked {
			_, err = h.options.Gate.Client.Disable(r.Context(), user.RemoteID, target.RemoteID)
		} else {
			_, err = h.options.Gate.Client.Enable(r.Context(), user.RemoteID, target.RemoteID)
		}
		switch {
		case err == nil:
			gate = "done"
		case errors.Is(err, gateapi.ErrNotFound), errors.Is(err, gateapi.ErrShared):
			gate = "unmanaged"
		case errors.Is(err, gateapi.ErrOperator):
			gate = "operator"
		default:
			// The lock of UnderDoc holds on its own: a gate that does not answer does not keep an account open.
			h.log.Warn("the gate did not lock or unlock an account", slog.String("error", err.Error()))
			gate = "failed"
		}
	}
	if !in.Locked {
		if locked, err = h.options.DB.SetLocked(r.Context(), target.ID, false, user.ID); h.failed(w, err, "lock") {
			return
		}
		h.forget(target.RemoteID)
	}
	action, word := db.LogUnlocked, "unlocked"
	if in.Locked {
		action, word = db.LogLocked, "locked"
	}
	h.note(r, action, "", fmt.Sprintf("%s %s (gate %s: %s); on behalf of %s", target.ID, word, cmpOr(target.RemoteID, "none"), gate, user.RemoteID))
	writeJSON(w, http.StatusOK, map[string]any{"locked": locked.Locked, "gate": gate})
}

// removeFromMyGroups is the lock of a group admin (B-7): the person leaves the groups the admin administers, and
// neither the account in UnderDoc nor the one in the gate is locked; they are not the group admin's. There is nothing
// to unlock: the admin puts the person into the group again.
func (h *handler) removeFromMyGroups(w http.ResponseWriter, r *http.Request, user account.User, target db.Account, lock bool) {
	if !lock {
		h.accountFailed(w, errAccountUnlock, "lock")
		return
	}
	n, err := h.options.DB.RemoveFromMyGroups(r.Context(), h.actor(r), target.ID)
	if h.failed(w, err, "lock") {
		return
	}
	h.note(r, db.LogMember, "", fmt.Sprintf("%s taken out of %d of the groups of %s instead of a lock; on behalf of %s", target.ID, n, user.ID, user.RemoteID))
	writeJSON(w, http.StatusOK, map[string]any{"locked": target.Locked, "removed": n, "gate": "untouched"})
}

func cmpOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// apiReset asks the gate to reset the access of an account that has signed in before: its password, factors and keys
// go, and a new sign-in link is the way back (the gate's approval tinysesam.account.reset_access). The person who asks
// confirms it in the gate, as a holder of its approver role; UnderDoc picks the link up afterwards (apiWaitApproval).
func (h *handler) apiReset(w http.ResponseWriter, r *http.Request) {
	user, ok := h.accountsGuard(w, r, nil)
	if !ok {
		return
	}
	if !h.mayReset(user) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only a holder of the gate's approver role resets the access of an account"})
		return
	}
	target, _, err := h.targetAccount(r, user, false)
	switch {
	case h.accountFailed(w, err, "reset"):
		return
	case target.RemoteID == "":
		h.accountFailed(w, errNotBound, "reset")
		return
	case target.Locked:
		h.accountFailed(w, errAccountLocked, "reset")
		return
	}
	fields := map[string]any{"account": target.RemoteID}
	asked, err := h.options.Gate.Client.Ask(r.Context(), gateapi.Ask{Type: gateapi.ResetAccess, Subject: user.RemoteID, Fields: fields})
	if h.accountFailed(w, err, "reset") {
		return
	}
	if err := h.options.DB.PutApproval(r.Context(), db.PendingApproval{ID: asked.ID, Account: user.ID, Kind: db.ApprovalReset,
		Object: target.ID, Fields: fields}); h.failed(w, err, "reset") {
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"approval": asked.ID, "approveUrl": asked.ApproveURL, "matchCode": asked.MatchCode})
}

// apiWaitApproval waits up to 25 seconds for the answer to a reset of access the viewer asked for, and hands the new
// sign-in link over once, when it comes.
func (h *handler) apiWaitApproval(w http.ResponseWriter, r *http.Request) {
	user, ok := h.accountsGuard(w, r, nil)
	if !ok {
		return
	}
	id := r.PathValue("approval")
	p, err := h.options.DB.PendingApprovalOf(r.Context(), id, user.ID)
	if err == nil && p.Kind != db.ApprovalReset {
		err = db.ErrNotFound
	}
	if h.failed(w, err, "approval") {
		return
	}
	a, err := h.options.Gate.Client.Approval(r.Context(), id, 25)
	if h.accountFailed(w, err, "approval") {
		return
	}
	if a.State == gateapi.ApprovalPending || a.State == "approved" {
		writeJSON(w, http.StatusOK, map[string]any{"state": "pending"})
		return
	}
	// Answered: the approval is taken, and the link, where there is one, goes out with this answer alone.
	if _, err := h.options.DB.TakeApproval(r.Context(), id, user.ID); h.failed(w, err, "approval") {
		return
	}
	result := a.State
	if a.State == gateapi.ApprovalConsumed {
		result = cmpOr(a.Result, "ok")
	}
	if result == "ok" && a.Link.URL == "" {
		result = "lost"
	}
	h.note(r, db.LogReset, "", fmt.Sprintf("access of %s (gate %v) reset: %s; on behalf of %s", p.Object, p.Fields["account"], result, user.RemoteID))
	out := map[string]any{"state": result}
	if result == "ok" {
		out["link"], out["expires"] = a.Link.URL, a.Link.ExpiresAt
	}
	writeJSON(w, http.StatusOK, out)
}

// --- Approvals for deleting for good (B-3, B-5): the gate's VerifyApproval behind db.SetApprover.

// approvalCookie carries the state of a browser entry of the gate's approvals: the way back is taken only by the browser
// that asked.
const approvalCookie = "underdoc_approval"

// approvalReturn is the address the gate sends the browser back to; the operator registers it for UnderDoc.
const approvalReturn = "/approvals/return"

// confirmation is a confirmation of the gate that a request carries to the approver of the database.
type confirmation struct {
	token, nonce, subject, action, object string
	fields                                map[string]any
}

type confirmationKey struct{}

// gateApprover checks a confirmation of the gate with the gate's Go client, after every other check of the action
// (db.Approver): the type of UnderDoc's action, its fields as asked, the person, the browser that asked, and once.
type gateApprover struct {
	verifier *tinysesam.ApprovalVerifier
	app      string
	jti      tinysesam.JTIStore
}

// Errors of approvals.
var (
	errApprovalOther   = errors.New("this confirmation of the gate is for another action")
	errApprovalRefused = errors.New("the confirmation of the gate does not hold")
)

func (a *gateApprover) Approve(ctx context.Context, action, objectID string) error {
	c, ok := ctx.Value(confirmationKey{}).(confirmation)
	if !ok {
		return db.ErrApproval
	}
	if c.action != action || c.object != objectID {
		return errApprovalOther
	}
	if _, err := a.verifier.Verify(ctx, c.token, tinysesam.ApprovalExpect{Type: a.app + "." + action, Fields: c.fields,
		Subject: c.subject, Nonce: c.nonce}, a.jti); err != nil {
		return fmt.Errorf("%w (%s)", errApprovalRefused, tinysesam.ClassOf(err))
	}
	return nil
}

// newApprover makes the approver of the database from the gate's address and UnderDoc's id there.
func newApprover(g *GateAPI, store *db.DB) (*gateApprover, error) {
	v, err := gateapi.NewVerifier(g.Client, g.App)
	if err != nil {
		return nil, err
	}
	return &gateApprover{verifier: v, app: g.App, jti: tinysesam.JTIStoreFunc(store.RememberJTI)}, nil
}

// askApproval asks the gate to have deleting for good confirmed by the person, and answers the browser where to go: the
// gate's page, from which it comes back to approvalReturn.
func (h *handler) askApproval(w http.ResponseWriter, r *http.Request, p db.PendingApproval) {
	user := h.me(r)
	if !gateapi.ID(user.RemoteID) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": errNoGateID.Error()})
		return
	}
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		h.failedPage(w, err, "approval")
		return
	}
	p.Nonce, p.Account = base64.RawURLEncoding.EncodeToString(nonce), user.ID
	action := db.ApprovePurge
	if p.Kind == db.ApprovalEmpty {
		action = db.ApproveEmptyTrash
	}
	asked, err := h.options.Gate.Client.Ask(r.Context(), gateapi.Ask{Type: h.options.Gate.App + "." + action, Subject: user.RemoteID,
		Fields: p.Fields, ReturnURL: requestOrigin(r) + approvalReturn, State: p.Nonce})
	if err != nil {
		h.log.Warn("the gate did not take a request for an approval", slog.String("error", err.Error()))
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "the gate did not take the request for a confirmation"})
		return
	}
	p.ID = asked.ID
	if err := h.options.DB.PutApproval(r.Context(), p); h.failed(w, err, "approval") {
		return
	}
	http.SetCookie(w, &http.Cookie{Name: approvalCookie, Value: p.Nonce, Path: approvalReturn, MaxAge: int(db.ApprovalWait.Seconds()),
		HttpOnly: true, Secure: strings.HasPrefix(requestOrigin(r), "https://"), SameSite: http.SameSiteLaxMode})
	writeJSON(w, http.StatusAccepted, map[string]any{"approveUrl": asked.ApproveURL, "matchCode": asked.MatchCode})
}

// approvalBack takes the browser back from the gate's page (approvalReturn): with the state of the browser that asked,
// it picks the confirmation up, and deletes for good what was asked for, after every check once more.
func (h *handler) approvalBack(w http.ResponseWriter, r *http.Request) {
	user := h.me(r)
	q := r.URL.Query()
	id, state := q.Get("approval"), q.Get("state")
	back := message{Title: "Nothing deleted", Link: "To the start"}
	if !h.approvalsOn() || !personAtWork(user) || !gateapi.ApprovalID(id) {
		back.Text = "This way back from the gate leads nowhere."
		h.message(w, http.StatusNotFound, back)
		return
	}
	cookie, err := r.Cookie(approvalCookie)
	p, perr := h.options.DB.PendingApprovalOf(r.Context(), id, user.ID)
	if err != nil || perr != nil || state == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) != 1 ||
		subtle.ConstantTimeCompare([]byte(p.Nonce), []byte(state)) != 1 || (p.Kind != db.ApprovalPurge && p.Kind != db.ApprovalEmpty) {
		back.Text = "This confirmation was not asked for in this browser, or it is answered already."
		h.message(w, http.StatusForbidden, back)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: approvalCookie, Value: "", Path: approvalReturn, MaxAge: -1, HttpOnly: true,
		Secure: strings.HasPrefix(requestOrigin(r), "https://"), SameSite: http.SameSiteLaxMode})
	if _, err := h.options.DB.TakeApproval(r.Context(), id, user.ID); err != nil {
		back.Text = "This confirmation is answered already."
		h.message(w, http.StatusConflict, back)
		return
	}
	if result := q.Get("result"); result != "" {
		back.Text = "The confirmation in the gate was not given (" + map[string]string{"rejected": "rejected", "expired": "expired"}[result] + "). Nothing was deleted."
		h.message(w, http.StatusOK, back)
		return
	}
	a, err := h.options.Gate.Client.Approval(r.Context(), id, 5)
	if err != nil || a.Confirmation == "" {
		if err != nil {
			h.log.Warn("the gate did not hand out a confirmation", slog.String("error", err.Error()))
		}
		back.Text = "The gate handed out no confirmation. Nothing was deleted; try again."
		h.message(w, http.StatusConflict, back)
		return
	}
	action, object := db.ApprovePurge, p.Object
	var cutoff int64
	if p.Kind == db.ApprovalEmpty {
		// The approval of emptying is bound to its cutoff (microseconds in the fields the gate signed); the database
		// names it in seconds when it asks the approver (B-6).
		n, ok := p.Fields["cutoff"].(json.Number)
		if v, err := n.Int64(); ok && err == nil {
			cutoff = v
		}
		action, object = db.ApproveEmptyTrash, strconv.FormatInt(time.UnixMicro(cutoff).Unix(), 10)
	}
	ctx := context.WithValue(r.Context(), confirmationKey{}, confirmation{token: a.Confirmation, nonce: p.Nonce,
		subject: user.RemoteID, action: action, object: object, fields: p.Fields})
	actor := h.actor(r)
	if p.Kind == db.ApprovalEmpty {
		n, err := h.options.DB.EmptyTrashBefore(ctx, actor, db.TrashName, time.UnixMicro(cutoff))
		if !h.purgeRefused(w, r, err, back) {
			h.note(r, db.LogPurge, "", fmt.Sprintf("emptied the trash: %d deleted for good, confirmed in the gate", n))
			h.message(w, http.StatusOK, message{Title: "Deleted for good", Text: strconv.Itoa(n) + " of your trash are deleted for good.", Link: "To the start"})
		}
		return
	}
	title, _ := p.Fields["title"].(string)
	err = h.options.DB.Purge(ctx, p.Object, actor, title)
	if !h.purgeRefused(w, r, err, back) {
		h.note(r, db.LogPurge, "", "deleted for good "+p.Object+", confirmed in the gate")
		h.message(w, http.StatusOK, message{Title: "Deleted for good", Text: "“" + title + "” is deleted for good.", Link: "To the start"})
	}
}

// purgeRefused answers a deletion for good that did not go, on the page of the way back, and reports whether it did not.
func (h *handler) purgeRefused(w http.ResponseWriter, _ *http.Request, err error, back message) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, errApprovalRefused), errors.Is(err, errApprovalOther), errors.Is(err, db.ErrApproval):
		back.Text = "The confirmation of the gate does not hold for this. Nothing was deleted."
		h.message(w, http.StatusForbidden, back)
	case errors.Is(err, db.ErrNotFound):
		back.Text = "It is no longer in the trash. Nothing was deleted."
		h.message(w, http.StatusNotFound, back)
	case errors.Is(err, db.ErrNotAllowed), errors.Is(err, db.ErrAgent), errors.Is(err, db.ErrSession), errors.Is(err, db.ErrConfirmName):
		back.Text = "You may not delete this for good (any more). Nothing was deleted: " + err.Error()
		h.message(w, http.StatusForbidden, back)
	default:
		h.log.Error("delete for good failed", slog.String("error", err.Error()))
		back.Text = "Something failed. Nothing was deleted."
		h.message(w, http.StatusInternalServerError, back)
	}
	return true
}

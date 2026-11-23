// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Ollornog/UnderDoc/internal/account"
)

// Actions of ADR-36. Each is set per person or group on an object to inherit (no entry), allow or deny.
const (
	// ActRead sees and opens, reads comments and embeds what it reads.
	ActRead = "read"
	// ActComment adds comments and replies.
	ActComment = "comment"
	// ActCreate makes children: pages, folders, files, uploads.
	ActCreate = "create"
	// ActWrite changes content, title and tags, reorders, and sees versions.
	ActWrite = "write"
	// ActDelete moves to the trash and restores.
	ActDelete = "delete"
	// ActShare makes and revokes share links.
	ActShare = "share"
	// ActManage changes the rights of the object, breaks and restores the inheritance.
	ActManage = "manage"
)

// Actions are all actions, in the order of the table of ADR-36.
var Actions = []string{ActRead, ActComment, ActCreate, ActWrite, ActDelete, ActShare, ActManage}

// Values of an entry. Inherit is no entry at all.
const (
	Inherit = ""
	Allow   = "allow"
	Deny    = "deny"
)

// ToAgents is the built-in group Agents as the holder of an entry: only ever deny, as a ceiling (ADR-36).
const ToAgents = "agents"

// PresetCustom names entries that are no preset: a preset changed in the rights dialog.
const PresetCustom = "custom"

// AgentActions are what an agent may do at most: the undoable work. Agents never manage rights, share, delete for good
// or read passwords (ADR-36, ADR-7).
var AgentActions = []string{ActRead, ActComment, ActCreate, ActWrite, ActDelete}

// PresetActions are the actions of a role of ADR-29, which stay as presets. Owner holds all of them.
func PresetActions(role string) []string {
	switch role {
	case RoleRead:
		return []string{ActRead}
	case RoleComment:
		return []string{ActRead, ActComment}
	case RoleEdit:
		return []string{ActRead, ActComment, ActCreate, ActWrite, ActDelete}
	case RoleManage, RoleOwner:
		return slices.Clone(Actions)
	}
	return nil
}

// PresetOf names the preset that entries make: the role whose actions are exactly the allowed ones, with nothing
// denied; PresetCustom for anything else; RoleNone for no entry.
func PresetOf(entries map[string]string) string {
	if len(entries) == 0 {
		return RoleNone
	}
	var allowed []string
	for action, value := range entries {
		if value != Allow {
			return PresetCustom
		}
		allowed = append(allowed, action)
	}
	slices.Sort(allowed)
	for _, role := range []string{RoleRead, RoleComment, RoleEdit, RoleManage} {
		want := PresetActions(role)
		slices.Sort(want)
		if slices.Equal(allowed, want) {
			return role
		}
	}
	return PresetCustom
}

// IsAction tells whether a name is one of the actions.
func IsAction(action string) bool { return slices.Contains(Actions, action) }

// agentSetting is what the connection of a request learns about it: "on" for a request of an agent (ADR-36), "off" for
// a person at work in a session and for whatever runs without a request.
func agentSetting(ctx context.Context) string {
	if requestAgent(ctx) {
		return "on"
	}
	return "off"
}

// requestAgent tells whether the request a context carries is one of an agent: a signed-in user whose sign-in is not an
// interactive session, a missing or unknown one included (account.User.Agent). A context without a user (a job, a
// visitor of a share link) is no agent.
func requestAgent(ctx context.Context) bool {
	u, ok := account.From(ctx)
	return ok && u.Agent()
}

// ErrAgent is what the group Agents never does: manage rights, share, delete for good, read passwords (ADR-36).
var ErrAgent = fmt.Errorf("%w: agents may not do this; sign in to do it yourself", ErrNotAllowed)

// ErrSession is something only a person does in the browser: a request of an agent, a key, an app password or no
// request at all is refused (ADR-35, B-3).
var ErrSession = fmt.Errorf("%w: only a person signed in in the browser does this", ErrNotAllowed)

// browserSession refuses anything but a request of the account itself, signed in in an interactive session as the
// gate's client checked it (account.User.AuthKind), never an agent. A context without a user is refused too.
func browserSession(ctx context.Context, accountID string) error {
	u, ok := account.From(ctx)
	if ok && u.Agent() {
		return ErrAgent
	}
	if !ok || u.ID == "" || u.ID != accountID || u.AuthKind != account.AuthSession {
		return ErrSession
	}
	return nil
}

// noAgent refuses a request of an agent.
func noAgent(ctx context.Context) error {
	if requestAgent(ctx) {
		return ErrAgent
	}
	return nil
}

// Decision is what decided one action of an account on an object (ADR-36): whether it is allowed, and the deciding
// entry with where it comes from, so a deny is never a riddle.
type Decision struct {
	Action  string
	Allowed bool
	// Via is owner, owner above, entry, link, private, none, agents, agents default, agents own or unknown.
	Via string
	// Object is where the deciding entry is, Title its name; Kind and Who the holder (person, group, tenant, agents,
	// link) and Name how the interface names it; Value allow or deny.
	Object, Title   string
	Kind, Who, Name string
	Value           string
}

// Ways a decision is made.
const (
	ViaOwner         = "owner"
	ViaOwnerAbove    = "owner above"
	ViaEntry         = "entry"
	ViaLink          = "link"
	ViaPrivate       = "private"
	ViaNone          = "none"
	ViaAgents        = "agents"
	ViaAgentsDefault = "agents default"
	ViaAgentsOwn     = "agents own"
	ViaUnknown       = "unknown"
)

// decideColumns reads a right_decision d with the names of where it was found and of whom.
const decideColumns = `(d).allowed, (d).via, (d).object, coalesce((SELECT title FROM objects WHERE id = (d).object), ''), (d).kind, (d).who,
	CASE (d).kind WHEN 'person' THEN coalesce((SELECT name FROM accounts WHERE id = (d).who), (d).who)
		WHEN 'group' THEN coalesce((SELECT name FROM groups WHERE id = (d).who), (d).who) ELSE '' END, (d).value`

func scanDecision(row interface{ Scan(...any) error }, action string) (Decision, error) {
	d := Decision{Action: action}
	err := row.Scan(&d.Allowed, &d.Via, &d.Object, &d.Title, &d.Kind, &d.Who, &d.Name, &d.Value)
	return d, err
}

// Decide decides one action of an account on an object, as it would for a request of an agent or not.
func (db *DB) Decide(ctx context.Context, objectID, accountID, action string, agent bool) (Decision, error) {
	return scanDecision(db.pool.QueryRow(ctx, `SELECT `+decideColumns+` FROM (SELECT object_decide($1, $2, $3, $4, true) AS d) x`,
		objectID, accountID, action, agent), action)
}

// May tells whether the account of the request may do an action on an object.
func (db *DB) May(ctx context.Context, objectID, viewer, action string) (bool, error) {
	var ok bool
	err := db.pool.QueryRow(ctx, "SELECT object_may($1, $2, $3)", objectID, viewer, action).Scan(&ok)
	return ok, err
}

// MayFind tells whether a hit of the search may show to the account of the request: an object of the database it may
// read, or something that is no object of the database at all (a document of the folder source, M-1).
func (db *DB) MayFind(ctx context.Context, objectID, viewer string) (bool, error) {
	var ok bool
	err := db.pool.QueryRow(ctx, "SELECT NOT EXISTS (SELECT 1 FROM objects WHERE id = $1) OR object_may($1, $2, 'read')", objectID, viewer).Scan(&ok)
	return ok, err
}

// ActionsOf returns the actions the account of the request may do on an object.
func (db *DB) ActionsOf(ctx context.Context, objectID, viewer string) ([]string, error) {
	var held []string
	err := db.pool.QueryRow(ctx, "SELECT object_actions($1, $2, request_agent(), true)", objectID, viewer).Scan(&held)
	return nonNil(held), err
}

// mayDo refuses an action the account of the request may not do on an object. A refusal by a deny, for Agents or for
// the account, is logged with the deciding entry once per account, object and hour, so the log tells why (ADR-36).
func (db *DB) mayDo(ctx context.Context, o Object, viewer, action string) error {
	d, err := scanDecision(db.pool.QueryRow(ctx, `SELECT `+decideColumns+` FROM (SELECT object_decide($1, $2, $3, request_agent(), true) AS d) x`,
		o.ID, viewer, action), action)
	if err != nil {
		return err
	}
	if d.Allowed {
		return nil
	}
	if d.Value == Deny && viewer != "" {
		if private, err := isPrivate(ctx, db.pool, o.ID); err == nil && !private {
			_ = db.Log(ctx, viewer, LogRefused, o.ID, DescribeDecision(d))
		}
	}
	return ErrNotAllowed
}

// DescribeDecision says in words what decided, for the log.
func DescribeDecision(d Decision) string {
	holder := strings.TrimSpace(d.Kind + " " + d.Name)
	if d.Kind == "group" || d.Kind == "person" {
		holder = d.Kind + " " + d.Name + " (" + d.Who + ")"
	}
	switch d.Via {
	case ViaAgentsDefault:
		return d.Action + ": agents never do this"
	case ViaAgentsOwn:
		return d.Action + ": the person does not let their agents do this"
	case ViaNone:
		return d.Action + ": no entry up to the top"
	}
	where := ""
	if d.Object != "" {
		where = " on " + d.Object
	}
	return d.Action + ": " + d.Value + " for " + holder + where
}

// isNotAllowed tells refusals apart from other errors.
func isNotAllowed(err error) bool { return errors.Is(err, ErrNotAllowed) }

// SPDX-License-Identifier: MIT

package share

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/files"
	"github.com/Ollornog/UnderDoc/internal/samba"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

const syscallNoFollow = syscall.O_NOFOLLOW

func accountUser(id string) account.User { return account.User{ID: id} }

func detect(name string, head []byte) string { return files.Detect(name, head) }

// The ranks of the roles (package db): read and comment read on the share, edit and above write.
const (
	rankRead = 1
	rankEdit = 3
)

// applyRights gives the companion its users and shares and the ACLs of every folder (ADR-30): read and comment read,
// edit and above write, nothing else. Whoever reads the root of a library reaches its share; whoever holds a role on a
// folder without reading the folder above gets that folder as a share of its own, and passage through the folders above.
func (ts *tenantShare) applyRights(ctx context.Context, roots []db.SharedRoot, v *view) error {
	ids := make([]string, 0, len(v.accounts))
	uid := map[string]int{}
	name := map[string]string{}
	for _, a := range v.accounts {
		ids = append(ids, a.Account)
		uid[a.Account] = a.UID
		name[a.Account] = a.Name
	}
	homes := map[string]string{}
	var shares []samba.Share
	taken := map[string]bool{}
	// A share keeps its name: a Samba user named like a share of the pass before does not push it aside (the web refuses
	// such a name; this holds if one slips through). A new share steps aside for a user, whose [homes] has the name.
	had := map[string]bool{}
	for _, info := range ts.roots {
		had[strings.ToLower(info.share)] = !info.home
	}
	users := map[string]bool{}
	for _, a := range v.accounts {
		users[strings.ToLower(a.Name)] = true
	}
	unique := func(s string) string {
		base, n := s, 2
		for taken[strings.ToLower(s)] || (users[strings.ToLower(s)] && !had[strings.ToLower(s)]) {
			s = fmt.Sprintf("%s %d", base, n)
			n++
		}
		taken[strings.ToLower(s)] = true
		return s
	}
	roots = append([]db.SharedRoot(nil), roots...)
	sort.SliceStable(roots, func(i, j int) bool { return roots[i].Role == db.RootTenant && roots[j].Role != db.RootTenant })
	acls := map[string]map[int]samba.Access{}
	infos := map[string]rootInfo{}
	reaches := map[string][]string{}
	for _, root := range roots {
		access, err := ts.m.db.FolderAccess(ctx, root.ID, ids)
		if err != nil {
			return err
		}
		t, err := ts.tree(ctx, root.ID)
		if err != nil {
			return err
		}
		rootShare := ""
		library := -1
		switch root.Role {
		case db.RootUser:
			if n := name[root.Account]; n != "" {
				homes[root.Account] = ts.full(root.ID, "")
				rootShare = n
				infos[root.ID] = rootInfo{share: n, home: true}
				reaches[root.Account] = append(reaches[root.Account], n)
			}
		default:
			rootShare = unique(samba.ShareName(root.Title))
			infos[root.ID] = rootInfo{share: rootShare}
			var users []string
			for _, a := range ids {
				if access[root.ID][a] >= rankRead {
					users = append(users, name[a])
					reaches[a] = append(reaches[a], rootShare)
				}
			}
			sort.Strings(users)
			shares = append(shares, samba.Share{Name: rootShare, Path: ts.full(root.ID, ""), Comment: root.Title, Users: users})
			library = len(shares) - 1
		}
		for folder, ranks := range access {
			p, ok := t.paths[folder]
			if !ok {
				continue
			}
			m := map[int]samba.Access{}
			for a, rank := range ranks {
				switch {
				case rank >= rankEdit:
					m[uid[a]] = samba.Write
				case rank >= rankRead:
					m[uid[a]] = samba.Read
				}
			}
			acls[ts.full(root.ID, p)] = m
		}
		if root.Role == db.RootUser {
			continue
		}
		// A folder someone reads without reading the folder above: a share of its own, and passage above it.
		own := map[string][]string{}
		// passed holds, per account, the folders it only passes through on the way to a share of its own (T-43).
		passed := map[string][]string{}
		for _, n := range t.order {
			if n.Kind != db.KindFolder || n.ID == root.ID {
				continue
			}
			p, ok := t.paths[n.ID]
			if !ok {
				continue
			}
			// smb.conf substitutes variables such as %U in a path, so a folder with '%' in its path cannot be a share of
			// its own: it would be another folder for each user. It stays reachable through the share of its library.
			if strings.ContainsRune(p, '%') {
				ts.m.log.Warn("samba: a folder with '%' in its path is no share of its own", "tenant", ts.tenant, "folder", n.ID)
				continue
			}
			for _, a := range ids {
				if access[n.ID][a] >= rankRead && access[n.Parent][a] < rankRead {
					own[n.ID] = append(own[n.ID], a)
				}
			}
		}
		folders := make([]string, 0, len(own))
		for f := range own {
			folders = append(folders, f)
		}
		sort.Slice(folders, func(i, j int) bool { return t.paths[folders[i]] < t.paths[folders[j]] })
		for _, f := range folders {
			shareName := unique(samba.ShareName(t.nodes[f].Name))
			infos[f] = rootInfo{share: shareName}
			var users []string
			for _, a := range own[f] {
				users = append(users, name[a])
				reaches[a] = append(reaches[a], shareName)
				for up := t.nodes[f].Parent; up != ""; up = t.nodes[up].Parent {
					full := ts.full(root.ID, t.paths[up])
					if acls[full] == nil {
						acls[full] = map[int]samba.Access{}
					}
					if _, has := acls[full][uid[a]]; !has {
						acls[full][uid[a]] = samba.Pass
						// The root of the library is the share itself, which a veto cannot hide.
						if up != root.ID {
							passed[a] = append(passed[a], up)
						}
					}
					if up == root.ID {
						break
					}
				}
			}
			sort.Strings(users)
			shares = append(shares, samba.Share{Name: shareName, Path: ts.full(root.ID, t.paths[f]), Comment: t.nodes[f].Name, Users: users})
		}
		if library >= 0 && len(passed) > 0 {
			// Only who reaches the library's share is hidden anything there; the others do not reach it at all.
			veto := ts.hidden(root.ID, t, access, passed, name)
			for u := range veto {
				if !slices.Contains(shares[library].Users, u) {
					delete(veto, u)
				}
			}
			if len(veto) > 0 {
				shares[library].Veto = veto
			}
		}
	}
	state := samba.State{Shares: shares}
	for _, a := range v.accounts {
		secret, err := ts.m.vault.Open(ts.tenant, db.SambaSecretName(a.Account), a.Sealed)
		if err != nil {
			ts.m.log.Warn("samba: the app password of an account does not open", "account", a.Account, "err", err)
			continue
		}
		state.Users = append(state.Users, samba.User{Name: a.Name, UID: a.UID, NTHash: string(secret), Home: homes[a.Account]})
	}
	key := stateKey(state)
	if key != ts.state {
		if _, err := ts.helper.Do(ctx, samba.Request{Op: samba.OpApply, State: &state}); err != nil {
			return err
		}
		ts.state = key
	}
	ts.roots = infos
	ts.reaches = reaches
	// Only the ACLs that changed, folders from the top down.
	var change []samba.PathACL
	for p, m := range acls {
		k := aclKey(m)
		if ts.acl[p] != k {
			change = append(change, samba.PathACL{Path: p, Access: m})
		}
	}
	// The files that came to a folder since the pass before get its ACL, whether or not the folder's changed (then the
	// helper sets the files in it anyway); after the folders, as a path sorts after its folder's.
	changing := map[string]bool{}
	for _, c := range change {
		changing[c.Path] = true
	}
	for _, f := range v.refresh {
		dir := filepath.Dir(f)
		if m, ok := acls[dir]; ok && !changing[dir] && !changing[f] {
			changing[f] = true
			change = append(change, samba.PathACL{Path: f, Access: m})
		}
	}
	sort.Slice(change, func(i, j int) bool { return change[i].Path < change[j].Path })
	if len(change) > 0 {
		if _, err := ts.helper.Do(ctx, samba.Request{Op: samba.OpACL, ACL: change}); err != nil {
			return err
		}
		for _, c := range change {
			ts.acl[c.Path] = aclKey(c.Access)
		}
	}
	for p := range ts.acl {
		if _, ok := acls[p]; !ok {
			delete(ts.acl, p)
		}
	}
	return nil
}

// hidden names, per Samba user, the folders of a library they only pass through, to hide them in its share (T-43): there
// a name in such a folder answers the same whether it exists or not. Samba hides by name, not by path, so a folder is not
// hidden when the user reads something of the same name in the library (or the name cannot stand in a veto list); it
// keeps telling a guessed name then, and the log says so. Hiding never takes away what the user may read.
func (ts *tenantShare) hidden(rootID string, t tree, access map[string]map[string]int, passed map[string][]string, name map[string]string) map[string][]string {
	out := map[string][]string{}
	for a, folders := range passed {
		readable := map[string]bool{}
		for _, n := range t.order {
			if n.ID == rootID {
				continue
			}
			if _, on := t.paths[n.ID]; !on {
				continue
			}
			from := n.ID
			if n.Kind != db.KindFolder {
				from = n.Parent
			}
			if access[from][a] >= rankRead {
				readable[strings.ToLower(n.Name)] = true
			}
		}
		seen := map[string]bool{}
		for _, f := range folders {
			n := t.nodes[f].Name
			if seen[strings.ToLower(n)] {
				continue
			}
			seen[strings.ToLower(n)] = true
			if readable[strings.ToLower(n)] || !samba.ValidVeto(n) {
				ts.m.log.Warn("samba: a folder passed through keeps telling a guessed name, as the user reads a name like it in the library",
					"tenant", ts.tenant, "folder", f, "account", a)
				continue
			}
			out[name[a]] = append(out[name[a]], n)
		}
		sort.Strings(out[name[a]])
		if len(out[name[a]]) == 0 {
			delete(out, name[a])
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func aclKey(m map[int]samba.Access) string {
	keys := make([]string, 0, len(m))
	for uid, a := range m {
		keys = append(keys, fmt.Sprintf("%d=%s", uid, a))
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

// FilesChanged tells the working copy of the tenant of ctx to look soon; the database does it through its notifications,
// this is for callers that know better.
func (m *Manager) FilesChanged(ctx context.Context) {
	t, _ := tenant.From(ctx)
	if ts := m.tenants[t]; ts != nil {
		ts.poke()
	}
}

// Guard holds the web back from a file or folder of a shared library that is open over SMB (ADR-30): it answers an
// OpenError that names who has it open. Without a companion, or when it does not answer, nobody has anything open.
func (m *Manager) Guard(ctx context.Context, objectID string) error {
	t, _ := tenant.From(ctx)
	ts := m.tenants[t]
	if ts == nil {
		return nil
	}
	rec, root, ok := ts.record(ctx, objectID)
	if !ok {
		return nil
	}
	full := ts.full(root, rec.Path)
	r, err := ts.helper.Do(ctx, samba.Request{Op: samba.OpOpen})
	if err != nil {
		return nil
	}
	var users []string
	seen := map[string]bool{}
	for _, o := range r.Open {
		if o.Path == full || (rec.Kind == db.KindFolder && strings.HasPrefix(o.Path, full+"/")) {
			if !seen[o.User] {
				seen[o.User] = true
				users = append(users, o.User)
			}
		}
	}
	if len(users) == 0 {
		return nil
	}
	sort.Strings(users)
	return &db.OpenError{Users: users}
}

// record finds the record of an object in a shared library.
func (ts *tenantShare) record(ctx context.Context, objectID string) (db.ShareRecord, string, bool) {
	root, err := ts.m.db.SharedRootOf(ctx, objectID)
	if err != nil || root == "" {
		return db.ShareRecord{}, "", false
	}
	list, err := ts.m.db.ShareRecords(ctx, root)
	if err != nil {
		return db.ShareRecord{}, "", false
	}
	for _, r := range list {
		if r.Object == objectID {
			return r, root, true
		}
	}
	return db.ShareRecord{}, "", false
}

// Place is where an object of a shared library is on the share: the path to copy, and who has it open now.
type Place struct {
	Path   string   `json:"path"`
	OpenBy []string `json:"open_by,omitempty"`
}

// PlaceOf returns where an object is on the share, ok false when it is in no shared library or not on disk yet. The path
// goes through the share of the library, or through a share of its own that a folder above it is for the viewer.
func (m *Manager) PlaceOf(ctx context.Context, objectID, viewer string) (Place, bool) {
	t, _ := tenant.From(ctx)
	ts := m.tenants[t]
	if ts == nil {
		return Place{}, false
	}
	rec, root, ok := ts.record(ctx, objectID)
	if !ok {
		return Place{}, false
	}
	ts.mu.Lock()
	infos := ts.roots
	reach := map[string]bool{}
	for _, s := range ts.reaches[viewer] {
		reach[s] = true
	}
	ts.mu.Unlock()
	share, rel := "", rec.Path
	if info, ok := infos[root]; ok && reach[info.share] {
		share = info.share
	} else {
		// The nearest folder above that is a share of its own for the viewer.
		list, err := m.db.ShareRecords(ctx, root)
		if err != nil {
			return Place{}, false
		}
		best := -1
		for _, r := range list {
			info, ok := infos[r.Object]
			if !ok || r.Kind != db.KindFolder || !reach[info.share] {
				continue
			}
			if rec.Path == r.Path || strings.HasPrefix(rec.Path, r.Path+"/") {
				if len(r.Path) > best {
					best, share, rel = len(r.Path), info.share, strings.TrimPrefix(strings.TrimPrefix(rec.Path, r.Path), "/")
				}
			}
		}
	}
	if share == "" {
		return Place{}, false
	}
	p := `\\` + m.cfg.Server + `\` + share
	if rel != "" {
		p += `\` + strings.ReplaceAll(rel, "/", `\`)
	}
	place := Place{Path: p}
	if r, err := ts.helper.Do(ctx, samba.Request{Op: samba.OpOpen, Paths: []string{ts.full(root, rec.Path)}}); err == nil {
		for _, o := range r.Open {
			place.OpenBy = append(place.OpenBy, o.User)
		}
		sort.Strings(place.OpenBy)
	}
	return place, true
}

// Reaches returns the shares an account reaches, as paths to copy, after the last pass.
func (m *Manager) Reaches(ctx context.Context, accountID string) []string {
	t, _ := tenant.From(ctx)
	ts := m.tenants[t]
	if ts == nil {
		return nil
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	var out []string
	for _, s := range ts.reaches[accountID] {
		out = append(out, `\\`+m.cfg.Server+`\`+s)
	}
	sort.Strings(out)
	return out
}

// NameTaken tells whether a name is the name of a share of the tenant of ctx, as the last pass made it or as a shared
// library would get it now, in any case: a Samba user may not take it, since its [homes] would push the share aside.
func (m *Manager) NameTaken(ctx context.Context, name string) (bool, error) {
	t, _ := tenant.From(ctx)
	ts := m.tenants[t]
	if ts == nil {
		return false, nil
	}
	name = strings.ToLower(name)
	ts.mu.Lock()
	for _, info := range ts.roots {
		if !info.home && strings.ToLower(info.share) == name {
			ts.mu.Unlock()
			return true, nil
		}
	}
	ts.mu.Unlock()
	roots, err := m.db.SharedRoots(ctx)
	if err != nil {
		return false, err
	}
	for _, r := range roots {
		if r.Role != db.RootUser && strings.ToLower(samba.ShareName(r.Title)) == name {
			return true, nil
		}
	}
	return false, nil
}

// Server is the name of the server as share paths show it.
func (m *Manager) Server() string { return m.cfg.Server }

// UIDs is the range of the UIDs of Samba users.
func (m *Manager) UIDs() (int, int) { return m.cfg.UIDMin, m.cfg.UIDMax }

// On tells whether the tenant of ctx has a companion.
func (m *Manager) On(ctx context.Context) bool {
	t, _ := tenant.From(ctx)
	return m.tenants[t] != nil
}

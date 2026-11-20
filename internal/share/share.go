// SPDX-License-Identifier: MIT

// Package share keeps the working copy of the libraries shared over Samba (ADR-30): it writes their files to disk, takes
// every change made on disk into the database as the account that made it, gives the Samba companion its users, shares
// and ACLs, and holds the web back from a file that is open over SMB. UnderDoc does all of it without root; the helper
// of the companion does the few things that need it (package samba).
package share

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/samba"
	"github.com/Ollornog/UnderDoc/internal/store"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/vault"
)

// Helper is the helper of a Samba companion: samba.Client, or a stand-in in tests.
type Helper interface {
	Do(ctx context.Context, r samba.Request) (samba.Response, error)
}

// Config is how the working copy is set up.
type Config struct {
	// Dir is the share directory; each tenant has a directory of its own below it, and a library one below that.
	Dir string
	// Helpers names the companion of each tenant that has one.
	Helpers map[string]Helper
	// Server is the name of the server as a share path shows it, such as "files" for \\files\Company.
	Server string
	// UIDMin and UIDMax are the range of the UIDs of Samba users.
	UIDMin, UIDMax int
	// Quiet is how long a path stays without an event before a pass takes it; 2 seconds by default.
	Quiet time.Duration
	// Every is the time between full passes without an event; an hour by default.
	Every time.Duration
}

// Manager keeps the working copies of all tenants with a companion.
type Manager struct {
	db      *db.DB
	store   *store.Store
	vault   *vault.Vault
	cfg     Config
	log     *slog.Logger
	tenants map[string]*tenantShare
}

// New makes the manager. It needs the vault for the NT hashes of the app passwords.
func New(database *db.DB, st *store.Store, v *vault.Vault, cfg Config, log *slog.Logger) (*Manager, error) {
	if !v.Ready() {
		return nil, errors.New("samba needs " + vault.KeyEnv + ": the NT hashes of the app passwords are sealed")
	}
	if st == nil {
		return nil, errors.New("samba needs the content store (UNDERDOC_STORE_DIR)")
	}
	if cfg.Dir == "" || !filepath.IsAbs(cfg.Dir) {
		return nil, errors.New("samba needs UNDERDOC_SHARE_DIR, an absolute directory")
	}
	if cfg.Quiet == 0 {
		cfg.Quiet = 2 * time.Second
	}
	if cfg.Every == 0 {
		cfg.Every = time.Hour
	}
	if cfg.UIDMin == 0 {
		cfg.UIDMin, cfg.UIDMax = 20000, 29999
	}
	if log == nil {
		log = slog.Default()
	}
	m := &Manager{db: database, store: st, vault: v, cfg: cfg, log: log, tenants: map[string]*tenantShare{}}
	on := map[string]bool{}
	for t, h := range cfg.Helpers {
		if !tenant.Valid(t) {
			return nil, fmt.Errorf("samba: %q is not a tenant", t)
		}
		dir := filepath.Join(cfg.Dir, t)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, err
		}
		m.tenants[t] = &tenantShare{m: m, tenant: t, dir: dir, helper: h, kick: make(chan struct{}, 1),
			acl: map[string]string{}, roots: map[string]rootInfo{}, former: map[string]string{}, formerUID: map[int]string{},
			held: map[string]bool{}}
		on[t] = true
	}
	database.SetSamba(on)
	database.SetFileGuard(m.Guard)
	return m, nil
}

// Dir returns the directory of a tenant's working copy.
func (m *Manager) Dir(t string) string { return filepath.Join(m.cfg.Dir, t) }

// Start runs the working copies until ctx ends: a watcher per tenant, the notifications of the database, and a full pass
// at start and every hour.
func (m *Manager) Start(ctx context.Context) {
	for _, ts := range m.tenants {
		w, err := newWatcher(ts.poke)
		if err != nil {
			m.log.Error("samba: no watcher", "tenant", ts.tenant, "err", err)
		} else {
			ts.watcher = w
			go func() {
				<-ctx.Done()
				w.close()
			}()
		}
		go ts.loop(ctx)
		ts.poke()
	}
	go m.listen(ctx)
}

// listen waits for the notifications of the database and pokes the tenant each names; after a break it pokes every
// tenant, since a pass catches what changed meanwhile.
func (m *Manager) listen(ctx context.Context) {
	for ctx.Err() == nil {
		err := m.db.ListenShare(ctx, func(t string) {
			if ts := m.tenants[t]; ts != nil {
				ts.poke()
			}
		})
		if ctx.Err() != nil {
			return
		}
		m.log.Warn("samba: listening to the database", "err", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
		for _, ts := range m.tenants {
			ts.poke()
		}
	}
}

// Sync runs one pass for a tenant now and returns when it is done: for tests and for the start.
func (m *Manager) Sync(ctx context.Context, t string) error {
	ts := m.tenants[t]
	if ts == nil {
		return db.ErrNoSamba
	}
	_, err := ts.pass(ctx)
	return err
}

// tenantShare is the working copy of one tenant.
type tenantShare struct {
	m       *Manager
	tenant  string
	dir     string
	helper  Helper
	kick    chan struct{}
	watcher *watcher

	mu      sync.Mutex // one pass at a time
	state   string     // the state last applied to the companion
	boot    string     // the start of the helper that state was applied to
	acl     map[string]string
	roots   map[string]rootInfo // per shared library: its share name, for share paths
	reaches map[string][]string // per account: the shares it reaches
	seen    uint64              // the newest event of the audit the last pass saw

	// former keeps the Samba users of earlier passes by name and UID, so a change made before an app password was
	// removed is still taken as the account that made it.
	former    map[string]string
	formerUID map[int]string
	held      map[string]bool // what the last pass reported, so the log says it once
}

// rootInfo is what a share path needs of a library.
type rootInfo struct {
	share string
	home  bool
}

// poke asks for a pass.
func (ts *tenantShare) poke() {
	select {
	case ts.kick <- struct{}{}:
	default:
	}
}

// loop runs a pass after every poke, once nothing has happened for the quiet time, and at least every hour.
func (ts *tenantShare) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ts.kick:
		case <-time.After(ts.m.cfg.Every):
		}
		quiet := time.NewTimer(ts.m.cfg.Quiet)
	settle:
		for {
			select {
			case <-ctx.Done():
				quiet.Stop()
				return
			case <-ts.kick:
				quiet.Reset(ts.m.cfg.Quiet)
			case <-quiet.C:
				break settle
			}
		}
		busy, err := ts.pass(ctx)
		if err != nil && ctx.Err() == nil {
			ts.m.log.Error("samba: pass", "tenant", ts.tenant, "err", err)
		}
		if busy || err != nil {
			// Something was open or failed: look again soon.
			go func() {
				select {
				case <-ctx.Done():
				case <-time.After(ts.m.cfg.Quiet):
					ts.poke()
				}
			}()
		}
	}
}

// view is what a pass knows of the tenant: the Samba users and what the companion saw.
type view struct {
	accounts  []db.SambaAccount
	byName    map[string]string // Samba user → account
	byUID     map[int]string    // UID → account
	who       map[string]string // full path → the account that last changed it
	open      map[string][]string
	openWrite map[string]bool
	busy      bool
	// held is what this pass reported or would have: left alone, put back, kept on disk.
	held map[string]bool
	told map[string]bool // what the pass before reported
	// refresh are the files that came to a path or an inode the companion has not set the ACL of: moved, new, or saved
	// through a rename. Each gets the ACL of its folder again.
	refresh []string
}

// pass brings the working copy and the database of the tenant in step, gives the companion its state and the ACLs, and
// reports whether something was left for later because it was open.
func (ts *tenantShare) pass(ctx context.Context) (bool, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ctx = db.FromShare(tenant.With(ctx, ts.tenant))
	roots, err := ts.m.db.SharedRoots(ctx)
	if err != nil {
		return false, err
	}
	accounts, err := ts.m.db.SambaAccounts(ctx)
	if err != nil {
		return false, err
	}
	v := &view{accounts: accounts, byName: map[string]string{}, byUID: map[int]string{}, who: map[string]string{},
		open: map[string][]string{}, openWrite: map[string]bool{}, held: map[string]bool{}, told: ts.held}
	defer func() { ts.held = v.held }()
	for name, acc := range ts.former {
		v.byName[name] = acc
	}
	for uid, acc := range ts.formerUID {
		v.byUID[uid] = acc
	}
	for _, a := range accounts {
		v.byName[a.Name] = a.Account
		v.byUID[a.UID] = a.Account
		ts.former[a.Name] = a.Account
		ts.formerUID[a.UID] = a.Account
	}
	// A companion that started again has forgotten its users, its shares and its audit: they are applied again, and the
	// numbers of the audit start anew.
	if r, err := ts.helper.Do(ctx, samba.Request{Op: samba.OpPing}); err == nil && r.Boot != ts.boot {
		ts.boot, ts.state, ts.seen = r.Boot, "", 0
	}
	if r, err := ts.helper.Do(ctx, samba.Request{Op: samba.OpWho}); err == nil {
		for _, e := range r.Who {
			if acc := v.byName[e.User]; acc != "" {
				v.who[e.Path] = acc
				if e.To != "" {
					v.who[e.To] = acc
				}
			}
		}
		// Events pushed out of the audit before this pass asked: whoever changed something then is unknown, and what
		// they changed is put back (actor). Said in the log, since it is either a flood or a pass that came too late.
		if ts.seen > 0 && r.First > ts.seen+1 {
			lost := r.First - ts.seen - 1
			ts.m.log.Warn("samba: the audit lost events before the pass read them; changes whose writer is unknown are put back",
				"tenant", ts.tenant, "lost", lost)
			_ = ts.m.db.Log(ctx, "", LogShare, "", fmt.Sprintf("the audit of the share lost %d events; changes whose writer is unknown were put back", lost))
		}
		if r.Last > 0 {
			ts.seen = r.Last
		}
	}
	if r, err := ts.helper.Do(ctx, samba.Request{Op: samba.OpOpen}); err == nil {
		for _, o := range r.Open {
			v.open[o.Path] = append(v.open[o.Path], o.User)
			if o.Write {
				v.openWrite[o.Path] = true
			}
		}
	}
	keep := map[string]bool{}
	var failed []string
	for _, root := range roots {
		keep[root.ID] = true
		if err := ts.syncRoot(ctx, root, v); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", root.ID, err))
		}
	}
	ts.dropUnshared(ctx, keep, v)
	if err := ts.applyRights(ctx, roots, v); err != nil {
		failed = append(failed, err.Error())
	}
	if ts.watcher != nil {
		ts.watcher.watch(ts.dir)
	}
	if len(failed) > 0 {
		return v.busy, errors.New(strings.Join(failed, "; "))
	}
	return v.busy, nil
}

// dropUnshared takes the directories of libraries that are no longer shared off the disk, with their records; the
// store keeps everything. Only directories named like an object are touched. A last pass first takes what changed there
// since the pass before into the database, as the account that changed it; a directory that still holds something the
// database did not take stays on disk, said once in the log, and is looked at again with every pass.
func (ts *tenantShare) dropUnshared(ctx context.Context, keep map[string]bool, v *view) {
	list, err := os.ReadDir(ts.dir)
	if err != nil {
		return
	}
	for _, d := range list {
		if !d.IsDir() || !validID(d.Name()) || keep[d.Name()] {
			continue
		}
		dir := filepath.Join(ts.dir, d.Name())
		root, err := ts.m.db.ShareRootInfo(ctx, d.Name())
		if err != nil && !errors.Is(err, db.ErrNotFound) {
			ts.m.log.Warn("samba: a library that is no longer shared", "root", d.Name(), "err", err)
			continue
		}
		root.ID = d.Name()
		if err := ts.diskToDB(ctx, root, dir, v); err != nil {
			ts.m.log.Warn("samba: the last pass over a library that is no longer shared", "root", root.ID, "err", err)
			continue
		}
		left, err := ts.untaken(ctx, root.ID, dir)
		if err != nil {
			ts.m.log.Warn("samba: the last pass over a library that is no longer shared", "root", root.ID, "err", err)
			continue
		}
		if len(left) > 0 {
			if v.report("drop:" + root.ID) {
				ts.m.log.Warn("samba: a library that is no longer shared stays on disk: it holds what the database did not take",
					"root", root.ID, "paths", left)
				_ = ts.m.db.Log(ctx, "", LogShare, root.ID, "no longer shared, but it stays on disk: the database did not take "+
					strings.Join(left, ", "))
			}
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			ts.m.log.Warn("samba: removing a library that is no longer shared", "root", root.ID, "err", err)
			continue
		}
		_ = ts.m.db.DeleteShareRecords(ctx, root.ID)
	}
}

// untaken lists what lies in the directory of a library that the database does not hold: an entry without an ID, or a
// file that is neither the current version of its object nor what the last pass agreed on. At most ten.
func (ts *tenantShare) untaken(ctx context.Context, rootID, dir string) ([]string, error) {
	t, err := ts.tree(ctx, rootID)
	if err != nil {
		return nil, err
	}
	list, err := ts.m.db.ShareRecords(ctx, rootID)
	if err != nil {
		return nil, err
	}
	recs := map[string]db.ShareRecord{}
	for _, r := range list {
		recs[r.Object] = r
	}
	disk, err := scan(dir)
	if err != nil {
		return nil, err
	}
	var left []string
	for p, e := range disk {
		if p == "" {
			continue
		}
		taken := e.id != ""
		if taken && !e.dir {
			r, had := recs[e.id]
			taken = had && r.Path == p && r.Inode == e.inode && r.Size == e.size && r.MtimeNS == e.mtimeNS
			if n, ok := t.nodes[e.id]; !taken && ok {
				sha, _, err := hashFile(filepath.Join(dir, filepath.FromSlash(p)))
				taken = err == nil && sha == n.SHA
			}
		}
		if !taken {
			left = append(left, p)
		}
	}
	sort.Strings(left)
	if len(left) > 10 {
		left = append(left[:10], "…")
	}
	return left, nil
}

// full is the absolute path of a path of a library.
func (ts *tenantShare) full(rootID, rel string) string {
	return filepath.Join(ts.dir, rootID, filepath.FromSlash(rel))
}

// actor returns the account that made a change on disk: the audit of the companion, else the owner of the file on disk
// when that is a Samba user; ok false when neither knows. An unknown writer is never anyone else, not the owner of the
// object nor of the library: what it changed is put back (ADR-30, after an attack round).
func (v *view) actor(full string, e *entry) (string, bool) {
	if acc := v.who[full]; acc != "" {
		return acc, true
	}
	if e != nil {
		if acc := v.byUID[e.uid]; acc != "" {
			return acc, true
		}
	}
	return "", false
}

// report tells whether something is to be said in the log: not when the pass before said it already.
func (v *view) report(key string) bool {
	first := !v.held[key] && !v.told[key]
	v.held[key] = true
	return first
}

// putBack logs a change on disk whose writer is unknown and that the pass therefore does not take; the disk follows the
// database again.
func (ts *tenantShare) putBack(ctx context.Context, v *view, objectID, what string) {
	if !v.report("back:" + objectID + ":" + what) {
		return
	}
	ts.m.log.Warn("samba: a change whose writer is unknown is put back", "tenant", ts.tenant, "object", objectID, "change", what)
	_ = ts.m.db.Log(ctx, "", LogShare, objectID, what+" over Samba was put back: the writer is unknown")
}

// leftAlone logs once that something new on disk was not taken, since its writer is unknown; it stays on disk.
func (ts *tenantShare) leftAlone(ctx context.Context, v *view, root, rel string) {
	if !v.report("new:" + root + "/" + rel) {
		return
	}
	ts.m.log.Warn("samba: something new whose writer is unknown is not taken", "tenant", ts.tenant, "root", root, "path", rel)
	_ = ts.m.db.Log(ctx, "", LogShare, root, rel+" is new on the share and was not taken: the writer is unknown")
}

// LogShare is the action of the log for what the working copy did on its own: put back or left alone what came from
// an unknown writer, kept a file off the share.
const LogShare = "samba"

// syncRoot brings one library in step: first what changed on disk goes into the database, as the account that made it,
// then the disk is made to show the database.
func (ts *tenantShare) syncRoot(ctx context.Context, root db.SharedRoot, v *view) error {
	dir := filepath.Join(ts.dir, root.ID)
	if err := os.MkdirAll(dir, 0o770); err != nil {
		return err
	}
	if readID(dir) != root.ID {
		if err := writeID(dir, root.ID); err != nil {
			return err
		}
	}
	if err := ts.diskToDB(ctx, root, dir, v); err != nil {
		return err
	}
	return ts.dbToDisk(ctx, root, dir, v)
}

// tree is the database side of a library: its objects by ID and their paths.
type tree struct {
	nodes map[string]db.ShareNode
	paths map[string]string // object → path in the library
	order []db.ShareNode
}

func (ts *tenantShare) tree(ctx context.Context, rootID string) (tree, error) {
	list, err := ts.m.db.ShareTree(ctx, rootID)
	if err != nil {
		return tree{}, err
	}
	t := tree{nodes: map[string]db.ShareNode{}, paths: map[string]string{}, order: list}
	for _, n := range list {
		t.nodes[n.ID] = n
		if n.ID == rootID {
			t.paths[n.ID] = ""
			continue
		}
		// A name that is no file name, such as "..", one with a slash or a control character, made before names were
		// checked, never becomes a path: it and what lies below it stay off the disk.
		if clean, err := db.CleanFileName(n.Name); err != nil || clean != n.Name {
			continue
		}
		// A name the scan leaves out as temporary would vanish from the disk at the next pass and go to the trash; it
		// stays off the disk. So does a file that breaks the inheritance: Samba knows rights per folder only, and its
		// folder's ACL would show it to whoever reads the folder (ADR-29, ADR-30). The database refuses to let one in
		// (perFolder); this holds for a library shared with one already in it, such as the folder of the tenant.
		if ignored(n.Name, n.Kind == db.KindFolder) || (n.Kind == db.KindFile && n.Breaks) {
			continue
		}
		if p, ok := t.paths[n.Parent]; ok {
			t.paths[n.ID] = path.Join(p, n.Name)
		}
	}
	return t, nil
}

func parentOf(rel string) string {
	p := path.Dir(rel)
	if p == "." {
		return ""
	}
	return p
}

// diskToDB takes what changed on disk since the records into the database.
func (ts *tenantShare) diskToDB(ctx context.Context, root db.SharedRoot, dir string, v *view) error {
	t, err := ts.tree(ctx, root.ID)
	if err != nil {
		return err
	}
	list, err := ts.m.db.ShareRecords(ctx, root.ID)
	if err != nil {
		return err
	}
	recs := map[string]db.ShareRecord{}
	for _, r := range list {
		recs[r.Object] = r
	}
	disk, err := scan(dir)
	if err != nil {
		return err
	}
	ts.breakLinks(dir, disk, recs)
	// Which entry carries which ID: one entry per ID, the one where the record says it lies when the ID is found twice.
	byID := map[string]*entry{}
	for _, e := range disk {
		if e.id == "" || e.path == "" {
			continue
		}
		if _, known := t.nodes[e.id]; !known {
			if _, had := recs[e.id]; !had {
				e.id = ""
				continue
			}
		}
		if other := byID[e.id]; other != nil {
			if r, ok := recs[e.id]; ok && r.Path == other.path {
				e.id = ""
				continue
			}
			other.id = ""
		}
		byID[e.id] = e
	}
	claimed := map[string]bool{"": true}
	at := map[string]string{"": root.ID} // path on disk → object
	sorted := make([]db.ShareRecord, 0, len(list))
	for _, r := range list {
		if r.Object != root.ID {
			sorted = append(sorted, r)
		}
	}
	sort.Slice(sorted, func(i, j int) bool { return depth(sorted[i].Path) < depth(sorted[j].Path) })
	vanished := map[string]bool{}
	for _, r := range sorted {
		n, inDB := t.nodes[r.Object]
		e := byID[r.Object]
		if e == nil {
			// A file without an ID where the record says this one lies: it was saved through a temporary file and a
			// rename (Office, T-1), or replaced; it is the same object with new content.
			if x := disk[r.Path]; x != nil && x.id == "" && !claimed[x.path] && x.dir == (r.Kind == db.KindFolder) {
				e = x
			}
		}
		if e == nil {
			vanished[r.Path] = true
			if inDB && t.paths[r.Object] == r.Path && !vanished[parentOf(r.Path)] {
				full := ts.full(root.ID, r.Path)
				actor, known := v.actor(full, nil)
				if !known {
					ts.putBack(ctx, v, n.ID, "a deletion")
				} else if err := ts.m.db.Delete(ctx, n.ID, actor); err != nil {
					ts.m.log.Warn("samba: a deletion was not taken", "object", n.ID, "account", actor, "err", err)
				} else {
					_ = ts.m.db.DeleteShareRecords(ctx, root.ID, n.ID)
				}
			}
			continue
		}
		claimed[e.path] = true
		at[e.path] = r.Object
		if !inDB {
			continue // gone in the database: the disk follows below
		}
		full := ts.full(root.ID, e.path)
		if e.path != r.Path && t.paths[r.Object] == r.Path {
			parent, ok := at[parentOf(e.path)]
			if !ok {
				if pe := disk[parentOf(e.path)]; pe != nil && pe.id != "" {
					parent, ok = pe.id, true
				}
			}
			actor, known := v.actor(full, e)
			switch {
			case !known:
				ts.putBack(ctx, v, n.ID, "a move or rename")
			default:
				if ok && parent != n.Parent {
					if _, err := ts.m.db.Move(ctx, n.ID, actor, parent, ""); err != nil {
						ts.m.log.Warn("samba: a move was not taken", "object", n.ID, "account", actor, "err", err)
					}
				}
				if name := path.Base(e.path); name != n.Name {
					if _, err := ts.m.db.Rename(ctx, n.ID, actor, name); err != nil {
						ts.m.log.Warn("samba: a rename was not taken", "object", n.ID, "account", actor, "err", err)
					}
				}
			}
		}
		if e.id != r.Object {
			if err := writeID(full, r.Object); err == nil {
				e.id = r.Object
			}
		}
		if !e.dir && (e.inode != r.Inode || e.size != r.Size || e.mtimeNS != r.MtimeNS) {
			ts.takeContent(ctx, root, e, n, v)
		}
	}
	// What is new on disk, the folders before what is in them.
	var fresh []*entry
	for _, e := range disk {
		if !claimed[e.path] {
			fresh = append(fresh, e)
		}
	}
	sort.Slice(fresh, func(i, j int) bool {
		if depth(fresh[i].path) != depth(fresh[j].path) {
			return depth(fresh[i].path) < depth(fresh[j].path)
		}
		return fresh[i].path < fresh[j].path
	})
	children := map[string]map[string]db.ShareNode{} // parent → lower name → object without a record
	for _, n := range t.order {
		if _, had := recs[n.ID]; had || n.ID == root.ID {
			continue
		}
		if children[n.Parent] == nil {
			children[n.Parent] = map[string]db.ShareNode{}
		}
		children[n.Parent][strings.ToLower(n.Name)] = n
	}
	for _, e := range fresh {
		parent, ok := at[parentOf(e.path)]
		if !ok {
			continue // its folder did not come in
		}
		full := ts.full(root.ID, e.path)
		name := path.Base(e.path)
		// The database knows it already: an object made in the web that never reached the disk, or an ID of its own.
		if e.id != "" {
			if n, ok := t.nodes[e.id]; ok && t.paths[e.id] == e.path {
				at[e.path] = n.ID
				continue
			}
		}
		if n, ok := children[parent][strings.ToLower(name)]; ok && (n.Kind == db.KindFolder) == e.dir {
			at[e.path] = n.ID
			if writeID(full, n.ID) == nil {
				e.id = n.ID
			}
			if !e.dir {
				ts.takeContent(ctx, root, e, n, v)
			}
			continue
		}
		actor, known := v.actor(full, e)
		if !known {
			ts.leftAlone(ctx, v, root.ID, e.path)
			continue
		}
		if e.dir {
			o, err := ts.m.db.CreateFolder(ctx, accountUser(actor), name, parent)
			if err != nil {
				ts.m.log.Warn("samba: a new folder was not taken", "path", e.path, "account", actor, "err", err)
				continue
			}
			at[e.path] = o.ID
			_ = writeID(full, o.ID)
			continue
		}
		if v.openWrite[full] {
			v.busy = true
			continue
		}
		sha, size, contentType, err := ts.intoStore(full)
		if err != nil {
			ts.m.log.Warn("samba: a new file could not be read", "path", e.path, "err", err)
			continue
		}
		o, err := ts.m.db.ShareNewFile(ctx, actor, parent, name, sha, size, contentType)
		if err != nil {
			ts.m.log.Warn("samba: a new file was not taken", "path", e.path, "account", actor, "err", err)
			continue
		}
		at[e.path] = o.ID
		_ = writeID(full, o.ID)
	}
	return nil
}

// takeContent makes a version of what a file holds on disk, unless it is open for writing or holds what the database has.
func (ts *tenantShare) takeContent(ctx context.Context, root db.SharedRoot, e *entry, n db.ShareNode, v *view) {
	full := ts.full(root.ID, e.path)
	if v.openWrite[full] {
		v.busy = true
		return
	}
	sha, size, contentType, err := ts.intoStore(full)
	if err != nil {
		ts.m.log.Warn("samba: a changed file could not be read", "path", e.path, "err", err)
		return
	}
	if sha == n.SHA {
		return
	}
	actor, known := v.actor(full, e)
	if !known {
		ts.putBack(ctx, v, n.ID, "a save")
		return
	}
	if _, err := ts.m.db.ShareNewVersion(ctx, actor, n.ID, sha, size, contentType); err != nil {
		ts.m.log.Warn("samba: a save was not taken", "object", n.ID, "account", actor, "err", err)
	}
}

// breakLinks ends every hard link in a library before an ID is read or written (ADR-30, after an attack round): SMB2
// makes one with FileLinkInformation, and Samba has no setting against it. Two names of one inode share its ID; the
// pass would write the ID of a new object onto the inode of the old one and lose it. Of the names of an inode the one
// the record of its ID names stays; when none does and all names lie in this library, the first by path; every other
// name becomes a copy of its own, without an ID: a new file of whoever made the link (the audit names it, linkat).
func (ts *tenantShare) breakLinks(dir string, disk map[string]*entry, recs map[string]db.ShareRecord) {
	byInode := map[uint64][]*entry{}
	for _, e := range disk {
		if !e.dir && e.nlink > 1 {
			byInode[e.inode] = append(byInode[e.inode], e)
		}
	}
	for _, names := range byInode {
		sort.Slice(names, func(i, j int) bool { return names[i].path < names[j].path })
		keep := -1
		for i, e := range names {
			if r, ok := recs[e.id]; ok && e.id != "" && r.Path == e.path {
				keep = i
			}
		}
		if keep < 0 && uint64(len(names)) == names[0].nlink {
			keep = 0
		}
		for i, e := range names {
			if i == keep {
				continue
			}
			full := filepath.Join(dir, filepath.FromSlash(e.path))
			if err := copyOver(full); err != nil {
				ts.m.log.Warn("samba: ending a hard link", "path", e.path, "err", err)
				delete(disk, e.path) // left out of this pass, so its ID is neither read nor written
				continue
			}
			if x := stat(dir, e.path); x != nil {
				disk[e.path] = x
			} else {
				delete(disk, e.path)
			}
		}
	}
}

// dbToDisk makes the disk show the database: folders and files where they belong, with their current content, and
// nothing that is gone.
func (ts *tenantShare) dbToDisk(ctx context.Context, root db.SharedRoot, dir string, v *view) error {
	t, err := ts.tree(ctx, root.ID)
	if err != nil {
		return err
	}
	list, err := ts.m.db.ShareRecords(ctx, root.ID)
	if err != nil {
		return err
	}
	recs := map[string]db.ShareRecord{}
	for _, r := range list {
		recs[r.Object] = r
	}
	disk, err := scan(dir)
	if err != nil {
		return err
	}
	index := func() map[string]*entry {
		out := map[string]*entry{}
		for _, e := range disk {
			if e.id != "" {
				if old := out[e.id]; old == nil || recs[e.id].Path == e.path {
					out[e.id] = e
				}
			}
		}
		return out
	}
	byID := index()
	for _, n := range t.order {
		want, ok := t.paths[n.ID]
		if !ok {
			if n.Kind == db.KindFile && n.Breaks && v.report("off:"+n.ID) {
				ts.m.log.Warn("samba: a file that breaks the inheritance is kept off the share", "tenant", ts.tenant, "object", n.ID)
				_ = ts.m.db.Log(ctx, "", LogShare, n.ID, "kept off the share: it breaks the inheritance, and Samba knows rights per folder only")
			}
			continue
		}
		full := ts.full(root.ID, want)
		if n.ID == root.ID {
			continue
		}
		e := byID[n.ID]
		if e != nil && e.path != want {
			if other := disk[want]; other != nil && other.id != n.ID {
				ts.m.log.Warn("samba: a name is taken on disk", "path", want, "object", n.ID)
				continue
			}
			if err := os.Rename(ts.full(root.ID, e.path), full); err != nil {
				ts.m.log.Warn("samba: moving on disk", "from", e.path, "to", want, "err", err)
				continue
			}
			if e.dir {
				// Everything below moved with it.
				if disk, err = scan(dir); err != nil {
					return err
				}
				byID = index()
				e = byID[n.ID]
			} else {
				delete(disk, e.path)
				e.path = want
				disk[want] = e
			}
		}
		if e == nil {
			if other := disk[want]; other != nil {
				ts.m.log.Warn("samba: a name is taken on disk", "path", want, "object", n.ID)
				continue
			}
			if n.Kind == db.KindFolder {
				if err := os.Mkdir(full, 0o770); err != nil {
					ts.m.log.Warn("samba: making a folder", "path", want, "err", err)
					continue
				}
				if err := writeID(full, n.ID); err != nil {
					return err
				}
			} else if err := ts.write(full, n); err != nil {
				ts.m.log.Warn("samba: writing a file", "path", want, "err", err)
				continue
			}
			e = stat(dir, want)
			if e == nil {
				continue
			}
			disk[want] = e
		} else if n.Kind == db.KindFile && n.SHA != "" {
			r, had := recs[n.ID]
			current := ""
			if had && r.Inode == e.inode && r.Size == e.size && r.MtimeNS == e.mtimeNS {
				current = r.SHA
			} else if sha, _, err := hashFile(full); err == nil {
				current = sha
			}
			if current != n.SHA {
				if users := v.open[full]; len(users) > 0 {
					v.busy = true
					continue
				}
				if err := ts.write(full, n); err != nil {
					ts.m.log.Warn("samba: writing a file", "path", want, "err", err)
					continue
				}
				if e = stat(dir, want); e == nil {
					continue
				}
				disk[want] = e
			}
		}
		rec := db.ShareRecord{Object: n.ID, Root: root.ID, Path: want, Kind: n.Kind, Inode: e.inode, Size: e.size, MtimeNS: e.mtimeNS}
		if n.Kind == db.KindFile {
			rec.SHA = n.SHA
			// A file keeps the ACL it had where it was made: one that came to this path or is a new inode gets the ACL
			// of its folder, whoever moved or saved it, so a move never carries the readers of the folder it left.
			if old, had := recs[n.ID]; !had || old.Path != want || old.Inode != e.inode {
				v.refresh = append(v.refresh, full)
			}
		}
		if old, had := recs[n.ID]; !had || old != rec {
			if err := ts.m.db.PutShareRecord(ctx, rec); err != nil {
				return err
			}
		}
		delete(recs, n.ID)
	}
	// What the database no longer has in this library, or keeps off the disk: off the disk, if it is still what was agreed
	// on.
	var gone []string
	for id, r := range recs {
		if _, onDisk := t.paths[id]; onDisk {
			continue
		}
		gone = append(gone, id)
		e := disk[r.Path]
		if e == nil || (e.id != id && e.id != "") {
			continue
		}
		full := ts.full(root.ID, r.Path)
		if e.dir {
			if !onlyKnown(disk, r.Path, t.nodes) {
				ts.m.log.Warn("samba: a folder that is gone holds something new; it stays", "path", r.Path)
				continue
			}
			if err := os.RemoveAll(full); err != nil {
				ts.m.log.Warn("samba: removing a folder", "path", r.Path, "err", err)
			}
			continue
		}
		if len(v.open[full]) > 0 {
			v.busy = true
			gone = gone[:len(gone)-1]
			continue
		}
		if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
			ts.m.log.Warn("samba: removing a file", "path", r.Path, "err", err)
		}
	}
	if len(gone) > 0 {
		return ts.m.db.DeleteShareRecords(ctx, root.ID, gone...)
	}
	return nil
}

// onlyKnown tells whether everything below a folder on disk carries the ID of an object that is gone from the database:
// nothing in it was made on disk after the folder went.
func onlyKnown(disk map[string]*entry, dir string, alive map[string]db.ShareNode) bool {
	prefix := dir + "/"
	for p, e := range disk {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		if e.id == "" {
			return false
		}
		if _, ok := alive[e.id]; ok {
			return false
		}
	}
	return true
}

// write puts the current version of a file on disk.
func (ts *tenantShare) write(full string, n db.ShareNode) error {
	f, err := ts.m.store.Open(ts.tenant, n.SHA)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return writeAtomic(full, n.ID, f)
}

// intoStore copies a file of the working copy into the content store and returns its hash, size and type.
func (ts *tenantShare) intoStore(full string) (string, int64, string, error) {
	f, err := os.OpenFile(full, os.O_RDONLY|syscallNoFollow, 0) // #nosec G304 -- a path of the working copy
	if err != nil {
		return "", 0, "", err
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, 512)
	n, _ := f.Read(head)
	if _, err := f.Seek(0, 0); err != nil {
		return "", 0, "", err
	}
	limit := int64(1) << 62
	sha, size, err := ts.m.store.Put(ts.tenant, f, limit)
	if err != nil {
		return "", 0, "", err
	}
	return sha, size, detect(path.Base(full), head[:n]), nil
}

func depth(p string) int {
	if p == "" {
		return 0
	}
	return strings.Count(p, "/") + 1
}

// stateKey is a short form of a state, to tell whether the companion has it already.
func stateKey(s samba.State) string {
	b, _ := json.Marshal(s)
	return string(b)
}

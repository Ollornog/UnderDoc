// SPDX-License-Identifier: MIT

package db

import (
	"errors"
	"slices"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/account"
)

// The archive of spaces and the preset Members (T-34). Each test is also the attack round on the door it opens.

// TestArchivingASpace: only the owner or an administrator archives or restores a space, in the browser; archived, the
// tree leaves it out, whoever may read still reads it, and nothing in it changes through any way, for the owner too.
func TestArchivingASpace(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	inner, _ := db.CreateCollection(ctx, anna, "Projects", team.ID)
	page, _ := db.CreatePage(ctx, anna, "Minutes", inner.ID)
	other, _ := db.createShared(ctx, anna, "Elsewhere")
	for _, step := range []error{
		db.SetGrant(ctx, team.ID, admin, ToPerson, bob.ID, RoleEdit),
		db.SetGrant(ctx, team.ID, admin, ToPerson, carl.ID, RoleMembers),
	} {
		if step != nil {
			t.Fatal(step)
		}
	}
	block, err := db.AddBlock(ctx, page.ID, anna.ID, "", "Before.")
	if err != nil {
		t.Fatal(err)
	}
	trashed, _ := db.CreatePage(ctx, anna, "Old", team.ID)
	if err := db.Delete(ctx, trashed.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	// A link to edit made before, by the owner, used by dora.
	link, _, err := db.CreateLink(ctx, page.ID, Actor{ID: anna.ID}, NewLink{Mode: LinkEdit})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UseLink(ctx, link.ID, dora.ID); err != nil {
		t.Fatal(err)
	}
	if !may(t, db, ctx, page.ID, dora.ID, ActWrite) {
		t.Fatal("dora writes through the link before")
	}

	// Attacks: the editor, the holder of Members, an agent of the owner, the owner without a session, a service account
	// with the owner's ID, a collection inside, private notes.
	service := account.With(ctx, account.User{ID: anna.ID, AuthKind: account.AuthSession, Kind: account.KindService})
	for name, try := range map[string]struct {
		err  error
		want error
	}{
		"the editor":                {db.SetArchived(asPerson(bob), team.ID, Actor{ID: bob.ID}, true), ErrArchiver},
		"the holder of Members":     {db.SetArchived(asPerson(carl), team.ID, Actor{ID: carl.ID}, true), ErrArchiver},
		"an agent of the owner":     {db.SetArchived(asAgent(anna), team.ID, Actor{ID: anna.ID}, true), ErrAgent},
		"an agent of an admin":      {db.SetArchived(asAgent(carl), team.ID, Actor{ID: carl.ID, Admin: true}, true), ErrAgent},
		"the owner without session": {db.SetArchived(ctx, team.ID, Actor{ID: anna.ID}, true), ErrSession},
		"a service account":         {db.SetArchived(service, team.ID, Actor{ID: anna.ID}, true), ErrAgent},
		"a session of someone else": {db.SetArchived(asPerson(bob), team.ID, Actor{ID: anna.ID}, true), ErrSession},
		"a collection inside":       {db.SetArchived(asPerson(anna), inner.ID, Actor{ID: anna.ID}, true), ErrArchivePlace},
		"a page":                    {db.SetArchived(asPerson(anna), page.ID, Actor{ID: anna.ID}, true), ErrArchivePlace},
	} {
		if !errors.Is(try.err, try.want) {
			t.Errorf("%s archives: %v, want %v", name, try.err, try.want)
		}
	}
	notes, _ := db.Collections(ctx, anna.ID)
	if err := db.SetArchived(asPerson(anna), notes[0].ID, Actor{ID: anna.ID}, true); !errors.Is(err, ErrArchivePlace) {
		t.Errorf("private notes are archived: %v", err)
	}
	if ok, _ := db.MayArchive(asPerson(carl), team.ID, Actor{ID: carl.ID}); ok {
		t.Error("the workspace offers the holder of Members to archive")
	}
	if ok, _ := db.MayArchive(asPerson(anna), team.ID, Actor{ID: anna.ID}); !ok {
		t.Error("the workspace does not offer the owner to archive")
	}

	if err := db.SetArchived(asPerson(anna), team.ID, Actor{ID: anna.ID}, true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetArchived(asPerson(anna), team.ID, Actor{ID: anna.ID}, true); err != nil {
		t.Errorf("archived twice: %v", err)
	}

	// The tree leaves it out, the archive lists it for whoever reads it, and an address still opens it.
	for _, who := range []string{anna.ID, bob.ID} {
		top, _ := db.Collections(ctx, who)
		if slices.ContainsFunc(top, func(o Object) bool { return o.ID == team.ID }) {
			t.Errorf("%s sees the archived space in the tree", who)
		}
		archive, _ := db.ArchivedSpaces(ctx, who)
		if !slices.ContainsFunc(archive, func(o Object) bool { return o.ID == team.ID && o.Archived }) {
			t.Errorf("%s does not find the space in the archive: %v", who, titles(archive))
		}
		if _, err := db.Object(ctx, page.ID, who); err != nil {
			t.Errorf("%s does not read a page of the archive: %v", who, err)
		}
	}
	if archive, _ := db.ArchivedSpaces(ctx, dora.ID); len(archive) != 0 {
		t.Errorf("the archive shows dora what she may not read: %v", titles(archive))
	}
	if in, _ := db.InArchive(ctx, page.ID); !in {
		t.Error("a page inside is not in the archive")
	}

	// Read only, for the owner too; manage stays.
	for _, who := range []string{anna.ID, bob.ID, carl.ID, dora.ID} {
		for _, act := range []string{ActComment, ActCreate, ActWrite, ActDelete, ActShare} {
			if may(t, db, ctx, page.ID, who, act) {
				t.Errorf("%s may %s in the archive", who, act)
			}
		}
	}
	if !may(t, db, ctx, page.ID, anna.ID, ActRead) || !may(t, db, ctx, team.ID, anna.ID, ActManage) || !may(t, db, ctx, page.ID, bob.ID, ActRead) {
		t.Error("reading or managing the archive is refused")
	}
	if d, _ := db.Decide(ctx, page.ID, anna.ID, ActWrite, false); d.Via != ViaArchived || d.Object != team.ID {
		t.Errorf("the access check names the archive: %+v", d)
	}
	if role, _ := db.Role(ctx, page.ID, anna.ID); role != RoleRead {
		t.Errorf("the owner's role in the archive: %q", role)
	}

	// Every way that changes something is refused.
	refused := map[string]error{}
	_, refused["a new page"] = db.CreatePage(ctx, anna, "New", team.ID)
	_, refused["a new folder"] = db.CreateFolder(ctx, anna, "New", team.ID)
	_, refused["a new collection"] = db.CreateCollection(ctx, anna, "New", team.ID)
	_, refused["renaming"] = db.Rename(ctx, page.ID, anna.ID, "Renamed")
	_, refused["a new block"] = db.AddBlock(ctx, page.ID, anna.ID, "", "After.")
	_, refused["writing a block"] = db.EditBlock(ctx, page.ID, block.ID, anna.ID, "Changed.")
	_, refused["a comment"] = db.AddComment(ctx, page.ID, bob.ID, "", "Note.", 0)
	refused["deleting a page"] = db.Delete(ctx, page.ID, anna.ID)
	refused["deleting the space"] = db.Delete(ctx, team.ID, anna.ID)
	_, refused["moving out"] = db.Move(ctx, page.ID, anna.ID, other.ID, "")
	_, refused["moving in"] = db.Move(ctx, other.ID, anna.ID, team.ID, "")
	_, refused["moving the space"] = db.Move(ctx, team.ID, anna.ID, other.ID, "")
	_, refused["copying in"] = db.Copy(ctx, other.ID, anna.ID, team.ID, "")
	_, refused["restoring from the trash"] = db.Restore(ctx, trashed.ID, anna.ID)
	_, _, refused["a new link"] = db.CreateLink(ctx, page.ID, Actor{ID: anna.ID}, NewLink{Mode: LinkComment})
	for name, err := range refused {
		if !isNotAllowed(err) {
			t.Errorf("%s in the archive: %v", name, err)
		}
	}
	if !errors.Is(refused["a new page"], ErrArchived) {
		t.Errorf("the refusal does not say why: %v", refused["a new page"])
	}
	if may(t, db, ctx, page.ID, dora.ID, ActWrite) || may(t, db, ctx, page.ID, dora.ID, ActRead) {
		t.Error("a link lends a role in the archive")
	}
	// Copying out needs only read here and create there.
	if _, err := db.Copy(ctx, page.ID, anna.ID, other.ID, ""); err != nil {
		t.Errorf("copying out of the archive: %v", err)
	}
	// Rights stay manageable: the owner lets dora read; nobody gives write in the archive but an administrator.
	if err := db.SetGrant(ctx, team.ID, Actor{ID: anna.ID}, ToPerson, dora.ID, RoleRead); err != nil {
		t.Errorf("the owner grants read in the archive: %v", err)
	}
	if err := db.SetGrant(ctx, team.ID, Actor{ID: anna.ID}, ToPerson, dora.ID, RoleEdit); !errors.Is(err, ErrWiden) {
		t.Errorf("the owner gives edit in the archive: %v", err)
	}

	// Restoring: the editor and the holder of Members may not, the owner may; an administrator archives and restores
	// a space they may not read.
	if err := db.SetArchived(asPerson(bob), team.ID, Actor{ID: bob.ID}, false); !errors.Is(err, ErrArchiver) {
		t.Errorf("the editor restores: %v", err)
	}
	if err := db.SetArchived(asPerson(carl), team.ID, Actor{ID: carl.ID}, false); !errors.Is(err, ErrArchiver) {
		t.Errorf("the holder of Members restores: %v", err)
	}
	if err := db.SetArchived(asAgent(anna), team.ID, Actor{ID: anna.ID}, false); !errors.Is(err, ErrAgent) {
		t.Errorf("an agent restores: %v", err)
	}
	if err := db.SetArchived(asPerson(anna), team.ID, Actor{ID: anna.ID}, false); err != nil {
		t.Fatal(err)
	}
	if !may(t, db, ctx, page.ID, bob.ID, ActWrite) || !may(t, db, ctx, page.ID, dora.ID, ActWrite) {
		t.Error("restoring does not bring writing and the link back")
	}
	top, _ := db.Collections(ctx, bob.ID)
	if !slices.ContainsFunc(top, func(o Object) bool { return o.ID == team.ID }) {
		t.Error("the restored space is not in the tree")
	}
	hidden, _ := db.CreateCollection(ctx, bob, "Bob's", "")
	if err := db.SetArchived(asPerson(dora), hidden.ID, Actor{ID: dora.ID, Admin: true}, true); err != nil {
		t.Errorf("an administrator archives a space she may not read: %v", err)
	}
	if err := db.SetArchived(asPerson(dora), hidden.ID, Actor{ID: dora.ID, Admin: true}, false); err != nil {
		t.Errorf("an administrator restores a space she may not read: %v", err)
	}
	// The database keeps the archive at the top, whatever a change does.
	if _, err := db.pool.Exec(ctx, "UPDATE objects SET archived = now() WHERE id = $1", inner.ID); err == nil {
		t.Error("a collection inside a space is archived in the database")
	}
}

// TestArchivedFilesAndTheShare: an archived collection of Files is read only on the share and for uploads; the folder
// of the workspace is never archived.
func TestArchivedFilesAndTheShare(t *testing.T) {
	db, ctx := setup(t)
	library, err := db.createSharedFiles(ctx, anna, "Library")
	if err != nil {
		t.Fatal(err)
	}
	started, err := db.StartUpload(ctx, bob.ID, UploadTarget{Parent: library.ID}, "before.txt", 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Received(ctx, started.ID, bob.ID, 0, 3); err != nil {
		t.Fatal(err)
	}
	if err := db.SetArchived(asPerson(anna), library.ID, Actor{ID: anna.ID}, true); err != nil {
		t.Fatal(err)
	}
	roots, _ := db.FileRoots(ctx, bob.ID)
	if slices.ContainsFunc(roots, func(r Root) bool { return r.ID == library.ID }) {
		t.Error("the archived library is among the roots")
	}
	archived, _ := db.ArchivedFileRoots(ctx, bob.ID)
	if !slices.ContainsFunc(archived, func(r Root) bool { return r.ID == library.ID && r.Archived }) {
		t.Error("the archived library is not in the archive")
	}
	access, err := db.FolderAccess(ctx, library.ID, []string{anna.ID, bob.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, who := range []string{anna.ID, bob.ID} {
		if got := access[library.ID][who]; got != 1 {
			t.Errorf("the share gives %s rank %d in the archive, want read only", who, got)
		}
	}
	if _, err := db.ShareNewFile(ctx, bob.ID, library.ID, "new.txt", sha(4), 3, "text/plain"); !errors.Is(err, ErrArchived) {
		t.Errorf("a new file over the share in the archive: %v", err)
	}
	if _, err := db.StartUpload(ctx, bob.ID, UploadTarget{Parent: library.ID}, "new.txt", 3); !errors.Is(err, ErrArchived) {
		t.Errorf("an upload into the archive: %v", err)
	}
	if _, err := db.FinishUpload(ctx, started.ID, bob.ID, sha(3), 3, "text/plain", func() error { return nil }); !isNotAllowed(err) {
		t.Errorf("an upload started before the archive ends in it: %v", err)
	}
	if err := db.EnsureRoots(ctx, anna, true, ""); err != nil {
		t.Fatal(err)
	}
	all, _ := db.FileRoots(ctx, anna.ID)
	for _, r := range all {
		if r.Role == RootTenant || r.Role == RootUser {
			if err := db.SetArchived(asPerson(anna), r.ID, Actor{ID: anna.ID, Admin: true}, true); !errors.Is(err, ErrArchivePlace) && !errors.Is(err, ErrNotFound) {
				t.Errorf("the folder %s is archived: %v", r.Role, err)
			}
		}
	}
}

// TestMembersPreset: Members manages members and rights without delete, and never opens up what it does not hold:
// not delete, not through the tenant, not through a link, not by restoring the inheritance.
func TestMembersPreset(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	folder, _ := db.CreateFolder(ctx, anna, "Plans", team.ID)
	page, _ := db.CreatePage(ctx, anna, "Plan", folder.ID)
	if err := db.SetGrant(ctx, team.ID, admin, ToPerson, carl.ID, RoleMembers); err != nil {
		t.Fatal(err)
	}
	rights, _ := db.RightsOf(ctx, team.ID, admin)
	if i := slices.IndexFunc(rights.Own, func(g Grant) bool { return g.Who == carl.ID }); i < 0 || rights.Own[i].Role != RoleMembers {
		t.Errorf("carl's entries are not the preset Members: %+v", rights.Own)
	}
	for _, act := range []string{ActRead, ActComment, ActCreate, ActWrite, ActShare, ActManage} {
		if !may(t, db, ctx, page.ID, carl.ID, act) {
			t.Errorf("Members may not %s", act)
		}
	}
	if may(t, db, ctx, page.ID, carl.ID, ActDelete) {
		t.Error("Members deletes")
	}
	if err := db.Delete(ctx, page.ID, carl.ID); !isNotAllowed(err) {
		t.Errorf("Members moves a page to the trash: %v", err)
	}
	if role, _ := db.Role(ctx, page.ID, carl.ID); role != RoleEdit {
		t.Errorf("the workspace shows Members as %q, want edit", role)
	}

	members := Actor{ID: carl.ID}
	// What Members holds, it gives and takes back.
	for _, role := range []string{RoleRead, RoleComment, RoleMembers, RoleNone} {
		if err := db.SetGrant(ctx, team.ID, members, ToPerson, bob.ID, role); err != nil {
			t.Errorf("Members gives bob %q: %v", role, err)
		}
	}
	if err := db.SetEntries(ctx, folder.ID, members, ToPerson, bob.ID, entries(ActRead, Allow, ActDelete, Deny), true); err != nil {
		t.Errorf("Members denies delete: %v", err)
	}
	// Attacks: give delete to someone, to oneself, to the tenant; take a deny of delete back; restore the inheritance
	// below a break; make a link to edit.
	for name, err := range map[string]error{
		"edit for bob":           db.SetGrant(ctx, team.ID, members, ToPerson, bob.ID, RoleEdit),
		"manage for bob":         db.SetGrant(ctx, team.ID, members, ToPerson, bob.ID, RoleManage),
		"delete for himself":     db.SetEntries(ctx, team.ID, members, ToPerson, carl.ID, entries(ActRead, Allow, ActManage, Allow, ActDelete, Allow), true),
		"edit for everyone":      db.SetGrant(ctx, team.ID, members, ToTenant, "", RoleEdit),
		"the deny of delete off": db.SetEntries(ctx, folder.ID, members, ToPerson, bob.ID, entries(ActRead, Allow), true),
	} {
		if !errors.Is(err, ErrWiden) {
			t.Errorf("Members gives %s: %v", name, err)
		}
	}
	if _, err := db.SetAccess(ctx, folder.ID, members, RoleEdit); !errors.Is(err, ErrWiden) {
		t.Errorf("Members sets the level edit: %v", err)
	}
	if err := db.SetInherit(ctx, folder.ID, members, false); err != nil {
		t.Errorf("Members breaks the inheritance: %v", err)
	}
	if err := db.SetInherit(ctx, folder.ID, members, true); !errors.Is(err, ErrWiden) {
		t.Errorf("Members restores the inheritance: %v", err)
	}
	if _, _, err := db.CreateLink(ctx, page.ID, members, NewLink{Mode: LinkEdit}); !isNotAllowed(err) {
		t.Errorf("Members makes a link to edit: %v", err)
	}
	if err := db.SetArchived(asPerson(carl), team.ID, members, true); !errors.Is(err, ErrArchiver) {
		t.Errorf("Members archives: %v", err)
	}
	// The owner and administrators give what they like.
	if err := db.SetGrant(ctx, team.ID, Actor{ID: anna.ID}, ToPerson, bob.ID, RoleEdit); err != nil {
		t.Errorf("the owner gives edit: %v", err)
	}
	if err := db.SetInherit(ctx, folder.ID, admin, true); err != nil {
		t.Errorf("an administrator restores the inheritance: %v", err)
	}

	// A link lends only what its maker still holds: carl makes a link to edit while he manages, then holds Members.
	if err := db.SetGrant(ctx, team.ID, admin, ToPerson, carl.ID, RoleManage); err != nil {
		t.Fatal(err)
	}
	link, _, err := db.CreateLink(ctx, page.ID, members, NewLink{Mode: LinkEdit})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UseLink(ctx, link.ID, dora.ID); err != nil {
		t.Fatal(err)
	}
	if !may(t, db, ctx, page.ID, dora.ID, ActDelete) {
		t.Fatal("dora deletes through the link while carl manages")
	}
	if err := db.SetGrant(ctx, team.ID, admin, ToPerson, carl.ID, RoleMembers); err != nil {
		t.Fatal(err)
	}
	if may(t, db, ctx, page.ID, dora.ID, ActDelete) {
		t.Error("the link lends delete its maker no longer holds")
	}
	if !may(t, db, ctx, page.ID, dora.ID, ActWrite) {
		t.Error("the link stops lending what its maker still holds")
	}
	// The database names the preset too, for the level of the whole tenant.
	if err := db.SetGrant(ctx, folder.ID, admin, ToTenant, "", RoleMembers); err != nil {
		t.Fatal(err)
	}
	if o, _ := db.Object(ctx, folder.ID, anna.ID); o.Access != RoleMembers {
		t.Errorf("the tenant's preset on Plans: %q", o.Access)
	}
}

// TestImportingTheArchive: an import brings archived spaces and groups as they were, only without a request (T-28).
func TestImportingTheArchive(t *testing.T) {
	db, ctx := setup(t)
	done, err := db.Import(ctx, anna.Login, "Old wiki", &Node{Object: Object{Kind: KindCollection, Archived: true},
		Kids: []*Node{{Object: Object{Kind: KindPage, Title: "Old page", Archived: true}}}})
	if err != nil {
		t.Fatal(err)
	}
	if !done.Top.Archived {
		t.Error("the imported space is not archived")
	}
	if archive, _ := db.ArchivedSpaces(ctx, anna.ID); !slices.ContainsFunc(archive, func(o Object) bool { return o.ID == done.Top.ID }) {
		t.Error("the imported space is not in the archive")
	}
	plain, _ := db.CreateCollection(ctx, anna, "Plain", "")
	if err := db.ArchiveAsOperator(asPerson(anna), plain.ID); !errors.Is(err, ErrSession) {
		t.Errorf("a request archives as the operator: %v", err)
	}
	if err := db.ArchiveAsOperator(ctx, plain.ID); err != nil {
		t.Errorf("the operator archives: %v", err)
	}
	g, err := db.ImportGroup(ctx, "Former team", "", "", true)
	if err != nil || !g.Archived {
		t.Errorf("an archived group from an import: %+v %v", g, err)
	}
	if _, err := db.ImportGroup(asPerson(anna), "Other", "", "", true); !errors.Is(err, ErrSession) {
		t.Errorf("a request imports a group: %v", err)
	}
	if _, err := db.ImportGroup(ctx, "Inside", "", g.ID, false); !errors.Is(err, ErrGroupArchived) && err == nil {
		t.Errorf("a group goes inside an archived one: %v", err)
	}
	if _, err := db.ImportGroup(ctx, "Archived inside", "", g.ID, true); !errors.Is(err, ErrGroupArchived) {
		t.Errorf("an archived group goes inside one: %v", err)
	}
}

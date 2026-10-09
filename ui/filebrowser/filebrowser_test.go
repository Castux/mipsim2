package filebrowser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// realLister lists a folder of the real filesystem, as platform.ListDir does.
func realLister(dir string, done func([]Entry, error)) {
	des, err := os.ReadDir(dir)
	if err != nil {
		done(nil, err)
		return
	}
	var out []Entry
	for _, d := range des {
		out = append(out, Entry{Name: d.Name(), Dir: d.IsDir()})
	}
	done(out, nil)
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func setup(t *testing.T) string {
	root := t.TempDir()
	for _, p := range []string{"b.mip", "A.mip", "notes.txt", "t.fix", ".hidden.mip", "sub/c.mip", "Zed/x.mip"} {
		full := filepath.Join(root, p)
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte("{}"), 0o644)
	}
	return root
}

func names(b *Browser) string {
	var s []string
	for _, e := range b.Entries {
		if e.Dir {
			s = append(s, e.Name+"/")
		} else {
			s = append(s, e.Name)
		}
	}
	return strings.Join(s, " ")
}

func TestListingAndNavigation(t *testing.T) {
	root := setup(t)
	b := New(Open, filepath.Join(root, "b.mip"), realLister, exists)
	if got, want := names(b), "../ sub/ Zed/ A.mip b.mip"; got != want {
		t.Fatalf("entries %q, want %q", got, want)
	}
	if b.Selected != 4 {
		t.Errorf("start file not selected: %d", b.Selected)
	}
	b.Click(1) // sub
	b.Click(1) // again: enter it
	if b.Dir != filepath.Join(root, "sub") || names(b) != "../ c.mip" {
		t.Fatalf("in %s: %q", b.Dir, names(b))
	}
	b.Parent()
	if b.Dir != root || b.Entries[b.Selected].Name != "sub" {
		t.Errorf("parent: %s, selected %d", b.Dir, b.Selected)
	}
	b.Name = "Zed/"
	if _, done := b.Confirm(); done || b.Dir != filepath.Join(root, "Zed") {
		t.Errorf("typing a folder did not navigate: %s", b.Dir)
	}
}

func TestOpenChoosesExistingFile(t *testing.T) {
	root := setup(t)
	b := New(Open, root+string(filepath.Separator), realLister, exists)
	b.Move(5) // from no selection to the fifth entry: b.mip
	if b.Name != "b.mip" {
		t.Fatalf("name %q", b.Name)
	}
	path, done := b.Confirm()
	if !done || path != filepath.Join(root, "b.mip") {
		t.Errorf("confirm = %q %v", path, done)
	}
	b.Name = "missing.mip"
	if _, done := b.Confirm(); done || !strings.Contains(b.Err, "does not exist") {
		t.Errorf("opening a missing file: done %v, err %q", done, b.Err)
	}
}

func TestSaveAddsExtensionAndConfirmsOverwrite(t *testing.T) {
	root := setup(t)
	b := New(Save, filepath.Join(root, "new.mip"), realLister, exists)
	if b.Name != "new.mip" {
		t.Fatalf("name field %q", b.Name)
	}
	b.Name = "fresh"
	if path, done := b.Confirm(); !done || path != filepath.Join(root, "fresh.mip") {
		t.Errorf("save fresh: %q %v", path, done)
	}
	b.Name = "A.mip"
	if _, done := b.Confirm(); done || !strings.Contains(b.Err, "exists") {
		t.Fatalf("overwrite not confirmed first: %v %q", done, b.Err)
	}
	if path, done := b.Confirm(); !done || path != filepath.Join(root, "A.mip") {
		t.Errorf("second confirm: %q %v", path, done)
	}
}

func TestSaveListsOnlyDocuments(t *testing.T) {
	root := setup(t)
	b := New(Save, filepath.Join(root, "x.mip"), realLister, exists)
	if got, want := names(b), "../ sub/ Zed/ A.mip b.mip"; got != want {
		t.Errorf("save entries %q, want %q", got, want)
	}
}

func TestPickListsEveryFile(t *testing.T) {
	root := setup(t)
	b := New(Pick, filepath.Join(root, "notes.txt"), realLister, exists)
	if got, want := names(b), "../ sub/ Zed/ A.mip b.mip notes.txt t.fix"; got != want {
		t.Errorf("pick entries %q, want %q", got, want)
	}
	if b.Selected < 0 || b.Entries[b.Selected].Name != "notes.txt" {
		t.Errorf("start file not selected: %d", b.Selected)
	}
	if path, done := b.Confirm(); !done || path != filepath.Join(root, "notes.txt") {
		t.Errorf("confirm %q %v", path, done)
	}
}

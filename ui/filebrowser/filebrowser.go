// Package filebrowser is the logic of the in-app Open and Save As dialog:
// the current folder, its entries, the selection and the filename field. It
// has no graphics; the ui draws it and feeds it keys and clicks. Listing a
// folder goes through a function supplied by the caller (platform.ListDir),
// so file access stays in the platform layer.
package filebrowser

import (
	"path/filepath"
	"slices"
	"strings"
)

// Mode is what the dialog is for.
type Mode int

const (
	Open Mode = iota
	Save
	Pick // choose any existing file, such as a memory's init file
)

func (m Mode) String() string { return [...]string{"Open", "Save as", "Choose file"}[m] }

// Entry is one item in a folder.
type Entry struct {
	Name string
	Dir  bool
}

// Lister lists a folder and calls done with its entries.
type Lister func(dir string, done func([]Entry, error))

// Ext is the extension of files the dialog saves.
const Ext = ".mip"

// FixtureExt is the extension of test fixtures, which Open also lists: they
// load like documents.
const FixtureExt = ".fix"

// Browser is the dialog's state.
type Browser struct {
	Mode     Mode
	Dir      string
	Entries  []Entry // ".." first when there is a parent, then folders, then .mip files
	Selected int     // index into Entries, or -1
	Name     string  // the filename field
	Err      string  // a problem to show, cleared by the next action
	Scroll   int     // first visible entry, kept by the ui

	list     Lister
	confirm  string // a path that exists, awaiting a second confirm to overwrite
	statFile func(path string) bool
}

// New opens a browser in the folder of start (a file or a folder).
// exists reports whether a file exists, for the overwrite check.
func New(mode Mode, start string, list Lister, exists func(string) bool) *Browser {
	b := &Browser{Mode: mode, list: list, statFile: exists, Selected: -1}
	dir, name := start, ""
	if filepath.Ext(start) != "" {
		dir, name = filepath.Dir(start), filepath.Base(start)
	}
	if dir == "" {
		dir = "."
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if mode == Save {
		b.Name = name
	}
	b.chdir(dir)
	if mode != Save && name != "" {
		b.selectName(name)
	}
	return b
}

func (b *Browser) chdir(dir string) {
	b.list(dir, func(entries []Entry, err error) {
		if err != nil {
			b.Err = err.Error()
			return
		}
		var dirs, files []Entry
		for _, e := range entries {
			switch {
			case strings.HasPrefix(e.Name, "."):
			case e.Dir:
				dirs = append(dirs, e)
			case b.shows(e.Name):
				files = append(files, e)
			}
		}
		byName := func(a, b Entry) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) }
		slices.SortFunc(dirs, byName)
		slices.SortFunc(files, byName)
		b.Entries = b.Entries[:0]
		if parent := filepath.Dir(dir); parent != dir {
			b.Entries = append(b.Entries, Entry{Name: "..", Dir: true})
		}
		b.Entries = append(append(b.Entries, dirs...), files...)
		b.Dir = dir
		b.Selected = -1
		b.Scroll = 0
		b.Err = ""
		b.confirm = ""
	})
}

func (b *Browser) selectName(name string) {
	for i, e := range b.Entries {
		if e.Name == name {
			b.Selected = i
		}
	}
}

// Move moves the selection by delta entries.
func (b *Browser) Move(delta int) {
	if len(b.Entries) == 0 {
		return
	}
	b.Selected = max(0, min(len(b.Entries)-1, b.Selected+delta))
	if e := b.Entries[b.Selected]; !e.Dir {
		b.Name = e.Name
	}
	b.confirm = ""
}

// Click selects entry i; a click on the selected entry activates it (like a
// double click).
func (b *Browser) Click(i int) (path string, done bool) {
	if i < 0 || i >= len(b.Entries) {
		return "", false
	}
	if i == b.Selected {
		return b.activate(i)
	}
	b.Selected = i
	if e := b.Entries[i]; !e.Dir {
		b.Name = e.Name
	}
	b.confirm = ""
	return "", false
}

// TypeRune adds a character to the filename field.
func (b *Browser) TypeRune(r rune) {
	if r >= ' ' && r != 127 {
		b.Name += string(r)
		b.confirm = ""
	}
}

// Backspace removes the last character of the filename field.
func (b *Browser) Backspace() {
	if r := []rune(b.Name); len(r) > 0 {
		b.Name = string(r[:len(r)-1])
	}
	b.confirm = ""
}

// Parent goes to the parent folder.
func (b *Browser) Parent() {
	if parent := filepath.Dir(b.Dir); parent != b.Dir {
		name := filepath.Base(b.Dir)
		b.chdir(parent)
		b.selectName(name)
	}
}

// Confirm acts on the filename field, or else the selected entry: it enters
// folders and returns the chosen file path when the dialog is done. Saving
// over an existing file needs a second Confirm.
func (b *Browser) Confirm() (path string, done bool) {
	name := strings.TrimSpace(b.Name)
	if name == "" {
		if b.Selected >= 0 {
			return b.activate(b.Selected)
		}
		b.Err = "type a file name or pick a file"
		return "", false
	}
	if !filepath.IsAbs(name) {
		name = filepath.Join(b.Dir, name)
	}
	// A typed folder (or drive) navigates.
	for _, e := range b.Entries {
		if e.Dir && filepath.Join(b.Dir, e.Name) == filepath.Clean(name) {
			b.Name = ""
			b.chdir(filepath.Clean(name))
			return "", false
		}
	}
	if strings.HasSuffix(b.Name, "/") || strings.HasSuffix(b.Name, `\`) || (filepath.IsAbs(strings.TrimSpace(b.Name)) && filepath.Ext(name) == "") {
		b.Name = ""
		b.chdir(filepath.Clean(name))
		return "", false
	}
	return b.choose(name)
}

func (b *Browser) activate(i int) (string, bool) {
	e := b.Entries[i]
	if e.Dir {
		if e.Name == ".." {
			b.Parent()
		} else {
			b.Name = ""
			b.chdir(filepath.Join(b.Dir, e.Name))
		}
		return "", false
	}
	return b.choose(filepath.Join(b.Dir, e.Name))
}

func (b *Browser) choose(path string) (string, bool) {
	if b.Mode == Save {
		if filepath.Ext(path) == "" {
			path += Ext
		}
		if b.statFile != nil && b.statFile(path) && b.confirm != path {
			b.confirm = path
			b.Err = filepath.Base(path) + " exists: confirm again to replace it"
			return "", false
		}
		return path, true
	}
	if b.statFile != nil && !b.statFile(path) {
		b.Err = filepath.Base(path) + " does not exist"
		return "", false
	}
	return path, true
}

// shows reports whether a file is listed: .mip files, and in Open mode also
// .fix fixtures. Pick lists every file.
func (b *Browser) shows(name string) bool {
	if b.Mode == Pick {
		return true
	}
	ext := strings.ToLower(filepath.Ext(name))
	return ext == Ext || b.Mode == Open && ext == FixtureExt
}

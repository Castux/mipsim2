// Package platform hides file access behind an asynchronous interface, with a
// native implementation now and a browser one in M10. Callers never block on
// it: results arrive through callbacks, which on native builds run before the
// function returns and in the browser will run on a later frame.
package platform

// WriteFile stores data under name (a path on desktop) and calls done with
// the result.
func WriteFile(name string, data []byte, done func(error)) {
	writeFile(name, data, done)
}

// ReadFile loads the file called name (a path on desktop) and calls done
// with its content.
func ReadFile(name string, done func([]byte, error)) {
	readFile(name, done)
}

// DirEntry is one item of a folder listing.
type DirEntry struct {
	Name string
	Dir  bool
}

// ListDir lists a folder and calls done with its entries.
func ListDir(dir string, done func([]DirEntry, error)) {
	listDir(dir, done)
}

// Exists reports whether a file exists. It is false where it cannot be
// known synchronously (the browser).
func Exists(path string) bool {
	return exists(path)
}

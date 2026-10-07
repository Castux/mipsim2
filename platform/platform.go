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

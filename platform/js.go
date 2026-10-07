//go:build js

package platform

import "errors"

var errWeb = errors.New("files in the browser come with the web port (M10)")

func writeFile(name string, data []byte, done func(error)) { done(errWeb) }

func readFile(name string, done func([]byte, error)) { done(nil, errWeb) }

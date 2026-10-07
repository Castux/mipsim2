//go:build js

package platform

import "errors"

func writeFile(name string, data []byte, done func(error)) {
	done(errors.New("saving files in the browser comes with the web port (M10)"))
}

//go:build !js

package platform

import "os"

func writeFile(name string, data []byte, done func(error)) {
	done(os.WriteFile(name, data, 0o644))
}

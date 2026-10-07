//go:build !js

package platform

import "os"

func writeFile(name string, data []byte, done func(error)) {
	done(os.WriteFile(name, data, 0o644))
}

func readFile(name string, done func([]byte, error)) {
	done(os.ReadFile(name))
}

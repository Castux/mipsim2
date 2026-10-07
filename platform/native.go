//go:build !js

package platform

import "os"

func writeFile(name string, data []byte, done func(error)) {
	done(os.WriteFile(name, data, 0o644))
}

func readFile(name string, done func([]byte, error)) {
	done(os.ReadFile(name))
}

func listDir(dir string, done func([]DirEntry, error)) {
	des, err := os.ReadDir(dir)
	if err != nil {
		done(nil, err)
		return
	}
	out := make([]DirEntry, 0, len(des))
	for _, d := range des {
		isDir := d.IsDir()
		if d.Type()&os.ModeSymlink != 0 {
			if info, err := os.Stat(dir + string(os.PathSeparator) + d.Name()); err == nil {
				isDir = info.IsDir()
			}
		}
		out = append(out, DirEntry{Name: d.Name(), Dir: isDir})
	}
	done(out, nil)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

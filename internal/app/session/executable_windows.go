package session

import "os"

// executable reports whether this process could run the file at p.
//
// Windows has no execute bit and no access(2): what makes a file runnable is
// its extension, and exec.LookPath already applies PATHEXT. So the only thing
// left to establish here is that the file is there and is a file.
func executable(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

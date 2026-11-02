package watch

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// replaysPath is where CS2 saves downloaded demos inside a Steam library.
var replaysPath = filepath.Join("steamapps", "common", "Counter-Strike Global Offensive", "game", "csgo", "replays")

// libraryPath matches the "path" entries of Steam's libraryfolders.vdf.
var libraryPath = regexp.MustCompile(`"path"\s+"([^"]+)"`)

// FindReplays returns the CS2 replays folder of the first Steam library that
// has one, or "" if none is found.
func FindReplays() string {
	for _, steam := range steamRoots() {
		libs := []string{steam}
		if vdf, err := os.ReadFile(filepath.Join(steam, "steamapps", "libraryfolders.vdf")); err == nil {
			for _, m := range libraryPath.FindAllSubmatch(vdf, -1) {
				// VDF escapes backslashes in Windows paths.
				libs = append(libs, filepath.Clean(strings.ReplaceAll(string(m[1]), `\\`, `\`)))
			}
		}
		for _, lib := range libs {
			dir := filepath.Join(lib, replaysPath)
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				return dir
			}
		}
	}
	return ""
}

func steamRoots() []string {
	if runtime.GOOS == "windows" {
		var roots []string
		for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
			if p := os.Getenv(env); p != "" {
				roots = append(roots, filepath.Join(p, "Steam"))
			}
		}
		return roots
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, ".steam", "steam"),
		filepath.Join(home, ".local", "share", "Steam"),
		filepath.Join(home, "Library", "Application Support", "Steam"),
	}
}

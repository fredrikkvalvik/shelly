// Package project finds per directory tool definitions and works out what a
// shell needs to load or drop as you move between them.
package project

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// the directory a project keeps its tools in
const DirName = ".shelly"

type Tool struct {
	Name string
	File string // absolute path to the config
}

// one .shelly directory found by the walk
type Dir struct {
	Path  string // the .shelly directory itself
	Tools []Tool
}

// Discover walks up from cwd collecting .shelly directories, outermost first
// so a nearer definition can override one further out.
//
// The walk stops after $HOME when home is an ancestor of cwd, and at the
// filesystem root otherwise, so a project outside your home directory still
// works without the walk being unbounded in the common case.
func Discover(cwd, home string) []Dir {
	cwd = filepath.Clean(cwd)
	home = filepath.Clean(home)

	var dirs []Dir
	for d := cwd; ; {
		if tools, ok := read(filepath.Join(d, DirName)); ok {
			dirs = append(dirs, Dir{Path: filepath.Join(d, DirName), Tools: tools})
		}
		if d == home {
			break
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}

	slices.Reverse(dirs)
	return dirs
}

// read lists the tools in a .shelly directory. Only the filenames are read:
// this runs on every prompt, and parsing would put the cost of every project
// config on every command you type.
func read(path string) ([]Tool, bool) {
	entries, err := os.ReadDir(path)
	if err != nil {
		// missing or unreadable is not worth breaking a prompt over
		return nil, false
	}

	var tools []Tool
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".toml" {
			continue
		}
		tools = append(tools, Tool{
			Name: strings.TrimSuffix(e.Name(), ".toml"),
			File: filepath.Join(path, e.Name()),
		})
	}

	slices.SortFunc(tools, func(a, b Tool) int { return strings.Compare(a.Name, b.Name) })
	return tools, true
}

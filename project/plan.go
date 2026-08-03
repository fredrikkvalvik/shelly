package project

import (
	"path/filepath"
	"slices"
	"strings"
)

// what a shell must do to catch up with the current directory
type Plan struct {
	Unload  []string // tool names whose definitions must go
	Load    []Tool   // tools to define, after the unloads
	Loaded  []string // absolute config paths, for the shell to remember
	Notices []string // lines for the user, e.g. a directory that is not trusted
}

// reports whether a discovered directory may be loaded, and why not if it may
// not. Passing this in keeps the planner testable and, until the trust store
// exists, keeps the gate closed at the one place that matters.
type TrustFunc func(Dir) (bool, string)

// Build works out the delta between what a shell has loaded and what the
// current directory calls for.
//
// Precedence runs outermost to innermost: a global tool is overridden by
// ~/.shelly, which is overridden by a project's own. Because the desired set
// is recomputed in full rather than accumulated, leaving a directory restores
// whatever its tools were shadowing.
func Build(global []Tool, dirs []Dir, loaded []string, trusted TrustFunc) Plan {
	desired := map[string]Tool{}
	var order []string

	add := func(t Tool) {
		if _, seen := desired[t.Name]; !seen {
			order = append(order, t.Name)
		}
		desired[t.Name] = t
	}
	for _, t := range global {
		add(t)
	}

	var notices []string
	for _, d := range dirs {
		if ok, why := trusted(d); !ok {
			if why != "" {
				notices = append(notices, why)
			}
			continue
		}
		for _, t := range d.Tools {
			add(t)
		}
	}

	have := map[string]string{} // name to the config path currently loaded
	for _, p := range loaded {
		if p == "" {
			continue
		}
		have[nameOf(p)] = p
	}

	plan := Plan{Notices: notices}

	for name := range have {
		if _, want := desired[name]; !want {
			plan.Unload = append(plan.Unload, name)
		}
	}
	slices.Sort(plan.Unload) // map order is random; the emitted script must not be

	for _, name := range order {
		t := desired[name]
		plan.Loaded = append(plan.Loaded, t.File)
		// redefining is only free-ish, so skip a tool already pointing at the
		// same config. a shadowed tool changes path and so gets re-emitted
		if have[name] != t.File {
			plan.Load = append(plan.Load, t)
		}
	}

	return plan
}

func nameOf(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

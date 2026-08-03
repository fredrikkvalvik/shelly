package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// build a tree of .shelly dirs: mk(t, home, "work/api", "deploy", "db")
func mk(t *testing.T, root, rel string, tools ...string) string {
	t.Helper()
	dir := filepath.Join(root, rel, DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range tools {
		if err := os.WriteFile(filepath.Join(dir, name+".toml"), []byte("doc=\"x\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func always(Dir) (bool, string) { return true, "" }

func names(tools []Tool) string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		out = append(out, t.Name)
	}
	return strings.Join(out, ",")
}

func TestDiscoverWalksUpToHome(t *testing.T) {
	home := t.TempDir()
	mk(t, home, ".", "personal")
	mk(t, home, "work", "shared")
	mk(t, home, "work/api", "deploy", "db")
	if err := os.MkdirAll(filepath.Join(home, "work/api/src/handlers"), 0o755); err != nil {
		t.Fatal(err)
	}

	dirs := Discover(filepath.Join(home, "work/api/src/handlers"), home)
	if len(dirs) != 3 {
		t.Fatalf("found %d dirs, want 3: %+v", len(dirs), dirs)
	}

	// outermost first, so a nearer definition can override one further out
	if got := names(dirs[0].Tools); got != "personal" {
		t.Errorf("dirs[0] = %q, want the home one first", got)
	}
	if got := names(dirs[2].Tools); got != "db,deploy" {
		t.Errorf("dirs[2] = %q, want the project's, sorted", got)
	}
}

func TestDiscoverStopsAtHome(t *testing.T) {
	root := t.TempDir()
	mk(t, root, ".", "above-home") // an ancestor of home
	home := filepath.Join(root, "home")
	mk(t, home, ".", "mine")

	dirs := Discover(home, home)
	if len(dirs) != 1 || names(dirs[0].Tools) != "mine" {
		t.Errorf("dirs = %+v, want only the one at home", dirs)
	}
}

func TestDiscoverIgnoresNonToml(t *testing.T) {
	home := t.TempDir()
	dir := mk(t, home, "p", "real")
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)

	dirs := Discover(filepath.Join(home, "p"), home)
	if len(dirs) != 1 || names(dirs[0].Tools) != "real" {
		t.Errorf("dirs = %+v", dirs)
	}
}

func TestDiscoverFindsNothing(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "empty"), 0o755)
	if dirs := Discover(filepath.Join(home, "empty"), home); len(dirs) != 0 {
		t.Errorf("dirs = %+v, want none", dirs)
	}
}

func TestPlanLoadsAndRemembers(t *testing.T) {
	global := []Tool{{Name: "ffzf", File: "/cfg/ffzf.toml"}}
	dirs := []Dir{{Path: "/p/.shelly", Tools: []Tool{{Name: "deploy", File: "/p/.shelly/deploy.toml"}}}}

	p := Build(global, dirs, nil, always)
	if got := names(p.Load); got != "ffzf,deploy" {
		t.Errorf("Load = %q", got)
	}
	if got := strings.Join(p.Loaded, " "); got != "/cfg/ffzf.toml /p/.shelly/deploy.toml" {
		t.Errorf("Loaded = %q", got)
	}
	if len(p.Unload) != 0 {
		t.Errorf("Unload = %q, want none", p.Unload)
	}
}

func TestPlanStayingPutIsANoOp(t *testing.T) {
	global := []Tool{{Name: "ffzf", File: "/cfg/ffzf.toml"}}
	p := Build(global, nil, []string{"/cfg/ffzf.toml"}, always)

	if len(p.Load) != 0 || len(p.Unload) != 0 {
		t.Errorf("Load=%q Unload=%q, want both empty", names(p.Load), p.Unload)
	}
}

// the point of recomputing the whole desired set rather than accumulating
func TestPlanRestoresAShadowedGlobalOnLeaving(t *testing.T) {
	global := []Tool{{Name: "deploy", File: "/cfg/deploy.toml"}}
	project := []Dir{{Path: "/p/.shelly", Tools: []Tool{{Name: "deploy", File: "/p/.shelly/deploy.toml"}}}}

	// entering: the project's version shadows the global one
	in := Build(global, project, []string{"/cfg/deploy.toml"}, always)
	if got := names(in.Load); got != "deploy" {
		t.Fatalf("entering Load = %q, want deploy re-emitted", got)
	}
	if in.Load[0].File != "/p/.shelly/deploy.toml" {
		t.Errorf("entering loaded %q, want the project's", in.Load[0].File)
	}

	// leaving: the global one comes back, and nothing is unloaded
	out := Build(global, nil, []string{"/p/.shelly/deploy.toml"}, always)
	if got := names(out.Load); got != "deploy" {
		t.Fatalf("leaving Load = %q, want the global restored", got)
	}
	if out.Load[0].File != "/cfg/deploy.toml" {
		t.Errorf("leaving loaded %q, want the global", out.Load[0].File)
	}
	if len(out.Unload) != 0 {
		t.Errorf("Unload = %q, want none: the name still exists", out.Unload)
	}
}

func TestPlanUnloadsWhatIsGone(t *testing.T) {
	p := Build(nil, nil, []string{"/p/.shelly/deploy.toml", "/p/.shelly/db.toml"}, always)
	if got := strings.Join(p.Unload, ","); got != "db,deploy" {
		t.Errorf("Unload = %q, want both, sorted", got)
	}
	if len(p.Loaded) != 0 {
		t.Errorf("Loaded = %q, want empty", p.Loaded)
	}
}

func TestPlanNearestWins(t *testing.T) {
	dirs := []Dir{
		{Path: "/a/.shelly", Tools: []Tool{{Name: "t", File: "/a/.shelly/t.toml"}}},
		{Path: "/a/b/.shelly", Tools: []Tool{{Name: "t", File: "/a/b/.shelly/t.toml"}}},
	}
	p := Build(nil, dirs, nil, always)
	if len(p.Load) != 1 || p.Load[0].File != "/a/b/.shelly/t.toml" {
		t.Errorf("Load = %+v, want only the nearest", p.Load)
	}
}

func TestPlanSkipsUntrusted(t *testing.T) {
	dirs := []Dir{{Path: "/p/.shelly", Tools: []Tool{{Name: "evil", File: "/p/.shelly/evil.toml"}}}}

	p := Build(nil, dirs, nil, func(d Dir) (bool, string) {
		return false, "not trusted: " + d.Path
	})
	if len(p.Load) != 0 {
		t.Errorf("Load = %+v, want nothing from an untrusted dir", p.Load)
	}
	if len(p.Notices) != 1 || !strings.Contains(p.Notices[0], "/p/.shelly") {
		t.Errorf("Notices = %q, want one naming the directory", p.Notices)
	}
}

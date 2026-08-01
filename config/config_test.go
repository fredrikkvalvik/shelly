package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const ffzf = `
name = "ffzf"
doc  = "fzf wrappers"

[cmd.search]
doc  = "fuzzy-pick a file matching a glob"
arg  = [{ name = "glob",   default = "*", doc = "glob to match", complete = "files" }]
flag = [{ name = "hidden", short = "H", type = "bool", pass = "--hidden", doc = "include hidden files" }]
exec = [
  { cmd = "rg",  argv = ["--files", "<hidden>", "--glob", "<glob>"] },
  { cmd = "fzf", argv = ["--height", "40%", "--preview", "bat --color=always {}"] },
]

[cmd.db]
doc = "database helpers"

[cmd.db.shell]
doc  = "open a psql shell"
exec = [{ cmd = "psql", argv = ["-h", "localhost", "-U", "dev", "app"] }]
`

func loadAs(t *testing.T, name, src string) (*Tool, error) {
	t.Helper()
	p := filepath.Join(t.TempDir(), name+".toml")
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return Load(p)
}

func load(t *testing.T, src string) (*Tool, error) {
	t.Helper()
	return loadAs(t, "tool", src)
}

func TestLoadTree(t *testing.T) {
	tool, err := loadAs(t, "ffzf", ffzf)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if tool.Name != "ffzf" || tool.Doc != "fzf wrappers" {
		t.Errorf("got name=%q doc=%q", tool.Name, tool.Doc)
	}
	if !filepath.IsAbs(tool.File) {
		t.Errorf("File = %q, want absolute", tool.File)
	}

	// document order, not alphabetical: search is declared before db
	var names []string
	for _, c := range tool.Root.Sub {
		names = append(names, c.Name)
	}
	if got := strings.Join(names, ","); got != "search,db" {
		t.Errorf("top level commands = %q, want \"search,db\"", got)
	}

	search := tool.Root.Lookup("search")
	if search == nil {
		t.Fatal("no search command")
	}
	if search.IsGroup() {
		t.Error("search should not be a group")
	}
	if len(search.Exec) != 2 || search.Exec[0].Cmd != "rg" || search.Exec[1].Cmd != "fzf" {
		t.Errorf("exec = %+v", search.Exec)
	}
	if got := strings.Join(search.Exec[0].Argv, "|"); got != "--files|<hidden>|--glob|<glob>" {
		t.Errorf("rg argv = %q", got)
	}

	glob := search.Arg("glob")
	if glob == nil || glob.Default != "*" || glob.Complete.Builtin != BuiltinFiles {
		t.Errorf("glob arg = %+v", glob)
	}
	hidden := search.Flag("hidden")
	if hidden == nil || !hidden.IsBool() || hidden.Pass != "--hidden" {
		t.Errorf("hidden flag = %+v", hidden)
	}
	if search.FlagShort("H") != hidden {
		t.Error("short H did not resolve to the hidden flag")
	}

	db := tool.Root.Lookup("db")
	if db == nil || !db.IsGroup() {
		t.Fatalf("db = %+v, want a group", db)
	}
	shell := db.Lookup("shell")
	if shell == nil {
		t.Fatal("no db shell command")
	}
	if got := strings.Join(shell.Path, " "); got != "db shell" {
		t.Errorf("path = %q, want \"db shell\"", got)
	}
}

func TestNameDefaultsToFilename(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mytool.toml")
	src := `
[cmd.go]
exec = [{ cmd = "echo", argv = ["hi"] }]
`
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	tool, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if tool.Name != "mytool" {
		t.Errorf("name = %q, want \"mytool\"", tool.Name)
	}
}

func TestValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{{
		name: "unreferenced flag",
		src: `
[cmd.x]
flag = [{ name = "force", type = "bool", pass = "-f" }]
exec = [{ cmd = "rm", argv = ["-r"] }]`,
		want: `flag "force" is declared but never referenced`,
	}, {
		name: "unreferenced arg",
		src: `
[cmd.x]
arg  = [{ name = "path" }]
exec = [{ cmd = "rm", argv = ["-r"] }]`,
		want: `arg "path" is declared but never referenced`,
	}, {
		name: "unknown reference",
		src: `
[cmd.x]
exec = [{ cmd = "rm", argv = ["<nope>"] }]`,
		want: `<nope> is not a declared arg or flag`,
	}, {
		name: "a hyphen after a reference is unambiguous now",
		src: `
[cmd.x]
arg  = [{ name = "a" }]
exec = [{ cmd = "echo", argv = ["<a>-post", "<nope>"] }]`,
		want: `<nope> is not a declared arg or flag`,
	}, {
		name: "no exec and no subcommands",
		src: `
[cmd.x]
doc = "nothing"`,
		want: `can never do anything`,
	}, {
		name: "group with args",
		src: `
[cmd.x]
arg = [{ name = "a" }]

[cmd.x.y]
exec = [{ cmd = "echo", argv = ["hi"] }]`,
		want: `declares arg or flag but has no "exec"`,
	}, {
		name: "bool flag without pass",
		src: `
[cmd.x]
flag = [{ name = "force", type = "bool" }]
exec = [{ cmd = "rm", argv = ["<force>"] }]`,
		want: `bool flag "force" needs "pass"`,
	}, {
		name: "bool flag embedded",
		src: `
[cmd.x]
flag = [{ name = "force", type = "bool", pass = "-f" }]
exec = [{ cmd = "rm", argv = ["--x=<force>"] }]`,
		want: `must be the whole element`,
	}, {
		name: "variadic not last",
		src: `
[cmd.x]
arg  = [{ name = "a", variadic = true }, { name = "b" }]
exec = [{ cmd = "echo", argv = ["<a...>", "<b>"] }]`,
		want: `must be the last arg`,
	}, {
		name: "variadic referenced without ellipsis",
		src: `
[cmd.x]
arg  = [{ name = "a", variadic = true }]
exec = [{ cmd = "echo", argv = ["<a>"] }]`,
		want: `reference it as <a...>`,
	}, {
		name: "required after optional",
		src: `
[cmd.x]
arg  = [{ name = "a" }, { name = "b", required = true }]
exec = [{ cmd = "echo", argv = ["<a>", "<b>"] }]`,
		want: `follows optional arg "a"`,
	}, {
		name: "duplicate short",
		src: `
[cmd.x]
flag = [{ name = "aa", short = "a", type = "bool", pass = "-a" },
        { name = "bb", short = "a", type = "bool", pass = "-b" }]
exec = [{ cmd = "echo", argv = ["<aa>", "<bb>"] }]`,
		want: `both use short "a"`,
	}, {
		name: "bad flag type",
		src: `
[cmd.x]
flag = [{ name = "n", type = "int" }]
exec = [{ cmd = "echo", argv = ["<n>"] }]`,
		want: `unknown flag type "int", want one of: bool, string`,
	}, {
		name: "unknown key",
		src: `
[cmd.x]
exce = [{ cmd = "echo", argv = ["hi"] }]`,
		want: `unknown key "exce": a subcommand must be a table`,
	}, {
		name: "unknown key in arg",
		src: `
[cmd.x]
arg  = [{ name = "a", require = true }]
exec = [{ cmd = "echo", argv = ["<a>"] }]`,
		want: `unknown key "require", want one of: name, doc, default, required, variadic, complete`,
	}, {
		name: "reserved subcommand name",
		src: `
[cmd.x.doc]
exec = [{ cmd = "echo", argv = ["hi"] }]`,
		want: `reserved key and cannot name a subcommand`,
	}, {
		name: "stage without cmd",
		src: `
[cmd.x]
exec = [{ argv = ["hi"] }]`,
		want: `missing "cmd"`,
	}, {
		name: "no commands",
		src:  `doc = "x"`,
		want: `defines no commands`,
	}, {
		name: "name disagrees with the filename",
		src: `
name = "other"

[cmd.x]
exec = [{ cmd = "echo", argv = ["hi"] }]`,
		want: `is "other" but the file is named "tool.toml"`,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := load(t, tc.src)
			if err == nil {
				t.Fatalf("expected an error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v\nwant it to contain %q", err, tc.want)
			}
		})
	}
}

func TestRefs(t *testing.T) {
	cases := []struct {
		in    string
		names []string
		whole bool
	}{
		{"<glob>", []string{"glob"}, true},
		{"<files...>", []string{"files"}, true},
		{"--glob=<glob>", []string{"glob"}, false},
		{"--files", nil, false},
		{"<<literal>", nil, false},
		{"<a>-<b>", []string{"a", "b"}, false},
		{"<x>-post", []string{"x"}, false},
		{"$HOME", nil, false},
		{"awk {print $1}", nil, false},
	}
	for _, tc := range cases {
		refs := Refs(tc.in)
		if len(refs) != len(tc.names) {
			t.Errorf("Refs(%q) = %+v, want %d refs", tc.in, refs, len(tc.names))
			continue
		}
		for i, r := range refs {
			if r.Name != tc.names[i] {
				t.Errorf("Refs(%q)[%d].Name = %q, want %q", tc.in, i, r.Name, tc.names[i])
			}
		}
		if len(refs) == 1 && refs[0].Whole(tc.in) != tc.whole {
			t.Errorf("Refs(%q)[0].Whole = %v, want %v", tc.in, refs[0].Whole(tc.in), tc.whole)
		}
	}
	if r := Refs("<files...>"); len(r) == 1 && !r[0].Variadic {
		t.Error("<files...> should be variadic")
	}
}

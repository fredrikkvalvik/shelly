package invoke

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fredrikkvalvik/shelly/config"
)

const src = `
name = "t"

[cmd.search]
doc  = "search files"
arg  = [{ name = "glob", default = "*" }]
flag = [
  { name = "hidden", short = "H", type = "bool", pass = "--hidden" },
  { name = "max",    short = "m", pass = "--max-count" },
  { name = "sep",    default = "," },
]
exec = [
  { cmd = "rg",  argv = ["--files", "<hidden>", "<max>", "--glob", "<glob>", "--sep=<sep>"] },
  { cmd = "fzf", argv = ["--height", "40%"] },
]

[cmd.cat]
arg  = [{ name = "files", variadic = true, required = true }]
exec = [{ cmd = "cat", argv = ["--", "<files...>"] }]

[cmd.lit]
arg  = [{ name = "x", default = "d" }]
exec = [{ cmd = "echo", argv = ["<<notaref>", "pre-<x>-post", "<x>"] }]

[cmd.db]
doc = "database helpers"

[cmd.db.shell]
exec = [{ cmd = "psql", argv = ["app"] }]
`

func loadSrc(t *testing.T, src string) *config.Tool {
	t.Helper()
	p := filepath.Join(t.TempDir(), "t.toml")
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	tl, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return tl
}

func tool(t *testing.T) *config.Tool {
	t.Helper()
	return loadSrc(t, src)
}

// "rg --files --glob * | fzf --height 40%"
func render(stages []Stage) string {
	parts := make([]string, 0, len(stages))
	for _, s := range stages {
		parts = append(parts, s.String())
	}
	return strings.Join(parts, " | ")
}

func TestResolve(t *testing.T) {
	tl := tool(t)

	cases := []struct {
		argv []string
		want string
	}{{
		// hidden unset drops, max has no default so drops, sep uses its default
		argv: []string{"search"},
		want: "rg --files --glob * --sep=, | fzf --height 40%",
	}, {
		argv: []string{"search", "*.go"},
		want: "rg --files --glob *.go --sep=, | fzf --height 40%",
	}, {
		argv: []string{"search", "-H", "*.go"},
		want: "rg --files --hidden --glob *.go --sep=, | fzf --height 40%",
	}, {
		argv: []string{"search", "--hidden", "*.go"},
		want: "rg --files --hidden --glob *.go --sep=, | fzf --height 40%",
	}, {
		// a string flag renders as pass plus value, two separate elements
		argv: []string{"search", "--max", "5"},
		want: "rg --files --max-count 5 --glob * --sep=, | fzf --height 40%",
	}, {
		argv: []string{"search", "--max=5"},
		want: "rg --files --max-count 5 --glob * --sep=, | fzf --height 40%",
	}, {
		argv: []string{"search", "-m", "5"},
		want: "rg --files --max-count 5 --glob * --sep=, | fzf --height 40%",
	}, {
		argv: []string{"search", "-m=5"},
		want: "rg --files --max-count 5 --glob * --sep=, | fzf --height 40%",
	}, {
		// embedded reference substitutes the bare value
		argv: []string{"search", "--sep", ";"},
		want: "rg --files --glob * --sep=; | fzf --height 40%",
	}, {
		argv: []string{"cat", "a", "b", "c"},
		want: "cat -- a b c",
	}, {
		// -- ends flag parsing, so a leading dash is just a value
		argv: []string{"search", "--", "-weird"},
		want: "rg --files --glob -weird --sep=, | fzf --height 40%",
	}, {
		argv: []string{"lit"},
		want: "echo <notaref> pre-d-post d",
	}, {
		argv: []string{"lit", "v"},
		want: "echo <notaref> pre-v-post v",
	}, {
		argv: []string{"db", "shell"},
		want: "psql app",
	}}

	for _, tc := range cases {
		t.Run(strings.Join(tc.argv, " "), func(t *testing.T) {
			inv, err := Parse(tl, tc.argv)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if inv.Help {
				t.Fatal("unexpected help")
			}
			stages, err := Resolve(inv)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if got := render(stages); got != tc.want {
				t.Errorf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}

// the core safety property: no shell means no interpretation
func TestValuesReachTheProgramVerbatim(t *testing.T) {
	tl := tool(t)

	for _, hostile := range []string{
		"$(rm -rf /)",
		"$HOME",
		"<glob>",
		"; rm -rf ~",
		"a b c",
		"`whoami`",
		"'quoted'",
		"*.go",
		"$HOME",
	} {
		inv, err := Parse(tl, []string{"search", "--", hostile})
		if err != nil {
			t.Fatalf("%q: parse: %v", hostile, err)
		}
		stages, err := Resolve(inv)
		if err != nil {
			t.Fatalf("%q: resolve: %v", hostile, err)
		}
		argv := stages[0].Argv
		// --files, --glob, <value>, --sep=,
		if got := argv[2]; got != hostile {
			t.Errorf("argv[2] = %q, want %q (argv %q)", got, hostile, argv)
		}
		if len(argv) != 4 {
			t.Errorf("%q split into %d elements: %q", hostile, len(argv), argv)
		}
	}
}

func TestHelp(t *testing.T) {
	tl := tool(t)

	for _, argv := range [][]string{
		{},
		{"--help"},
		{"-h"},
		{"db"},
		{"db", "--help"},
		{"search", "--help"},
		{"search", "-h"},
	} {
		inv, err := Parse(tl, argv)
		if err != nil {
			t.Fatalf("%q: %v", argv, err)
		}
		if !inv.Help {
			t.Errorf("%q: Help = false, want true", argv)
		}
	}
}

func TestParseErrors(t *testing.T) {
	tl := tool(t)

	cases := []struct {
		argv []string
		want string
	}{
		{[]string{"cat"}, `missing required argument "files"`},
		{[]string{"search", "a", "b"}, `unexpected argument "b"`},
		{[]string{"search", "--nope"}, `unknown flag "--nope"`},
		{[]string{"search", "-z"}, `unknown flag "-z"`},
		{[]string{"nope"}, `unknown command "nope"`},
		{[]string{"db", "nope"}, `unknown command "nope"`},
		{[]string{"search", "-H=x"}, `is a bool flag and takes no value`},
		{[]string{"search", "--max"}, `--max needs a value`},
		{[]string{"search", "-Hm"}, `short flags cannot be bundled`},
		{[]string{"db", "--verbose"}, `unexpected flag "--verbose"`},
	}

	for _, tc := range cases {
		t.Run(strings.Join(tc.argv, " "), func(t *testing.T) {
			_, err := Parse(tl, tc.argv)
			if err == nil {
				t.Fatalf("expected an error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v\nwant it to contain %q", err, tc.want)
			}
		})
	}
}

func TestUnknownCommandListsOptions(t *testing.T) {
	tl := tool(t)
	_, err := Parse(tl, []string{"nope"})
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, name := range []string{"search", "cat", "lit", "db"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not mention %q", err, name)
		}
	}
}

func ExampleResolve() {
	stages := []Stage{{Cmd: "rg", Argv: []string{"--files"}}, {Cmd: "fzf"}}
	fmt.Println(render(stages))
	// Output: rg --files | fzf
}

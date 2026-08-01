package invoke

import (
	"strings"
	"testing"

	"github.com/fredrikkvalvik/shelly/config"
)

func vals(t *testing.T, c Completion) string {
	t.Helper()
	vs := make([]string, 0, len(c.Candidates))
	for _, cand := range c.Candidates {
		vs = append(vs, cand.Value)
	}
	return strings.Join(vs, " ")
}

func TestComplete(t *testing.T) {
	tl := tool(t)

	cases := []struct {
		name      string
		words     []string
		cursor    int
		want      string
		directive config.Builtin
	}{{
		name:   "subcommands at the top level",
		words:  []string{"t", ""},
		cursor: 1,
		want:   "search cat lit db",
	}, {
		name:   "subcommands filtered by prefix",
		words:  []string{"t", "c"},
		cursor: 1,
		want:   "cat",
	}, {
		name:   "nested subcommands",
		words:  []string{"t", "db", ""},
		cursor: 2,
		want:   "shell",
	}, {
		name:   "flags when the partial starts with a dash",
		words:  []string{"t", "search", "-"},
		cursor: 2,
		want:   "--hidden -H --max -m --sep --help",
	}, {
		name:   "long flags only",
		words:  []string{"t", "search", "--"},
		cursor: 2,
		want:   "--hidden --max --sep --help",
	}, {
		name:      "positional uses the arg's completion source",
		words:     []string{"t", "search", ""},
		cursor:    2,
		directive: "",
	}, {
		name:   "cursor past the end of words",
		words:  []string{"t", "db"},
		cursor: 2,
		want:   "shell",
	}, {
		name:   "no candidates past the last positional",
		words:  []string{"t", "search", "a", ""},
		cursor: 3,
		want:   "",
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := Complete(tl, tc.words, tc.cursor)
			if got := vals(t, c); got != tc.want {
				t.Errorf("candidates = %q, want %q", got, tc.want)
			}
			if c.Directive != tc.directive {
				t.Errorf("directive = %q, want %q", c.Directive, tc.directive)
			}
		})
	}
}

func TestCompleteFromProgram(t *testing.T) {
	src := `
name = "t"

[cmd.pick]
arg  = [{ name = "which", complete = ["printf", "alpha\tfirst\nbeta\tsecond\n"] }]
exec = [{ cmd = "echo", argv = ["<which>"] }]
`
	tl := loadSrc(t, src)

	c := Complete(tl, []string{"t", "pick", ""}, 2)
	if got := vals(t, c); got != "alpha beta" {
		t.Fatalf("candidates = %q, want \"alpha beta\"", got)
	}
	if c.Candidates[0].Doc != "first" {
		t.Errorf("doc = %q, want \"first\"", c.Candidates[0].Doc)
	}

	// the prefix filter applies to the value, not the whole tab separated line
	c = Complete(tl, []string{"t", "pick", "b"}, 2)
	if got := vals(t, c); got != "beta" {
		t.Errorf("candidates = %q, want \"beta\"", got)
	}
}

func TestCompleteBuiltinDirective(t *testing.T) {
	src := `
name = "t"

[cmd.open]
arg  = [{ name = "path", complete = "files" }]
exec = [{ cmd = "cat", argv = ["<path>"] }]
`
	tl := loadSrc(t, src)

	c := Complete(tl, []string{"t", "open", ""}, 2)
	if c.Directive != config.BuiltinFiles {
		t.Errorf("directive = %q, want \"files\"", c.Directive)
	}
	if len(c.Candidates) != 0 {
		t.Errorf("candidates = %+v, want none", c.Candidates)
	}
}

func TestCompleteFlagValue(t *testing.T) {
	src := `
name = "t"

[cmd.x]
flag = [{ name = "mode", short = "m", complete = ["printf", "fast\nslow\n"] }]
exec = [{ cmd = "echo", argv = ["<mode>"] }]
`
	tl := loadSrc(t, src)

	// "--mode <TAB>" completes the value, not another flag
	if got := vals(t, Complete(tl, []string{"t", "x", "--mode", ""}, 3)); got != "fast slow" {
		t.Errorf("after --mode: %q, want \"fast slow\"", got)
	}
	if got := vals(t, Complete(tl, []string{"t", "x", "-m", "s"}, 3)); got != "slow" {
		t.Errorf("after -m s: %q, want \"slow\"", got)
	}
	// the joined form keeps the flag prefix on each candidate
	if got := vals(t, Complete(tl, []string{"t", "x", "--mode=f"}, 2)); got != "--mode=fast" {
		t.Errorf("--mode=f: %q, want \"--mode=fast\"", got)
	}
}

func TestHelpOutput(t *testing.T) {
	tl := tool(t)

	root := Help(tl, tl.Root)
	for _, want := range []string{"usage: t <command>", "commands:", "search", "database helpers"} {
		if !strings.Contains(root, want) {
			t.Errorf("root help missing %q:\n%s", want, root)
		}
	}

	search := Help(tl, tl.Root.Lookup("search"))
	for _, want := range []string{
		"t search — search files",
		"usage: t search [flags] [glob]",
		"-H, --hidden",
		"--max <value>",
		"(default: ,)",
		"-h, --help",
		"runs:",
		"rg --files <hidden> <max> --glob <glob> --sep=<sep> | fzf --height 40%",
	} {
		if !strings.Contains(search, want) {
			t.Errorf("search help missing %q:\n%s", want, search)
		}
	}

	cat := Help(tl, tl.Root.Lookup("cat"))
	if !strings.Contains(cat, "usage: t cat <files>...") {
		t.Errorf("cat help missing required variadic usage:\n%s", cat)
	}
}

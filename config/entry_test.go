package config

import (
	"strings"
	"testing"
)

func TestSplitEntry(t *testing.T) {
	cases := []struct {
		in   string
		spec string
		pass string // "|" joined; "-" means none given
		doc  string
	}{
		{`-H --hidden`, "-H --hidden", "-", ""},
		{`-H --hidden # include hidden files`, "-H --hidden", "-", "include hidden files"},
		{`-r --race -> -race`, "-r --race", "-race", ""},
		{`-r --race -> -race # the race detector`, "-r --race", "-race", "the race detector"},
		{`-n --run <regex> -> -run # only matching`, "-n --run <regex>", "-run", "only matching"},
		{`-q --query <s> -> --query --exact`, "-q --query <s>", "--query|--exact", ""},

		// a quoted token keeps its spaces, with either quote character
		{`-p --preview -> --preview 'bat --color=always {}'`,
			"-p --preview", "--preview|bat --color=always {}", ""},
		{`-p --preview -> --preview "bat --color=always {}"`,
			"-p --preview", "--preview|bat --color=always {}", ""},

		// markers inside a quoted token are just text
		{`--x -> --fmt "a # b" --other`, "--x", "--fmt|a # b|--other", ""},
		{`--x -> --fmt "a -> b"`, "--x", "--fmt|a -> b", ""},

		// and anything after # is the description, markers included
		{`--x # emits -> nothing`, "--x", "-", "emits -> nothing"},

		// an empty pass is how a string flag asks for the bare value
		{`--sep <s> ->`, "--sep <s>", "", ""},
	}

	for _, tc := range cases {
		e, err := splitEntry(ctx{}, tc.in)
		if err != nil {
			t.Errorf("%q: %v", tc.in, err)
			continue
		}
		if e.Spec != tc.spec {
			t.Errorf("%q: spec = %q, want %q", tc.in, e.Spec, tc.spec)
		}
		got := "-"
		if e.HasPass {
			got = strings.Join(e.Pass, "|")
		}
		if got != tc.pass {
			t.Errorf("%q: pass = %q, want %q", tc.in, got, tc.pass)
		}
		if e.Doc != tc.doc {
			t.Errorf("%q: doc = %q, want %q", tc.in, e.Doc, tc.doc)
		}
	}
}

func TestSplitEntryUnterminatedQuote(t *testing.T) {
	if _, err := splitEntry(ctx{}, `--x -> --fmt "unclosed`); err == nil {
		t.Fatal("an unterminated quote must be an error, not a silent token")
	}
}

// the whole point: this loads, and the preview command stays one argv element
func TestShorthandEndToEnd(t *testing.T) {
	tool, err := loadAs(t, "tool", `
[cmd.find]
flag = [
  "-r --race        -> -race   # enable the race detector",
  "-p --preview     -> --preview 'bat --color=always {}' --preview-window right,60%",
  "-H --hidden                 # include hidden files",
]
exec = [{ cmd = "fzf", argv = ["%{race}", "%{preview}", "%{hidden}"] }]`)
	if err != nil {
		t.Fatal(err)
	}

	find := tool.Root.Lookup("find")
	if got := find.Flag("race").Pass; len(got) != 1 || got[0] != "-race" {
		t.Errorf("race pass = %q", got)
	}
	if got := find.Flag("race").Doc; got != "enable the race detector" {
		t.Errorf("race doc = %q", got)
	}
	preview := find.Flag("preview").Pass
	if len(preview) != 4 || preview[1] != "bat --color=always {}" {
		t.Errorf("preview pass = %q, want the command as one token", preview)
	}
	// no pass given, so it falls back to the long name
	if got := find.Flag("hidden").Pass; len(got) != 1 || got[0] != "--hidden" {
		t.Errorf("hidden pass = %q, want [--hidden]", got)
	}
	if got := find.Flag("hidden").Doc; got != "include hidden files" {
		t.Errorf("hidden doc = %q", got)
	}
}

func TestShorthandConflictsWithKeys(t *testing.T) {
	cases := map[string]string{
		`[cmd.x]
flag = [{ spec = "--a -> -a", pass = "-b" }]
exec = [{ cmd = "e", argv = ["%{a}"] }]`: "pass is given twice",
		`[cmd.x]
flag = [{ spec = "--a # one", doc = "two" }]
exec = [{ cmd = "e", argv = ["%{a}"] }]`: "doc is given twice",
		`[cmd.x]
arg  = ["[a] -> -x"]
exec = [{ cmd = "e", argv = ["%{a}"] }]`: "an arg has nothing to pass",
	}
	for src, want := range cases {
		_, err := loadAs(t, "tool", src)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to contain %q", err, want)
		}
	}
}

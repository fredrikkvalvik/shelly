package shellgen

import (
	"os/exec"
	"strings"
	"testing"
)

func TestEmitUnsupportedShell(t *testing.T) {
	if _, err := Emit(Shell("fish"), "/bin/shelly", nil); err == nil {
		t.Fatal("expected an error for an unsupported shell")
	}
	if _, err := NewShell("fish"); err == nil {
		t.Fatal("expected NewShell to reject an unsupported shell")
	}
	for _, name := range []string{"zsh", "bash"} {
		if _, err := NewShell(name); err != nil {
			t.Errorf("NewShell(%q): %v", name, err)
		}
	}
}

func TestEmitDefinesEveryTool(t *testing.T) {
	tools := []Tool{{Name: "ffzf", File: "/a/ffzf.toml"}, {Name: "dk", File: "/a/dk.toml"}}

	for _, shell := range shells {
		out, err := Emit(shell, "/bin/shelly", tools)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"ffzf()", "dk()", "_shelly_ffzf", "_shelly_dk", "/a/ffzf.toml"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s output missing %q", shell, want)
			}
		}
	}
}

// a quote in a path must not be able to close the string and run something
func TestEmitQuotesHostilePaths(t *testing.T) {
	hostile := `/tmp/it's here/'; touch /tmp/PWNED; '/x.toml`
	tools := []Tool{{Name: "t", File: hostile}}

	for _, shell := range shells {
		out, err := Emit(shell, "/bin/shelly", tools)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out, "touch /tmp/PWNED; '") && !strings.Contains(out, `'\''`) {
			t.Errorf("%s output does not escape the quote:\n%s", shell, out)
		}

		// the shells themselves are the real oracle for whether this parses
		script := "shelly() { :; }; command() { :; }; compdef() { :; }; _describe() { :; }\n" + out
		bin := string(shell)
		if shell == Bash {
			bin = "/bin/bash" // macOS bash 3.2, the oldest we must support
		}
		if err := exec.Command(bin, "-n", "-c", script).Run(); err != nil {
			t.Errorf("%s cannot parse the emitted code: %v\n%s", shell, err, out)
		}
	}
}

func TestHelperNameIsShellSafe(t *testing.T) {
	if got := helper("my-tool.v2"); got != "_shelly_my_tool_v2" {
		t.Errorf("helper = %q, want \"_shelly_my_tool_v2\"", got)
	}
}

func TestSingleQuote(t *testing.T) {
	cases := map[string]string{
		"plain":   `'plain'`,
		"it's":    `'it'\''s'`,
		"":        `''`,
		`a'b'c`:   `'a'\''b'\''c'`,
		"$(evil)": `'$(evil)'`,
	}
	for in, want := range cases {
		if got := sq(in); got != want {
			t.Errorf("sq(%q) = %s, want %s", in, got, want)
		}
	}
}

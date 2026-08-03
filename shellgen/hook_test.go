package shellgen

import (
	"os/exec"
	"strings"
	"testing"
)

func shellBin(s Shell) string {
	if s == Bash {
		return "/bin/bash" // macOS 3.2, the oldest we must support
	}
	return string(s)
}

func parses(t *testing.T, shell Shell, script string) {
	t.Helper()
	stub := "shelly() { :; }; compdef() { :; }; complete() { :; }; _describe() { :; }\n"
	if err := exec.Command(shellBin(shell), "-n", "-c", stub+script).Run(); err != nil {
		t.Errorf("%s cannot parse:\n%s\n%v", shell, script, err)
	}
}

func TestHookParsesAndIsIdempotent(t *testing.T) {
	for _, shell := range shells {
		out, err := Hook(shell, "/path/to/shelly")
		if err != nil {
			t.Fatal(err)
		}
		parses(t, shell, out)

		// sourcing an rc file twice must not stack the hook up
		script := out + out + `
case ` + "`" + `echo x` + "`" + ` in *) ;; esac
`
		parses(t, shell, script)

		if !strings.Contains(out, "_shelly_hook") {
			t.Errorf("%s hook does not define _shelly_hook", shell)
		}
		if !strings.Contains(out, "'/path/to/shelly'") {
			t.Errorf("%s hook does not quote the binary path", shell)
		}
	}
}

func TestHookInstallsItselfOnlyOnce(t *testing.T) {
	// run the zsh hook twice for real and count the registrations
	out, err := Hook(Zsh, "/bin/true")
	if err != nil {
		t.Fatal(err)
	}
	script := out + out + "\nprint ${#precmd_functions[(R)_shelly_hook]}\nprint $precmd_functions\n"
	got, err := exec.Command("zsh", "-f", "-c", script).Output()
	if err != nil {
		t.Fatalf("zsh: %v", err)
	}
	if n := strings.Count(string(got), "_shelly_hook"); n != 1 {
		t.Errorf("hook registered %d times:\n%s", n, got)
	}
}

func TestUnloadRemovesFunctionAndCompletion(t *testing.T) {
	for _, shell := range shells {
		out, err := Unload(shell, []string{"ffzf", "my-tool"})
		if err != nil {
			t.Fatal(err)
		}
		parses(t, shell, out)

		for _, want := range []string{"unset -f ffzf _shelly_ffzf", "unset -f my-tool _shelly_my_tool"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s unload missing %q:\n%s", shell, want, out)
			}
		}
	}
	// the completion is torn down too, or it would offer subcommands for a
	// function that no longer exists
	zsh, _ := Unload(Zsh, []string{"ffzf"})
	if !strings.Contains(zsh, "compdef -d ffzf") {
		t.Errorf("zsh unload does not drop the completion:\n%s", zsh)
	}
	bash, _ := Unload(Bash, []string{"ffzf"})
	if !strings.Contains(bash, "complete -r ffzf") {
		t.Errorf("bash unload does not drop the completion:\n%s", bash)
	}
}

func TestNoticeAndSetenvQuote(t *testing.T) {
	hostile := "it's here; touch /tmp/PWNED"
	for _, shell := range shells {
		parses(t, shell, Notice([]string{hostile}))
		parses(t, shell, Setenv("SHELLY_LOADED", hostile))
	}
	if !strings.Contains(Notice([]string{"x"}), ">&2") {
		t.Error("notices must go to stderr")
	}
}

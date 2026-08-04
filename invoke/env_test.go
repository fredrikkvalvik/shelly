package invoke

import (
	"strings"
	"testing"
)

const envSrc = `
name = "t"

[cmd.run]
arg  = [{ name = "glob", default = "$HOME/default-glob" }]
flag = [{ name = "conf", default = "${XDG_CONFIG_HOME}/t.yml", pass = "--conf" }]
exec = [{ cmd = "rg", argv = [
  "--ignore-file", "$HOME/.rgignore",
  "--prog", "{print $1}",
  "--missing=$NOPE/x",
  "%{conf}",
  "%{glob}",
]}]
`

func fakeEnv(pairs map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := pairs[k]
		return v, ok
	}
}

func resolveWith(t *testing.T, argv []string, pairs map[string]string) []string {
	t.Helper()
	inv, err := Parse(loadSrc(t, envSrc), argv)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	inv.Env = fakeEnv(pairs)
	stages, err := Resolve(inv)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return stages[0].Argv
}

func TestEnvExpansion(t *testing.T) {
	argv := resolveWith(t, []string{"run"}, map[string]string{
		"HOME":            "/Users/f",
		"XDG_CONFIG_HOME": "/cfg",
	})

	want := []string{
		"--ignore-file", "/Users/f/.rgignore", // $VAR in config text
		"--prog", "{print $1}", // awk survives: $1 is not a reference
		// "--missing=$NOPE/x" dropped: unset, so no "--missing=/x"
		"--conf", "/cfg/t.yml", // ${VAR} inside a flag default
		"/Users/f/default-glob", // $VAR inside an arg default
	}
	if got := strings.Join(argv, "|"); got != strings.Join(want, "|") {
		t.Errorf("got  %q\nwant %q", argv, want)
	}
}

func TestUnsetEnvDropsTheElement(t *testing.T) {
	argv := resolveWith(t, []string{"run"}, map[string]string{})

	for _, a := range argv {
		// an unexpanded ref would be a leak; "/.rgignore" would mean the
		// element was truncated instead of dropped
		if strings.Contains(a, "$HOME") || strings.Contains(a, "$NOPE") || strings.HasPrefix(a, "/.") {
			t.Errorf("argv %q kept an unresolved or truncated element %q", argv, a)
		}
	}
	// everything env-dependent is gone, leaving only the literal flag names
	// and the awk program, whose $1 was never a reference
	if got := strings.Join(argv, "|"); got != "--ignore-file|--prog|{print $1}" {
		t.Errorf("argv = %q", got)
	}
}

// config authored text is expanded; what the user typed never is
func TestUserValuesAreNeverRescanned(t *testing.T) {
	for _, hostile := range []string{"$HOME", "${HOME}", "$HOME/x", "%{glob}", "%%{x}"} {
		argv := resolveWith(t, []string{"run", "--", hostile}, map[string]string{
			"HOME":            "/Users/f",
			"XDG_CONFIG_HOME": "/cfg",
			"glob":            "expanded",
		})
		got := argv[len(argv)-1]
		if got != hostile {
			t.Errorf("user value %q became %q", hostile, got)
		}
	}
}

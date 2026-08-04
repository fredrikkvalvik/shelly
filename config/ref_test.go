package config

import "testing"

func env(pairs map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := pairs[k]
		return v, ok
	}
}

func TestExpandLiteral(t *testing.T) {
	lookup := env(map[string]string{
		"HOME":  "/Users/f",
		"EMPTY": "",
	})

	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"plain", "plain", true},
		{"$HOME/.config", "/Users/f/.config", true},
		{"${HOME}/.config", "/Users/f/.config", true},
		{"--conf=$HOME/x", "--conf=/Users/f/x", true},
		{"$HOME$HOME", "/Users/f/Users/f", true},

		// set but empty is not the same as unset
		{"a${EMPTY}b", "ab", true},

		// unset fails the whole element rather than truncating the path
		{"$NOPE/x", "", false},
		{"${NOPE}", "", false},

		// escapes
		{"$$HOME", "$HOME", true},
		{"%%{literal}", "%{literal}", true},
		{"$$", "$", true},

		// shell special vars are NOT environment references, so an awk or sed
		// program passed as an argument survives
		{"{print $1}", "{print $1}", true},
		{"{print $1, $2}", "{print $1, $2}", true},
		{`echo $@`, `echo $@`, true},
		{"status $?", "status $?", true},
		{"$ alone", "$ alone", true},
	}

	for _, tc := range cases {
		got, ok := ExpandLiteral(tc.in, lookup)
		if ok != tc.ok {
			t.Errorf("ExpandLiteral(%q) ok = %v, want %v", tc.in, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("ExpandLiteral(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRefsAndEnvDoNotOverlap(t *testing.T) {
	// the whole point of the syntax split: neither sees the other's namespace
	if refs := Refs("$HOME"); len(refs) != 0 {
		t.Errorf("Refs(\"$HOME\") = %+v, want none", refs)
	}
	if refs := Refs("%{glob}"); len(refs) != 1 || refs[0].Name != "glob" {
		t.Errorf("Refs(%q) = %+v", "%{glob}", refs)
	}

	got, ok := ExpandLiteral("%{glob}", env(map[string]string{"glob": "no"}))
	if !ok || got != "%{glob}" {
		t.Errorf("ExpandLiteral saw a %%{name} reference: got %q, ok=%v", got, ok)
	}
}

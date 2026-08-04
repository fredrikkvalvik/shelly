package invoke

import (
	"strings"
	"testing"
)

const passSrc = `
name = "t"

[cmd.find]
flag = [
  { spec = "-p --preview", pass = [
      "--preview", "bat --color=always {}",
      "--preview-window", "right:60%",
      "--border"] },
  { spec = "-q --query <s>", pass = ["--query", "--exact"] },
]
exec = [{ cmd = "fzf", argv = ["--height", "40%", "%{preview}", "%{query}"] }]
`

func TestBoolFlagGatesSeveralArguments(t *testing.T) {
	tl := loadSrc(t, passSrc)

	cases := []struct {
		argv []string
		want string
	}{{
		// off: the whole cluster disappears, not just the first token
		argv: []string{"find"},
		want: "fzf --height 40%",
	}, {
		argv: []string{"find", "-p"},
		want: "fzf --height 40% --preview bat --color=always {} --preview-window right:60% --border",
	}, {
		// a string flag emits its pass tokens then the value
		argv: []string{"find", "-q", "main"},
		want: "fzf --height 40% --query --exact main",
	}}

	for _, tc := range cases {
		t.Run(strings.Join(tc.argv, " "), func(t *testing.T) {
			inv, err := Parse(tl, tc.argv)
			if err != nil {
				t.Fatalf("parse: %v", err)
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

// a multi word pass token stays one argv element
func TestPassTokensAreNotSplit(t *testing.T) {
	inv, err := Parse(loadSrc(t, passSrc), []string{"find", "-p"})
	if err != nil {
		t.Fatal(err)
	}
	stages, err := Resolve(inv)
	if err != nil {
		t.Fatal(err)
	}
	argv := stages[0].Argv
	if got := argv[3]; got != "bat --color=always {}" {
		t.Errorf("argv[3] = %q, want the preview command as a single element", got)
	}
	if len(argv) != 7 {
		t.Errorf("argv = %q, want 7 elements", argv)
	}
}

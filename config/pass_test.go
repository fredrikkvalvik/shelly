package config

import (
	"strings"
	"testing"
)

// one bool flag gating a cluster of arguments is the point of the list form
func TestPassAcceptsStringOrList(t *testing.T) {
	tool, err := loadAs(t, "tool", `
[cmd.find]
flag = [
  { name = "one",     type = "bool", pass = "--hidden" },
  { name = "preview", type = "bool", pass = [
      "--preview", "bat --color=always {}",
      "--preview-window", "right:60%",
      "--border"] },
]
exec = [{ cmd = "fzf", argv = ["%{one}", "%{preview}"] }]
`)
	if err != nil {
		t.Fatal(err)
	}

	find := tool.Root.Lookup("find")
	if got := find.Flag("one").Pass; len(got) != 1 || got[0] != "--hidden" {
		t.Errorf("string form = %q, want one element", got)
	}
	if got := find.Flag("preview").Pass; len(got) != 5 {
		t.Errorf("list form = %q, want 5 elements", got)
	}
}

func TestPassRejectsOtherTypes(t *testing.T) {
	_, err := loadAs(t, "tool", `
[cmd.x]
flag = [{ name = "n", type = "bool", pass = 3 }]
exec = [{ cmd = "echo", argv = ["%{n}"] }]
`)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "want a string or a list of strings") {
		t.Errorf("error = %v", err)
	}
}

func TestEmptyPassListStillRejectedForBool(t *testing.T) {
	_, err := loadAs(t, "tool", `
[cmd.x]
flag = [{ name = "n", type = "bool", pass = [] }]
exec = [{ cmd = "echo", argv = ["%{n}"] }]
`)
	if err == nil || !strings.Contains(err.Error(), `bool flag "n" needs "pass"`) {
		t.Errorf("error = %v, want the empty pass list rejected", err)
	}
}

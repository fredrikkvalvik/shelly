package config

import (
	"strings"
	"testing"
)

// most wrappers pass a flag straight through, so that is the default
func TestPassDefaultsToTheLongName(t *testing.T) {
	tool, err := loadAs(t, "tool", `
[cmd.find]
flag = ["-H --hidden", "--dry-run"]
exec = [{ cmd = "fzf", argv = ["%{hidden}", "%{dry-run}"] }]`)
	if err != nil {
		t.Fatal(err)
	}

	find := tool.Root.Lookup("find")
	for name, want := range map[string]string{"hidden": "--hidden", "dry-run": "--dry-run"} {
		got := find.Flag(name).Pass
		if len(got) != 1 || got[0] != want {
			t.Errorf("%s pass = %q, want [%s]", name, got, want)
		}
	}
}

// one bool flag gating a cluster of arguments is the point of the list form
func TestPassAsAPropertyOrAChildNode(t *testing.T) {
	tool, err := loadAs(t, "tool", `
[cmd.find]
flag = [
  { spec = "-o --one", pass = "--hidden" },
  { spec = "-p --preview", pass = [
      "--preview", "bat --color=always {}",
      "--preview-window", "right,60%",
      "--border"] },
]
exec = [{ cmd = "fzf", argv = ["%{one}", "%{preview}"] }]`)
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
	// a multi word token stays one element
	if got := find.Flag("preview").Pass[1]; got != "bat --color=always {}" {
		t.Errorf("pass[1] = %q, want the preview command unsplit", got)
	}
}

func TestFlagTypeComesFromTheSpec(t *testing.T) {
	tool, err := loadAs(t, "tool", `
[cmd.x]
flag = ["-H --hidden", "-m --max <count>"]
exec = [{ cmd = "echo", argv = ["%{hidden}", "%{max}"] }]`)
	if err != nil {
		t.Fatal(err)
	}

	x := tool.Root.Lookup("x")
	if !x.Flag("hidden").IsBool() {
		t.Error("a flag with no value placeholder should be a bool")
	}
	if x.Flag("max").IsBool() {
		t.Error("a flag with a value placeholder should be a string flag")
	}
}

func TestArgSpecForms(t *testing.T) {
	tool, err := loadAs(t, "tool", `
[cmd.x]
arg  = ["<req>", "[opt]", "[rest]..."]
exec = [{ cmd = "echo", argv = ["%{req}", "%{opt}", "%{rest...}"] }]`)
	if err != nil {
		t.Fatal(err)
	}

	x := tool.Root.Lookup("x")
	if !x.Arg("req").Required || x.Arg("req").Variadic {
		t.Errorf("<req> = %+v, want required and not variadic", x.Arg("req"))
	}
	if x.Arg("opt").Required {
		t.Error("[opt] should not be required")
	}
	if !x.Arg("rest").Variadic || x.Arg("rest").Required {
		t.Errorf("[rest]... = %+v, want variadic and optional", x.Arg("rest"))
	}
}

func TestFlagSpecErrors(t *testing.T) {
	cases := map[string]string{
		`-a -b --long`:    "two short names",
		`--one --two`:     "two long names",
		`-ab --long`:      "want a single character",
		`--long nonsense`: "neither a name nor a value placeholder",
	}
	for spec, want := range cases {
		_, err := loadAs(t, "tool", "[cmd.x]\nflag = [\""+spec+"\"]\nexec = [{ cmd = \"echo\", argv = [\"hi\"] }]")
		if err == nil {
			t.Errorf("%s: expected an error containing %q", spec, want)
			continue
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%s: error = %v, want it to contain %q", spec, err, want)
		}
	}
}

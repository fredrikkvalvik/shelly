package config

import (
	"strings"
	"testing"
)

func TestNewFlagType(t *testing.T) {
	for _, s := range []string{"bool", "string"} {
		got, err := NewFlagType(s)
		if err != nil {
			t.Errorf("NewFlagType(%q): %v", s, err)
			continue
		}
		if string(got) != s || !got.Valid() {
			t.Errorf("NewFlagType(%q) = %q", s, got)
		}
	}

	for _, s := range []string{"int", "Bool", "", "boolean"} {
		got, err := NewFlagType(s)
		if err == nil {
			t.Errorf("NewFlagType(%q) = %q, want an error", s, got)
			continue
		}
		if got != "" {
			t.Errorf("NewFlagType(%q) returned %q alongside its error, want the zero value", s, got)
		}
		// the message must name the alternatives, not just reject
		if !strings.Contains(err.Error(), "bool, string") {
			t.Errorf("NewFlagType(%q) error %q does not list the valid types", s, err)
		}
	}
}

func TestNewBuiltin(t *testing.T) {
	for _, s := range []string{"files", "dirs"} {
		got, err := NewBuiltin(s)
		if err != nil {
			t.Errorf("NewBuiltin(%q): %v", s, err)
			continue
		}
		if string(got) != s || !got.Valid() {
			t.Errorf("NewBuiltin(%q) = %q", s, got)
		}
	}

	// empty is not a builtin: absence is Complete.Empty, not a variant
	for _, s := range []string{"", "file", "FILES", "commands"} {
		if got, err := NewBuiltin(s); err == nil {
			t.Errorf("NewBuiltin(%q) = %q, want an error", s, got)
		}
	}
}

func TestZeroValuesAreInvalid(t *testing.T) {
	var ft FlagType
	if ft.Valid() {
		t.Error("the zero FlagType should not be valid")
	}
	var b Builtin
	if b.Valid() {
		t.Error("the zero Builtin should not be valid")
	}
}

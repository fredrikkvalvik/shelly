package config

import (
	"fmt"
	"slices"
	"strings"
)

// what a flag carries on the command line
type FlagType string

const (
	FlagBool   FlagType = "bool"   // present or absent, no value
	FlagString FlagType = "string" // takes a value
)

var flagTypes = []FlagType{FlagBool, FlagString}

func (t FlagType) Valid() bool { return valid(t, flagTypes) }

// NewFlagType converts a config value, rejecting anything unknown.
func NewFlagType(s string) (FlagType, error) {
	t := FlagType(s)
	if !t.Valid() {
		return "", fmt.Errorf("unknown flag type %q, want one of: %s", s, list(flagTypes))
	}
	return t, nil
}

// a completion source the shell already knows how to satisfy, which is why
// shelly hands it back as a directive rather than producing candidates itself
type Builtin string

const (
	BuiltinFiles Builtin = "files"
	BuiltinDirs  Builtin = "dirs"
)

var builtins = []Builtin{BuiltinFiles, BuiltinDirs}

func (b Builtin) Valid() bool { return valid(b, builtins) }

// NewBuiltin converts a config value, rejecting anything unknown. The empty
// string is not a builtin: absence is modelled by Complete.Empty.
func NewBuiltin(s string) (Builtin, error) {
	b := Builtin(s)
	if !b.Valid() {
		return "", fmt.Errorf("unknown builtin %q, want one of: %s", s, list(builtins))
	}
	return b, nil
}

func valid[T comparable](v T, all []T) bool { return slices.Contains(all, v) }

func list[T ~string](all []T) string {
	ss := make([]string, 0, len(all))
	for _, v := range all {
		ss = append(ss, string(v))
	}
	return strings.Join(ss, ", ")
}

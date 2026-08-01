// Package shellgen emits the shell code that makes a tool callable.
//
// The output is meant to be eval'd, so it never touches disk:
//
//	eval "$(shelly init zsh)"
//	eval "$(shelly load ./tools/ffzf.toml)"
//
// Both go through the same emitter; only the set of configs differs. Each tool
// gets a function pair with its config path baked in, rather than a shared
// completion function backed by a name to config map, because such a map needs
// an associative array and macOS still ships bash 3.2.
package shellgen

import (
	"fmt"
	"strings"
)

// a tool to make callable
type Tool struct {
	Name string
	File string // absolute path to its config
}

// Emit returns shell code defining every tool. bin must be the absolute path
// to the shelly binary, which is what keeps the functions from recursing when
// a tool happens to be named shelly.
func Emit(shell, bin string, tools []Tool) (string, error) {
	var emit func(*strings.Builder, string, Tool)

	switch shell {
	case "zsh":
		emit = emitZsh
	case "bash":
		emit = emitBash
	default:
		return "", fmt.Errorf("unsupported shell %q, want \"zsh\" or \"bash\"", shell)
	}

	var b strings.Builder
	for _, t := range tools {
		emit(&b, bin, t)
	}
	return b.String(), nil
}

func emitZsh(b *strings.Builder, bin string, t Tool) {
	fn := helper(t.Name)
	fmt.Fprintf(b, `%s() { %s run --config %s -- "$@"; }
%s() {
  local out
  out="$(%s complete --config %s --cursor $((CURRENT-1)) -- "${words[@]}")"
  case "$out" in
    ':files') _files; return ;;
    ':dirs') _files -/; return ;;
    '') return ;;
  esac
  local -a cands
  cands=("${(@)${(@f)out}//$'\t'/:}")
  _describe -t values %s cands
}
if (( $+functions[compdef] )); then compdef %s %s; fi
`,
		t.Name, sq(bin), sq(t.File),
		fn,
		sq(bin), sq(t.File),
		sq(t.Name),
		fn, sq(t.Name))
}

func emitBash(b *strings.Builder, bin string, t Tool) {
	fn := helper(t.Name)
	// nothing here may use bash 4 features: no mapfile, no associative arrays
	fmt.Fprintf(b, `%s() { %s run --config %s -- "$@"; }
%s() {
  local out cur IFS=$'\n'
  cur="${COMP_WORDS[COMP_CWORD]}"
  out="$(%s complete --config %s --cursor "$COMP_CWORD" -- "${COMP_WORDS[@]}")"
  case "$out" in
    ':files') COMPREPLY=( $(compgen -f -- "$cur") ); return ;;
    ':dirs') COMPREPLY=( $(compgen -d -- "$cur") ); return ;;
    '') COMPREPLY=(); return ;;
  esac
  COMPREPLY=( $(printf '%%s' "$out" | cut -f1) )
}
complete -F %s %s
`,
		t.Name, sq(bin), sq(t.File),
		fn,
		sq(bin), sq(t.File),
		fn, sq(t.Name))
}

// helper builds a function name that is safe in both shells
func helper(name string) string {
	var b strings.Builder
	b.WriteString("_shelly_")
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// sq single quotes a string for the shell, which is total: inside single
// quotes every character is literal, and a quote is closed, escaped, reopened
func sq(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

package invoke

import (
	"fmt"
	"strings"

	"github.com/fredrikkvalvik/shelly/config"
)

// one program of the pipeline, ready to run
type Stage struct {
	Cmd  string
	Argv []string
}

func (s Stage) String() string {
	return strings.Join(append([]string{s.Cmd}, s.Argv...), " ")
}

// Resolve substitutes the invocation's values into each stage's argv.
//
// A <name> that is the whole element renders fully: a bool flag becomes its
// pass token, a string flag becomes pass plus value, a variadic arg expands to
// all its values. A <name> embedded in a larger element substitutes only the
// raw value. Either way an unset value with no default drops the element,
// never leaving an empty string behind.
//
// Config authored text around those references is expanded too, so $HOME and
// ${HOME} become the environment's value. Values the user typed are never
// rescanned, so an argument that happens to contain $HOME stays literal.
func Resolve(inv *Invocation) ([]Stage, error) {
	stages := make([]Stage, 0, len(inv.Cmd.Exec))

	for _, s := range inv.Cmd.Exec {
		argv := make([]string, 0, len(s.Argv))
		for _, elem := range s.Argv {
			vals, ok, err := resolveElem(inv, elem)
			if err != nil {
				return nil, err
			}
			if ok {
				argv = append(argv, vals...)
			}
		}
		stages = append(stages, Stage{Cmd: s.Cmd, Argv: argv})
	}

	return stages, nil
}

func resolveElem(inv *Invocation, elem string) ([]string, bool, error) {
	refs := config.Refs(elem)
	if len(refs) == 0 {
		s, ok := inv.expand(elem)
		return []string{s}, ok, nil
	}
	if len(refs) == 1 && refs[0].Whole(elem) {
		return resolveWhole(inv, refs[0])
	}

	var b strings.Builder
	pos := 0
	for _, r := range refs {
		lit, ok := inv.expand(elem[pos:r.Start])
		if !ok {
			return nil, false, nil
		}
		b.WriteString(lit)

		v, ok, err := rawValue(inv, r.Name)
		if err != nil {
			return nil, false, err
		}
		if !ok {
			return nil, false, nil // unset, so the whole element goes away
		}
		b.WriteString(v)
		pos = r.End
	}

	lit, ok := inv.expand(elem[pos:])
	if !ok {
		return nil, false, nil
	}
	b.WriteString(lit)

	return []string{b.String()}, true, nil
}

func resolveWhole(inv *Invocation, r config.Ref) ([]string, bool, error) {
	if a := inv.Cmd.Arg(r.Name); a != nil {
		vals := inv.Args[a.Name]
		switch {
		case a.Variadic:
			if len(vals) == 0 {
				return nil, false, nil
			}
			return vals, true, nil
		case len(vals) > 0:
			return vals[:1], true, nil
		case a.Default != "":
			return inv.expandOne(a.Default)
		default:
			return nil, false, nil
		}
	}

	if f := inv.Cmd.Flag(r.Name); f != nil {
		v, set := inv.Flags[f.Name]
		if f.IsBool() {
			if !set {
				return nil, false, nil
			}
			return []string{f.Pass}, true, nil
		}
		if !set {
			if f.Default == "" {
				return nil, false, nil
			}
			// a default comes from the config, so it may reference the
			// environment; a value the user typed may not
			d, ok := inv.expand(f.Default)
			if !ok {
				return nil, false, nil
			}
			v = d
		}
		if f.Pass != "" {
			return []string{f.Pass, v}, true, nil
		}
		return []string{v}, true, nil
	}

	return nil, false, fmt.Errorf("<%s> is not a declared arg or flag", r.Name)
}

// the bare value, for a reference embedded in a larger element
func rawValue(inv *Invocation, name string) (string, bool, error) {
	if a := inv.Cmd.Arg(name); a != nil {
		if vals := inv.Args[name]; len(vals) > 0 {
			return vals[0], true, nil
		}
		if a.Default != "" {
			s, ok := inv.expand(a.Default)
			return s, ok, nil
		}
		return "", false, nil
	}

	if f := inv.Cmd.Flag(name); f != nil {
		if f.IsBool() {
			// validation rejects this, so reaching it means a bug upstream
			return "", false, fmt.Errorf("bool flag <%s> cannot be embedded in an element", name)
		}
		if v, set := inv.Flags[name]; set {
			return v, true, nil
		}
		if f.Default != "" {
			s, ok := inv.expand(f.Default)
			return s, ok, nil
		}
		return "", false, nil
	}

	return "", false, fmt.Errorf("<%s> is not a declared arg or flag", name)
}

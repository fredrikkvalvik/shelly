// Package invoke turns a command line into the argv of each pipeline stage.
package invoke

import (
	"fmt"
	"strings"

	"github.com/fredrikkvalvik/shelly/config"
)

// what the user typed, resolved against the command tree
type Invocation struct {
	Tool  *config.Tool
	Cmd   *config.Cmd
	Args  map[string][]string // declared arg name to the values given
	Flags map[string]string   // declared flag name to its value; "" for a bool
	Help  bool
}

func (i *Invocation) FlagSet(name string) bool {
	_, ok := i.Flags[name]
	return ok
}

// Parse walks the command tree, then reads flags and positional args for the
// command it lands on. There is no passthrough: anything undeclared is an error.
func Parse(t *config.Tool, argv []string) (*Invocation, error) {
	inv := &Invocation{
		Tool:  t,
		Cmd:   t.Root,
		Args:  map[string][]string{},
		Flags: map[string]string{},
	}

	i := 0
	for i < len(argv) && len(inv.Cmd.Sub) > 0 {
		a := argv[i]
		if a == "--help" || a == "-h" {
			inv.Help = true
			return inv, nil
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			return nil, fmt.Errorf("%s: unexpected flag %q, %s takes a subcommand",
				pathOf(inv), a, nameOf(inv))
		}
		sub := inv.Cmd.Lookup(a)
		if sub == nil {
			return nil, unknownCmd(inv, a)
		}
		inv.Cmd = sub
		i++
	}

	// a group has nothing to run, so it shows help
	if inv.Cmd.IsGroup() {
		inv.Help = true
		return inv, nil
	}

	positional, err := parseFlags(inv, argv[i:])
	if err != nil {
		return nil, err
	}
	if inv.Help {
		return inv, nil
	}
	if err := bindArgs(inv, positional); err != nil {
		return nil, err
	}
	return inv, nil
}

func parseFlags(inv *Invocation, argv []string) ([]string, error) {
	var positional []string
	endFlags := false

	for i := 0; i < len(argv); i++ {
		a := argv[i]

		switch {
		case endFlags:
			positional = append(positional, a)

		case a == "--":
			endFlags = true

		case a == "--help" || a == "-h":
			inv.Help = true
			return nil, nil

		case strings.HasPrefix(a, "--"):
			name, val, hasVal := strings.Cut(a[2:], "=")
			f := inv.Cmd.Flag(name)
			if f == nil {
				return nil, unknownFlag(inv, a)
			}
			v, consumed, err := flagValue(inv, f, "--"+name, val, hasVal, argv, i)
			if err != nil {
				return nil, err
			}
			inv.Flags[f.Name] = v
			i += consumed

		case strings.HasPrefix(a, "-") && a != "-":
			short, val, hasVal := strings.Cut(a[1:], "=")
			if len([]rune(short)) != 1 {
				return nil, fmt.Errorf("%s: unknown flag %q (short flags cannot be bundled, pass them separately)",
					pathOf(inv), a)
			}
			f := inv.Cmd.FlagShort(short)
			if f == nil {
				return nil, unknownFlag(inv, a)
			}
			v, consumed, err := flagValue(inv, f, "-"+short, val, hasVal, argv, i)
			if err != nil {
				return nil, err
			}
			inv.Flags[f.Name] = v
			i += consumed

		default:
			positional = append(positional, a)
		}
	}

	return positional, nil
}

// returns the flag's value and how many extra argv entries it consumed
func flagValue(inv *Invocation, f *config.Flag, as, val string, hasVal bool, argv []string, i int) (string, int, error) {
	if f.IsBool() {
		if hasVal {
			return "", 0, fmt.Errorf("%s: %s is a bool flag and takes no value", pathOf(inv), as)
		}
		return "", 0, nil
	}
	if hasVal {
		return val, 0, nil
	}
	if i+1 >= len(argv) {
		return "", 0, fmt.Errorf("%s: %s needs a value", pathOf(inv), as)
	}
	return argv[i+1], 1, nil
}

func bindArgs(inv *Invocation, positional []string) error {
	p := 0
	for _, a := range inv.Cmd.Args {
		if a.Variadic {
			rest := positional[p:]
			if a.Required && len(rest) == 0 {
				return fmt.Errorf("%s: missing required argument %q", pathOf(inv), a.Name)
			}
			if len(rest) > 0 {
				inv.Args[a.Name] = rest
			}
			p = len(positional)
			continue
		}
		if p < len(positional) {
			inv.Args[a.Name] = []string{positional[p]}
			p++
			continue
		}
		if a.Required {
			return fmt.Errorf("%s: missing required argument %q", pathOf(inv), a.Name)
		}
	}

	if p < len(positional) {
		return fmt.Errorf("%s: unexpected argument %q", pathOf(inv), positional[p])
	}
	return nil
}

func nameOf(inv *Invocation) string {
	if len(inv.Cmd.Path) == 0 {
		return inv.Tool.Name
	}
	return inv.Cmd.Name
}

func pathOf(inv *Invocation) string {
	return strings.Join(append([]string{inv.Tool.Name}, inv.Cmd.Path...), " ")
}

func unknownCmd(inv *Invocation, got string) error {
	names := make([]string, 0, len(inv.Cmd.Sub))
	for _, s := range inv.Cmd.Sub {
		names = append(names, s.Name)
	}
	return fmt.Errorf("%s: unknown command %q, want one of: %s",
		pathOf(inv), got, strings.Join(names, ", "))
}

func unknownFlag(inv *Invocation, got string) error {
	if len(inv.Cmd.Flags) == 0 {
		return fmt.Errorf("%s: unknown flag %q, this command takes no flags", pathOf(inv), got)
	}
	names := make([]string, 0, len(inv.Cmd.Flags))
	for _, f := range inv.Cmd.Flags {
		if f.Short != "" {
			names = append(names, fmt.Sprintf("-%s, --%s", f.Short, f.Name))
			continue
		}
		names = append(names, "--"+f.Name)
	}
	return fmt.Errorf("%s: unknown flag %q, want one of: %s",
		pathOf(inv), got, strings.Join(names, ", "))
}

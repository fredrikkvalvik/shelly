package config

import (
	"fmt"
	"regexp"
	"strings"
)

// names appear both on the command line and in generated shell function names.
// hyphens are allowed inside a name but not at either end, which is what keeps
// "$a-$b" unambiguous
const namePat = `[A-Za-z_](?:[A-Za-z0-9_-]*[A-Za-z0-9_])?`

var nameRe = regexp.MustCompile(`^` + namePat + `$`)

// what nameRe means, for error messages
const nameHelp = "a letter or underscore, then letters, digits, underscores or inner hyphens"

func validate(t *Tool) error {
	if !nameRe.MatchString(t.Name) {
		return fmt.Errorf("tool name %q must be %s", t.Name, nameHelp)
	}
	if len(t.Root.Sub) == 0 {
		return fmt.Errorf("defines no commands: expected at least one [cmd.*] table")
	}
	for _, c := range t.Root.Sub {
		if err := validateCmd(c); err != nil {
			return err
		}
	}
	return nil
}

func validateCmd(c *Cmd) error {
	where := "cmd." + strings.Join(c.Path, ".")
	errf := func(format string, a ...any) error {
		return fmt.Errorf("%s: %s", where, fmt.Sprintf(format, a...))
	}

	if !nameRe.MatchString(c.Name) {
		return errf("command name %q must be %s", c.Name, nameHelp)
	}
	if c.IsGroup() && len(c.Sub) == 0 {
		return errf(`has no "exec" and no subcommands, so it can never do anything`)
	}
	// otherwise "db shell" is ambiguous: a subcommand, or "shell" as a
	// positional arg to db
	if !c.IsGroup() && len(c.Sub) > 0 {
		return errf(`has subcommands and cannot also have "exec"`)
	}
	if c.IsGroup() && (len(c.Args) > 0 || len(c.Flags) > 0) {
		return errf(`declares arg or flag but has no "exec"`)
	}

	if err := validateArgs(c, errf); err != nil {
		return err
	}
	if err := validateFlags(c, errf); err != nil {
		return err
	}
	if err := validateRefs(c, errf); err != nil {
		return err
	}

	for _, s := range c.Sub {
		if err := validateCmd(s); err != nil {
			return err
		}
	}
	return nil
}

type errfn func(format string, a ...any) error

func validateArgs(c *Cmd, errf errfn) error {
	seen := map[string]bool{}
	optional := ""

	for i, a := range c.Args {
		if !nameRe.MatchString(a.Name) {
			return errf("arg %q must be %s", a.Name, nameHelp)
		}
		if seen[a.Name] {
			return errf("duplicate arg %q", a.Name)
		}
		seen[a.Name] = true

		if a.Required && a.Default != "" {
			return errf("arg %q is required and cannot also have a default", a.Name)
		}
		if a.Variadic {
			if i != len(c.Args)-1 {
				return errf("arg %q is variadic and must be the last arg", a.Name)
			}
			if a.Default != "" {
				return errf("arg %q is variadic and cannot have a default", a.Name)
			}
		}
		if a.Required && optional != "" {
			return errf("arg %q is required but follows optional arg %q", a.Name, optional)
		}
		if !a.Required {
			optional = a.Name
		}
	}
	return nil
}

func validateFlags(c *Cmd, errf errfn) error {
	seen := map[string]bool{}
	shorts := map[string]string{}

	for _, f := range c.Flags {
		if !nameRe.MatchString(f.Name) {
			return errf("flag %q must be %s", f.Name, nameHelp)
		}
		if seen[f.Name] {
			return errf("duplicate flag %q", f.Name)
		}
		seen[f.Name] = true

		// NewFlagType guards the load path; this catches a Tool built in Go
		if !f.Type.Valid() {
			return errf("flag %q has invalid type %q", f.Name, f.Type)
		}

		if f.Short != "" {
			if len([]rune(f.Short)) != 1 {
				return errf("flag %q has short %q, want a single character", f.Name, f.Short)
			}
			if prev, ok := shorts[f.Short]; ok {
				return errf("flags %q and %q both use short %q", prev, f.Name, f.Short)
			}
			shorts[f.Short] = f.Name
		}

		// --help and -h are handled before declared flags are consulted
		if f.Name == "help" {
			return errf(`flag "help" is reserved`)
		}
		if f.Short == "h" {
			return errf(`flag %q uses short "h", which is reserved for --help`, f.Name)
		}

		if f.IsBool() {
			if f.Pass == "" {
				return errf(`bool flag %q needs "pass", there is nothing to emit without it`, f.Name)
			}
			if f.Default != "" {
				return errf("bool flag %q cannot have a default", f.Name)
			}
		}
	}
	return nil
}

// the bare form takes the longest match, so "$x-post" swallows a trailing
// hyphenated word. if a declared name is hiding in there, say so
func braceHint(c *Cmd, name string) string {
	for i, r := range name {
		if r != '-' {
			continue
		}
		head := name[:i]
		if c.Arg(head) != nil || c.Flag(head) != nil {
			return fmt.Sprintf(" (did you mean ${%s}%s?)", head, name[i:])
		}
	}
	return ""
}

// every $name must resolve, and every declared arg and flag must be used
func validateRefs(c *Cmd, errf errfn) error {
	used := map[string]bool{}

	for i, s := range c.Exec {
		for j, elem := range s.Argv {
			for _, r := range Refs(elem) {
				at := fmt.Sprintf("exec[%d].argv[%d]", i, j)

				arg := c.Arg(r.Name)
				flag := c.Flag(r.Name)
				switch {
				case arg == nil && flag == nil:
					return errf("%s: $%s is not a declared arg or flag%s", at, r.Name, braceHint(c, r.Name))
				case arg != nil && flag != nil:
					return errf("$%s is declared as both an arg and a flag", r.Name)
				}
				used[r.Name] = true

				switch {
				case arg != nil && arg.Variadic && !r.Variadic:
					return errf("%s: arg %q is variadic, reference it as $%s...", at, r.Name, r.Name)
				case arg != nil && !arg.Variadic && r.Variadic:
					return errf("%s: arg %q is not variadic, reference it as $%s", at, r.Name, r.Name)
				case flag != nil && r.Variadic:
					return errf("%s: flag %q cannot be variadic", at, r.Name)
				}

				if r.Variadic && !r.Whole(elem) {
					return errf("%s: $%s... must be the whole element, not embedded in %q", at, r.Name, elem)
				}
				if flag != nil && flag.IsBool() && !r.Whole(elem) {
					return errf("%s: bool flag $%s must be the whole element, not embedded in %q", at, r.Name, elem)
				}
			}
		}
	}

	for _, a := range c.Args {
		if !used[a.Name] {
			return errf("arg %q is declared but never referenced in exec", a.Name)
		}
	}
	for _, f := range c.Flags {
		if !used[f.Name] {
			return errf("flag %q is declared but never referenced in exec", f.Name)
		}
	}
	return nil
}

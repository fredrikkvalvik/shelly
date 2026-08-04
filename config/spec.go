package config

import (
	"fmt"
	"strings"
)

// An arg or flag declares itself the way help output already reads, which is
// the notation the usage spec uses:
//
//	arg  "<pattern>"          required
//	arg  "[glob]"             optional
//	arg  "<files>..."         variadic, one or more
//	flag "-H --hidden"        boolean
//	flag "-m --max <count>"   takes a value

// parseArgSpec reads "<name>", "[name]" or either with a trailing "...".
func parseArgSpec(c ctx, spec string) (*Arg, error) {
	s := strings.TrimSpace(spec)

	a := &Arg{}
	if rest, ok := strings.CutSuffix(s, "..."); ok {
		a.Variadic = true
		s = rest
	}

	switch {
	case strings.HasPrefix(s, "<") && strings.HasSuffix(s, ">"):
		a.Required = true
		a.Name = s[1 : len(s)-1]
	case strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]"):
		a.Name = s[1 : len(s)-1]
	default:
		return nil, c.errf("arg %q must be <name> for a required one or [name] for an optional one", spec)
	}

	if !nameRe.MatchString(a.Name) {
		return nil, c.errf("arg %q must be %s", a.Name, nameHelp)
	}
	return a, nil
}

// parseFlagSpec reads "-s --long" or "--long <value>", in any order.
func parseFlagSpec(c ctx, spec string) (*Flag, error) {
	f := &Flag{Type: FlagBool}

	for _, tok := range strings.Fields(spec) {
		switch {
		case strings.HasPrefix(tok, "--"):
			if f.Name != "" {
				return nil, c.errf("flag %q gives two long names", spec)
			}
			f.Name = tok[2:]

		case strings.HasPrefix(tok, "-") && len(tok) > 1:
			if f.Short != "" {
				return nil, c.errf("flag %q gives two short names", spec)
			}
			f.Short = tok[1:]

		case strings.HasPrefix(tok, "<") && strings.HasSuffix(tok, ">"),
			strings.HasPrefix(tok, "[") && strings.HasSuffix(tok, "]"):
			// the value's name is only documentation; what matters is that
			// there is one, which is what makes this a string flag
			f.Type = FlagString

		default:
			return nil, c.errf("flag %q: %q is neither a name nor a value placeholder", spec, tok)
		}
	}

	if f.Name == "" {
		return nil, c.errf("flag %q has no long name", spec)
	}
	if !nameRe.MatchString(f.Name) {
		return nil, c.errf("flag %q must be %s", f.Name, nameHelp)
	}
	if f.Short != "" && len([]rune(f.Short)) != 1 {
		return nil, c.errf("flag %q has short %q, want a single character", spec, f.Short)
	}

	return f, nil
}

// usage() renders a flag back into the notation it was written in
func (f *Flag) usage() string {
	var b strings.Builder
	if f.Short != "" {
		fmt.Fprintf(&b, "-%s ", f.Short)
	}
	b.WriteString("--")
	b.WriteString(f.Name)
	if !f.IsBool() {
		b.WriteString(" <value>")
	}
	return b.String()
}

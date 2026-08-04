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

// A spec string may carry more than the spec: everything an entry usually
// needs fits on one line.
//
//	"-r --race        -> -race   # enable the race detector"
//	"-n --run <regex> -> -run    # only tests matching this regex"
//
// -> introduces the tokens the flag emits, and # the description. Tokens split
// on whitespace unless grouped by quotes, and both quote characters work: TOML
// leaves whichever one you did not spend on the string itself alone, so
//
//	"--preview -> --preview 'bat --color=always {}'"
//	'--preview -> --preview "bat --color=always {}"'
//
// both deliver the command as a single token with nothing escaped.
type entrySpec struct {
	Spec    string
	Pass    []string
	HasPass bool
	Doc     string
}

func splitEntry(c ctx, s string) (entrySpec, error) {
	var (
		e      entrySpec
		spec   []string
		cur    strings.Builder
		quote  rune
		quoted bool
		inPass bool
		runes  = []rune(s)
	)

	flush := func() {
		if cur.Len() == 0 && !quoted {
			return
		}
		tok := cur.String()
		cur.Reset()
		switch {
		case !inPass && !quoted && tok == "->":
			inPass, e.HasPass = true, true
		case inPass:
			e.Pass = append(e.Pass, tok)
		default:
			spec = append(spec, tok)
		}
		quoted = false
	}

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case quote != 0:
			if r == quote {
				quote, quoted = 0, true
				continue
			}
			cur.WriteRune(r)

		case r == '\'' || r == '"':
			quote = r

		// a description runs to the end of the line, so nothing after it is
		// scanned for markers
		case r == '#' && cur.Len() == 0 && !quoted:
			e.Doc = strings.TrimSpace(string(runes[i+1:]))
			e.Spec = strings.Join(spec, " ")
			return e, nil

		case r == ' ' || r == '\t':
			flush()

		default:
			cur.WriteRune(r)
		}
	}
	if quote != 0 {
		return e, c.errf("unterminated %c quote in %q", quote, s)
	}
	flush()

	e.Spec = strings.Join(spec, " ")
	return e, nil
}

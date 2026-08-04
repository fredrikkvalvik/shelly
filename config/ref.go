package config

import (
	"regexp"
	"strings"
)

// %{name}, or %{name...} for a variadic arg. %%{ is a literal %{.
//
// Angle brackets are deliberately not used here. In a declaration they already
// mean two other things — <name> marks a required arg, and a value placeholder
// is what makes a flag take a value — and a third meaning in the same file
// would be one too many. %{ is rare in real argv: printf's formats never
// contain it, and doubling only the opener leaves printf's own %% alone.
var refRe = regexp.MustCompile(`%%\{|%\{(` + namePat + `)(\.\.\.)?\}`)

// environment variable names are POSIX: no hyphens, unlike arg and flag names
const envNamePat = `[A-Za-z_][A-Za-z0-9_]*`

// what a config authored literal may contain once %{name} refs are removed
var literalRe = regexp.MustCompile(`%%\{|\$\$|\$\{(` + envNamePat + `)\}|\$(` + envNamePat + `)`)

// a %{name} reference inside an argv element
type Ref struct {
	Name     string
	Variadic bool
	Start    int // byte offsets into the element
	End      int
}

// Refs returns every reference in an argv element, in order.
func Refs(s string) []Ref {
	ms := refRe.FindAllStringSubmatchIndex(s, -1)
	refs := make([]Ref, 0, len(ms))

	for _, m := range ms {
		if m[2] < 0 {
			continue // matched %%{, not a reference
		}
		refs = append(refs, Ref{
			Name:     s[m[2]:m[3]],
			Variadic: m[4] >= 0,
			Start:    m[0],
			End:      m[1],
		})
	}

	return refs
}

// reports whether the reference spans the whole element, which is what
// distinguishes "%{glob}" from "--glob=%{glob}"
func (r Ref) Whole(s string) bool { return r.Start == 0 && r.End == len(s) }

// ExpandLiteral resolves config authored text: %%{ becomes %{, $$ becomes $,
// and $VAR or ${VAR} becomes the environment's value. It reports false when a
// referenced variable is unset, which drops the whole element rather than
// leaving a truncated path behind.
//
// Only config authored text goes through this. A value the user typed is never
// rescanned, so it cannot smuggle in an environment reference.
func ExpandLiteral(s string, env func(string) (string, bool)) (string, bool) {
	ms := literalRe.FindAllStringSubmatchIndex(s, -1)
	if len(ms) == 0 {
		return s, true
	}

	var b strings.Builder
	pos := 0

	for _, m := range ms {
		b.WriteString(s[pos:m[0]])
		pos = m[1]

		var name int
		switch {
		case m[2] >= 0:
			name = 2 // ${VAR}
		case m[4] >= 0:
			name = 4 // $VAR
		default:
			// %%{ or $$, a literal that loses one character
			b.WriteString(s[m[0]+1 : m[1]])
			continue
		}

		v, ok := env(s[m[name]:m[name+1]])
		if !ok {
			return "", false
		}
		b.WriteString(v)
	}
	b.WriteString(s[pos:])

	return b.String(), true
}

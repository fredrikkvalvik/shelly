package config

import (
	"regexp"
	"strings"
)

// $name or ${name}, with a ... suffix for a variadic arg. $$ escapes a literal
// dollar sign.
//
// a name may contain hyphens, so the bare form takes the longest match it can:
// "$x-post" is a reference named "x-post", not $x followed by "-post". write
// "${x}-post" when that is what you meant.
var refRe = regexp.MustCompile(
	`\$\$` +
		`|\$\{(` + namePat + `)(\.\.\.)?\}` +
		`|\$(` + namePat + `)(\.\.\.)?`)

// a $name reference inside an argv element
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
		var name, ellipsis int
		switch {
		case m[2] >= 0:
			name, ellipsis = 2, 4 // ${name}
		case m[6] >= 0:
			name, ellipsis = 6, 8 // $name
		default:
			continue // matched $$, not a reference
		}
		refs = append(refs, Ref{
			Name:     s[m[name]:m[name+1]],
			Variadic: m[ellipsis] >= 0,
			Start:    m[0],
			End:      m[1],
		})
	}

	return refs
}

// reports whether the reference spans the whole element, which is what
// distinguishes "$glob" from "--glob=$glob"
func (r Ref) Whole(s string) bool { return r.Start == 0 && r.End == len(s) }

// Unescape collapses $$ to $ in a literal segment.
func Unescape(s string) string { return strings.ReplaceAll(s, "$$", "$") }

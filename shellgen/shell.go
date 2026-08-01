package shellgen

import (
	"fmt"
	"slices"
	"strings"
)

// a shell shelly can generate integration code for
type Shell string

const (
	Zsh  Shell = "zsh"
	Bash Shell = "bash"
)

var shells = []Shell{Zsh, Bash}

func (s Shell) Valid() bool { return slices.Contains(shells, s) }

// NewShell converts a user supplied name, rejecting anything unsupported.
func NewShell(s string) (Shell, error) {
	sh := Shell(s)
	if !sh.Valid() {
		return "", fmt.Errorf("unsupported shell %q, want one of: %s", s, list(shells))
	}
	return sh, nil
}

func list(all []Shell) string {
	names := make([]string, 0, len(all))
	for _, v := range all {
		names = append(names, string(v))
	}
	return strings.Join(names, ", ")
}

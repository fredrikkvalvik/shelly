// Package trust records which project configs you have approved.
//
// Walking into a directory must not be enough to make its config run. A
// cloned repository can carry a .shelly that redefines ls or git, and a shelly
// config runs arbitrary programs by design, so a directory you merely visited
// getting to define commands is remote code execution.
//
// Approval is keyed on file contents rather than paths, so a config that
// changes after you approved it has to be approved again. That is the case
// that matters: a repository you already trust gaining a hostile config on the
// next pull.
package trust

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type Store struct {
	path   string
	hashes map[string]string // absolute config path to its approved digest
}

// Open reads the store, treating a missing one as empty.
func Open(path string) (*Store, error) {
	s := &Store{path: path, hashes: map[string]string{}}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}

	for line := range strings.Lines(string(data)) {
		digest, file, ok := strings.Cut(strings.TrimRight(line, "\n"), " ")
		if !ok || digest == "" || file == "" {
			continue
		}
		s.hashes[file] = digest
	}

	return s, nil
}

// Verify reports whether every file is present with the contents that were
// approved, and says which are not when they are not.
func (s *Store) Verify(files []string) (bool, string) {
	var unknown, changed []string

	for _, f := range files {
		want, known := s.hashes[f]
		got, err := hash(f)
		if err != nil {
			changed = append(changed, filepath.Base(f))
			continue
		}
		switch {
		case !known:
			unknown = append(unknown, filepath.Base(f))
		case want != got:
			changed = append(changed, filepath.Base(f))
		}
	}

	switch {
	case len(unknown) == 0 && len(changed) == 0:
		return true, ""
	case len(changed) > 0 && len(unknown) == 0:
		return false, fmt.Sprintf("changed since you trusted it: %s", strings.Join(changed, ", "))
	case len(changed) == 0 && len(files) == len(unknown):
		return false, "not trusted"
	default:
		return false, fmt.Sprintf("not trusted: %s", strings.Join(append(changed, unknown...), ", "))
	}
}

// Allow records the current contents of every file as approved.
func (s *Store) Allow(files []string) error {
	for _, f := range files {
		digest, err := hash(f)
		if err != nil {
			return err
		}
		abs, err := filepath.Abs(f)
		if err != nil {
			return err
		}
		s.hashes[abs] = digest
	}
	return s.save()
}

// Revoke drops every approval under dir. It reports how many it removed.
func (s *Store) Revoke(dir string) (int, error) {
	prefix := strings.TrimSuffix(dir, string(filepath.Separator)) + string(filepath.Separator)

	n := 0
	for f := range s.hashes {
		if strings.HasPrefix(f, prefix) {
			delete(s.hashes, f)
			n++
		}
	}
	if n == 0 {
		return 0, nil
	}
	return n, s.save()
}

// Files lists everything approved, in path order.
func (s *Store) Files() []string {
	out := make([]string, 0, len(s.hashes))
	for f := range s.hashes {
		out = append(out, f)
	}
	slices.Sort(out)
	return out
}

func (s *Store) Digest(file string) string { return s.hashes[file] }

// save writes through a temporary file, since several shells can be running
// this at once
func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}

	var b strings.Builder
	for _, f := range s.Files() {
		fmt.Fprintf(&b, "%s %s\n", s.hashes[f], f)
	}

	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".trust-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.WriteString(b.String()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}

func hash(file string) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

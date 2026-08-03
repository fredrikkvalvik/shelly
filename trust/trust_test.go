package trust

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func open(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestUnknownFilesAreNotTrusted(t *testing.T) {
	dir := t.TempDir()
	f := write(t, dir, "deploy.toml", "doc=\"x\"\n")

	s := open(t, filepath.Join(dir, "store"))
	ok, why := s.Verify([]string{f})
	if ok {
		t.Fatal("an unrecorded file must not be trusted")
	}
	if why != "not trusted" {
		t.Errorf("why = %q", why)
	}
}

func TestAllowThenVerify(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.toml", "doc=\"a\"\n")
	b := write(t, dir, "b.toml", "doc=\"b\"\n")
	store := filepath.Join(dir, "state", "store")

	s := open(t, store)
	if err := s.Allow([]string{a, b}); err != nil {
		t.Fatal(err)
	}
	if ok, why := s.Verify([]string{a, b}); !ok {
		t.Fatalf("not trusted after Allow: %s", why)
	}

	// approval must survive being written and read back
	if ok, why := open(t, store).Verify([]string{a, b}); !ok {
		t.Errorf("not trusted after reopening: %s", why)
	}
}

// the case the whole feature exists for: a repo you trusted, then pulled
func TestEditingATrustedFileRevokesIt(t *testing.T) {
	dir := t.TempDir()
	f := write(t, dir, "deploy.toml", "doc=\"honest\"\n")
	s := open(t, filepath.Join(dir, "store"))

	if err := s.Allow([]string{f}); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "deploy.toml", "doc=\"hostile\"\n")

	ok, why := s.Verify([]string{f})
	if ok {
		t.Fatal("an edited file must lose its approval")
	}
	if !strings.Contains(why, "changed since you trusted it") || !strings.Contains(why, "deploy.toml") {
		t.Errorf("why = %q, want it to name the changed file", why)
	}
}

// a new file appearing in an approved directory is not covered by it
func TestNewFileInATrustedDirIsNotTrusted(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.toml", "doc=\"a\"\n")
	s := open(t, filepath.Join(dir, "store"))
	if err := s.Allow([]string{a}); err != nil {
		t.Fatal(err)
	}

	b := write(t, dir, "b.toml", "doc=\"b\"\n")
	ok, why := s.Verify([]string{a, b})
	if ok {
		t.Fatal("a new file must not inherit the directory's approval")
	}
	if !strings.Contains(why, "b.toml") {
		t.Errorf("why = %q, want it to name the new file", why)
	}
}

func TestRestoringTheContentRestoresTrust(t *testing.T) {
	dir := t.TempDir()
	f := write(t, dir, "a.toml", "doc=\"a\"\n")
	s := open(t, filepath.Join(dir, "store"))
	s.Allow([]string{f})

	write(t, dir, "a.toml", "doc=\"changed\"\n")
	if ok, _ := s.Verify([]string{f}); ok {
		t.Fatal("edited file should not verify")
	}

	// keyed on contents, not on an edit counter
	write(t, dir, "a.toml", "doc=\"a\"\n")
	if ok, why := s.Verify([]string{f}); !ok {
		t.Errorf("restoring the contents should restore trust: %s", why)
	}
}

func TestRevoke(t *testing.T) {
	root := t.TempDir()
	pdir := filepath.Join(root, "p", ".shelly")
	other := filepath.Join(root, "q", ".shelly")
	os.MkdirAll(pdir, 0o755)
	os.MkdirAll(other, 0o755)

	a := write(t, pdir, "a.toml", "doc=\"a\"\n")
	b := write(t, other, "b.toml", "doc=\"b\"\n")
	store := filepath.Join(root, "store")

	s := open(t, store)
	s.Allow([]string{a, b})

	n, err := s.Revoke(pdir)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("revoked %d, want 1", n)
	}
	if ok, _ := s.Verify([]string{a}); ok {
		t.Error("revoked file still trusted")
	}
	// revoking one directory must not touch another
	if ok, why := s.Verify([]string{b}); !ok {
		t.Errorf("unrelated approval lost: %s", why)
	}

	if n, _ := s.Revoke(pdir); n != 0 {
		t.Errorf("revoking twice removed %d, want 0", n)
	}
}

func TestMissingFileIsNotTrusted(t *testing.T) {
	dir := t.TempDir()
	s := open(t, filepath.Join(dir, "store"))
	if ok, _ := s.Verify([]string{filepath.Join(dir, "gone.toml")}); ok {
		t.Error("a file that cannot be read must not be trusted")
	}
}

func TestStoreIsPrivate(t *testing.T) {
	dir := t.TempDir()
	f := write(t, dir, "a.toml", "x\n")
	store := filepath.Join(dir, "state", "store")

	if err := open(t, store).Allow([]string{f}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(store)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("store mode = %o, want 600", perm)
	}
}

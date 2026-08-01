package config

import (
	"os"
	"regexp"
	"testing"
)

const referencePath = "../docs/reference.toml"

// the reference doubles as a config, so every example in it stays honest
func TestReferenceLoads(t *testing.T) {
	tool, err := Load(referencePath)
	if err != nil {
		t.Fatalf("%s does not load: %v", referencePath, err)
	}
	if tool.Name != "reference" {
		t.Errorf("name = %q, want \"reference\"", tool.Name)
	}
	if len(tool.Root.Sub) == 0 {
		t.Error("the reference declares no commands")
	}
}

// a new key cannot ship undocumented
func TestReferenceDocumentsEveryKey(t *testing.T) {
	src, err := os.ReadFile(referencePath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)

	groups := map[string][]string{
		"root table":    rootKeys,
		"command table": reserved,
		"arg":           argKeys,
		"flag":          flagKeys,
		"exec stage":    execKeys,
	}

	for group, keys := range groups {
		for _, k := range keys {
			// "name = " or "{ name = ", not the word inside a comment
			used := regexp.MustCompile(`(?m)(^|[\[{,\s])` + regexp.QuoteMeta(k) + `\s*=`)
			if !used.MatchString(text) {
				t.Errorf("%s key %q is never used in %s", group, k, referencePath)
			}
		}
	}
}

// the enum variants are part of the schema too, so they need documenting
func TestReferenceDocumentsEveryVariant(t *testing.T) {
	src, err := os.ReadFile(referencePath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)

	for _, ft := range flagTypes {
		used := regexp.MustCompile(`type\s*=\s*"` + regexp.QuoteMeta(string(ft)) + `"`)
		if !used.MatchString(text) {
			t.Errorf("flag type %q is never used in %s", ft, referencePath)
		}
	}
	for _, b := range builtins {
		used := regexp.MustCompile(`complete\s*=\s*"` + regexp.QuoteMeta(string(b)) + `"`)
		if !used.MatchString(text) {
			t.Errorf("completion builtin %q is never used in %s", b, referencePath)
		}
	}
}

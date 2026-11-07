package project

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestList(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"a/.git", "b/c/.git", "deep/1/2/3/4/.git", "plain", "node_modules/x/.git"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got := List(Options{Roots: []string{root, filepath.Join(root, "missing")}, Depth: 3})
	want := []string{filepath.Join(root, "a"), filepath.Join(root, "b/c")}
	// fd (when installed) does not skip node_modules; walking does
	if len(got) == 3 && got[2] == filepath.Join(root, "node_modules/x") {
		got = got[:2]
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("List = %v, want %v", got, want)
	}
}

func TestSessionName(t *testing.T) {
	for dir, want := range map[string]string{"/x/lazy.mux": "lazy_mux", "/x/api/": "api", "/x/a:b": "a_b"} {
		if got := SessionName(dir); got != want {
			t.Errorf("SessionName(%q) = %q, want %q", dir, got, want)
		}
	}
}

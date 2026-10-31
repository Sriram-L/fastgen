package store

import (
	"os"
	"path/filepath"
	"testing"

	"fastgen/internal/runner"
)

func TestSave(t *testing.T) {
	dir := t.TempDir()
	r := runner.Result{Seed: 42, Input: []byte("in\n"), Expected: []byte("exp\n"), Got: []byte("got\n")}
	paths, err := Save(dir, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 3 {
		t.Fatalf("got %d paths, want 3", len(paths))
	}
	for _, p := range []struct {
		name string
		want string
	}{
		{"fail_42.in", "in\n"},
		{"fail_42.expected", "exp\n"},
		{"fail_42.got", "got\n"},
	} {
		got, err := os.ReadFile(filepath.Join(dir, p.name))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != p.want {
			t.Fatalf("%s = %q, want %q", p.name, got, p.want)
		}
	}
}

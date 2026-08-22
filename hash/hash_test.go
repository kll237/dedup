package hash

import (
	"os"
	"path/filepath"
	"testing"

	"dedup/result"
)

func TestFindExact(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	c := filepath.Join(dir, "c.txt")
	if err := os.WriteFile(a, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("hello world"), 0644); err != nil { // duplicate of a
		t.Fatal(err)
	}
	if err := os.WriteFile(c, []byte("different"), 0644); err != nil {
		t.Fatal(err)
	}

	files := []result.FileRef{
		{Path: a, Size: 11},
		{Path: b, Size: 11},
		{Path: c, Size: 9},
	}
	groups := FindExact(files, 2, nil)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0].Files) != 2 {
		t.Fatalf("expected 2 files in group, got %d", len(groups[0].Files))
	}
}

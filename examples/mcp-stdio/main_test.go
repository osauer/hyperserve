package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSamplesPreserveExistingFiles(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.json")
	for _, p := range []string{filepath.Join(root, "hello.txt"), outside} {
		if err := os.WriteFile(p, []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(root, "test.json")); err != nil {
		t.Fatal(err)
	}
	if err := createSampleFiles(root); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(root, "hello.txt"), outside} {
		b, err := os.ReadFile(p)
		if err != nil || string(b) != "keep" {
			t.Errorf("existing file changed: %s: %q %v", p, b, err)
		}
	}
}
func TestSamplesCanInitializeTwice(t *testing.T) {
	root := t.TempDir()
	for range 2 {
		if err := createSampleFiles(root); err != nil {
			t.Fatal(err)
		}
	}
	if b, err := os.ReadFile(filepath.Join(root, "hello.txt")); err != nil || len(b) == 0 {
		t.Fatalf("sample absent: %v", err)
	}
}

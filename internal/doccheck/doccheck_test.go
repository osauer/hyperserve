package doccheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

var markdownLink = regexp.MustCompile(`\[[^]]+\]\(([^)]+)\)`)

func TestEntryPointLinksResolve(t *testing.T) {
	root := repoRoot(t)
	readmes := []string{"README.md", "examples/README.md"}
	err := filepath.WalkDir(filepath.Join(root, "examples"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "README.md" {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		readmes = append(readmes, relative)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, relative := range readmes {
		t.Run(relative, func(t *testing.T) {
			path := filepath.Join(root, relative)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", relative, err)
			}

			for _, match := range markdownLink.FindAllStringSubmatch(string(data), -1) {
				target := strings.Trim(match[1], "<>")
				if target == "" || strings.HasPrefix(target, "#") || strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
					continue
				}
				target, _, _ = strings.Cut(target, "#")
				resolved := filepath.Clean(filepath.Join(filepath.Dir(path), filepath.FromSlash(target)))
				if _, err := os.Stat(resolved); err != nil {
					t.Errorf("link %q resolves to %s: %v", match[1], resolved, err)
				}
			}
		})
	}
}

func TestExampleReadmesDoNotRegressToKnownStaleClaims(t *testing.T) {
	root := repoRoot(t)
	stale := []string{
		"FIPS 140-3 Compliance",
		"JWT (RS256), API keys, Basic auth",
		"X-XSS-Protection: 1; mode=block",
		"localhost:8081/healthz",
		"localhost:8080/healthz",
		"server, err := server.NewServer",
	}

	err := filepath.WalkDir(filepath.Join(root, "examples"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "README.md" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, phrase := range stale {
			if strings.Contains(string(data), phrase) {
				t.Errorf("%s contains stale claim %q", filepath.ToSlash(path), phrase)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate doccheck source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

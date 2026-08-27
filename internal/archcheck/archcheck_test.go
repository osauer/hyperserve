package archcheck

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"
)

const modulePath = "github.com/osauer/hyperserve/v2"

type listedPackage struct {
	ImportPath string
	Name       string
	Imports    []string
}

func TestCanonicalPublicPackageGraph(t *testing.T) {
	want := map[string]struct {
		name    string
		imports []string
	}{
		modulePath:                  {name: "hyperserve", imports: []string{modulePath + "/mcp", modulePath + "/websocket"}},
		modulePath + "/auth":        {name: "auth"},
		modulePath + "/jsonrpc":     {name: "jsonrpc"},
		modulePath + "/mcp":         {name: "mcp", imports: []string{modulePath + "/jsonrpc"}},
		modulePath + "/mcp/builtin": {name: "builtin", imports: []string{modulePath, modulePath + "/mcp"}},
		modulePath + "/ratelimit":   {name: "ratelimit"},
		modulePath + "/websocket":   {name: "websocket"},
	}

	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "GOWORK=off")
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list package graph: %v\n%s", err, exitErr.Stderr)
		}
		t.Fatal(err)
	}

	seen := make(map[string]bool)
	decoder := json.NewDecoder(strings.NewReader(string(output)))
	for {
		var pkg listedPackage
		if err := decoder.Decode(&pkg); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("decode go list output: %v", err)
		}
		expected, stable := want[pkg.ImportPath]
		if !stable {
			if isNonPublicPackage(pkg.ImportPath) {
				continue
			}
			t.Errorf("unexpected public package %s", pkg.ImportPath)
			continue
		}
		seen[pkg.ImportPath] = true
		if pkg.Name != expected.name {
			t.Errorf("%s package name = %q, want %q", pkg.ImportPath, pkg.Name, expected.name)
		}
		gotImports := stablePackageImports(pkg.Imports, want)
		wantImports := slices.Clone(expected.imports)
		sort.Strings(wantImports)
		if !slices.Equal(gotImports, wantImports) {
			t.Errorf("%s direct first-party imports = %v, want %v", pkg.ImportPath, gotImports, wantImports)
		}
	}

	for importPath := range want {
		if !seen[importPath] {
			t.Errorf("canonical public package missing: %s", importPath)
		}
	}
}

func stablePackageImports(imports []string, stable map[string]struct {
	name    string
	imports []string
}) []string {
	result := make([]string, 0)
	for _, importPath := range imports {
		if _, ok := stable[importPath]; ok {
			result = append(result, importPath)
		}
	}
	sort.Strings(result)
	return result
}

func isNonPublicPackage(importPath string) bool {
	for _, prefix := range []string{
		modulePath + "/benchmarks/",
		modulePath + "/cmd/",
		modulePath + "/examples/",
		modulePath + "/internal/",
	} {
		if strings.HasPrefix(importPath, prefix) {
			return true
		}
	}
	return false
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate architecture check source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

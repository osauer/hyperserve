package doccheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"
)

var (
	markdownLink  = regexp.MustCompile(`\[[^]]+\]\(([^)]+)\)`)
	historicalADR = regexp.MustCompile(`^docs/(?:000[1-9]|001[0-3])-.*\.md$`)
)

type staleSurfaceRule struct {
	name         string
	pattern      *regexp.Regexp
	markdownOnly bool
	allow        func(relative, line string) bool
}

func TestCurrentMarkdownLinksResolve(t *testing.T) {
	root := repoRoot(t)
	for _, relative := range currentAuthorityFiles(t, root) {
		if filepath.Ext(relative) != ".md" {
			continue
		}
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

func TestCurrentAuthorityUsesCanonicalAPI(t *testing.T) {
	root := repoRoot(t)
	rules := []staleSurfaceRule{
		{
			name:    "old public package path",
			pattern: regexp.MustCompile(`(?:github\.com/osauer/hyperserve/v2/)?pkg/(?:server|auth|jsonrpc|mcp|websocket)\b`),
			allow: func(relative, line string) bool {
				return relative == "docs/0014-root-package-and-concern-subpackages.md" &&
					strings.Contains(strings.ToLower(line), "intermediate public layout")
			},
		},
		{
			name:    "old HyperServe constructor",
			pattern: regexp.MustCompile(`\b(?:hyperserve|server|serverpkg)\.NewServer\s*\(`),
		},
		{
			name:         "old HyperServe constructor name",
			pattern:      regexp.MustCompile(`\bNewServer\b`),
			markdownOnly: true,
			allow: func(_ string, line string) bool {
				lower := strings.ToLower(line)
				for _, explicitMigration := range []string{
					"app.newserver",
					"do not add",
					"does not recreate",
					"no `newserver`",
					"not retained",
					"rename",
					"removed",
					"retired",
				} {
					if strings.Contains(lower, explicitMigration) {
						return true
					}
				}
				return false
			},
		},
		{
			name:    "retired server-owned limiter API",
			pattern: regexp.MustCompile(`\b(?:RateLimitMiddleware|WithRateLimit|ClientLimiterCount)\b`),
		},
		{
			name:    "retired server-owned limiter option",
			pattern: regexp.MustCompile(`\b(?:options|opts|server)\.RateLimit\b|\.Options\(\)\.RateLimit\b`),
		},
		{
			name:    "retired server-owned limiter config key",
			pattern: regexp.MustCompile(`["'](?:rate_limit|burst)["']\s*:`),
		},
		{
			name:    "retired burst environment name",
			pattern: regexp.MustCompile(`\bHS_BURST_LIMIT\b`),
			allow: func(relative, line string) bool {
				if relative == "internal/scaffold/templates/internal/app/config_test.go.tmpl" {
					return true
				}
				lower := strings.ToLower(line)
				return strings.Contains(lower, "retired") || strings.Contains(lower, "reject")
			},
		},
		{name: "unsupported FIPS claim", pattern: regexp.MustCompile(`FIPS 140-3 Compliance`)},
		{name: "unsupported bundled auth claim", pattern: regexp.MustCompile(`JWT \(RS256\), API keys, Basic auth`)},
		{name: "obsolete XSS header claim", pattern: regexp.MustCompile(`X-XSS-Protection: 1; mode=block`)},
		{name: "stale health listener example", pattern: regexp.MustCompile(`localhost:808[01]/healthz`)},
	}

	for _, relative := range currentAuthorityFiles(t, root) {
		path := filepath.Join(root, relative)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		isMarkdown := filepath.Ext(relative) == ".md"
		lines := strings.Split(string(data), "\n")
		for lineIndex, line := range lines {
			surrounding := line
			if lineIndex > 0 {
				surrounding = lines[lineIndex-1] + " " + surrounding
			}
			if lineIndex+1 < len(lines) {
				surrounding += " " + lines[lineIndex+1]
			}
			for _, rule := range rules {
				if rule.markdownOnly && !isMarkdown {
					continue
				}
				if rule.allow != nil && rule.allow(relative, surrounding) {
					continue
				}
				if rule.pattern.MatchString(line) {
					t.Errorf("%s:%d contains %s: %q", relative, lineIndex+1, rule.name, strings.TrimSpace(line))
				}
			}
		}
	}
}

func TestAPIStabilityDisclosesControlledV21Reset(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "docs", "API_STABILITY.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read API stability policy: %v", err)
	}
	content := string(data)
	lower := strings.ToLower(content)

	for _, required := range []string{
		"v2.1.0",
		"2026-08-27",
		"narrow",
		"controlled",
		"compatibility reset",
		"github.com/osauer/hyperserve/v2@v2.0.3",
	} {
		if !strings.Contains(lower, strings.ToLower(required)) {
			t.Errorf("API stability policy must disclose %q", required)
		}
	}

	futureMajor := regexp.MustCompile(`(?is)(?:after v2\.1\.0|later|subsequent|future).{0,160}breaking.{0,160}(?:new|future) major`)
	if !futureMajor.MatchString(content) {
		t.Error("API stability policy must restore the rule that later breaking changes require a future major version")
	}

	reset := strings.Index(lower, "v2.1.0")
	semver := strings.Index(lower, "semantic version")
	if semver >= 0 && reset > semver && !strings.Contains(lower[:semver], "normally") {
		t.Error("a SemVer promise before the v2.1.0 reset must be explicitly qualified as the normal rule")
	}
}

func TestHistoricalAuthorityExemptions(t *testing.T) {
	for _, relative := range []string{
		"CHANGELOG.md",
		"docs/MIGRATING_V2.md",
		"docs/MIGRATING_V2_1.md",
		"docs/0009-single-package-architecture.md",
		"docs/0013-example-historical-adr.md",
	} {
		if !isHistoricalAuthority(relative) {
			t.Errorf("%s must remain exempt as explicit migration/history material", relative)
		}
	}
	if isHistoricalAuthority("docs/API_STABILITY.md") {
		t.Error("current API stability policy must not be exempt from canonical-surface checks")
	}
	if isHistoricalAuthority("docs/0014-root-package-and-concern-subpackages.md") {
		t.Error("ADR-0014 establishes the current architecture and must be scanned")
	}

	foundCurrentADR := slices.Contains(currentAuthorityFiles(t, repoRoot(t)), "docs/0014-root-package-and-concern-subpackages.md")
	if !foundCurrentADR {
		t.Error("ADR-0014 is not included in current-authority scanning")
	}
}

func currentAuthorityFiles(t *testing.T, root string) []string {
	t.Helper()

	files := map[string]struct{}{}
	for _, relative := range []string{
		"README.md",
		"ARCHITECTURE.md",
		"PROJECT_STRUCTURE.md",
		"CONTRIBUTING.md",
		"CLAUDE.md",
		"SECURITY.md",
		".github/release-notes-template.md",
		"tools/dependencies.go",
	} {
		files[relative] = struct{}{}
	}

	for _, relativeRoot := range []string{
		"benchmarks",
		"docs",
		"examples",
		"internal/scaffold/templates",
		"internal/validate",
	} {
		walkRoot := filepath.Join(root, relativeRoot)
		err := filepath.WalkDir(walkRoot, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			if isHistoricalAuthority(relative) || !isCurrentTextFile(relative) {
				return nil
			}
			files[relative] = struct{}{}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", relativeRoot, err)
		}
	}

	result := make([]string, 0, len(files))
	for relative := range files {
		result = append(result, relative)
	}
	sort.Strings(result)
	return result
}

func isHistoricalAuthority(relative string) bool {
	if relative == "CHANGELOG.md" || strings.HasPrefix(relative, "docs/MIGRATING") {
		return true
	}
	return historicalADR.MatchString(relative)
}

func isCurrentTextFile(relative string) bool {
	for _, suffix := range []string{".go", ".html", ".js", ".json", ".md", ".mod", ".sh", ".tmpl", ".yaml", ".yml"} {
		if strings.HasSuffix(relative, suffix) {
			return true
		}
	}
	return false
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate doccheck source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

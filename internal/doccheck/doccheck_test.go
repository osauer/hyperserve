package doccheck

import (
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

var (
	markdownLink  = regexp.MustCompile(`\[[^]]+\]\(([^)]+)\)`)
	markdownTitle = regexp.MustCompile(`(?m)^ {0,3}#{1,6}[ \t]+(.+?)[ \t]*#*[ \t]*$`)
	explicitID    = regexp.MustCompile(`(?i)<(?:a|[a-z][a-z0-9-]*)[^>]+(?:id|name)=["']([^"']+)["'][^>]*>`)
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
				if target == "" || strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
					continue
				}
				linkPath, fragment, _ := strings.Cut(target, "#")
				resolved := path
				if linkPath != "" {
					resolved = filepath.Clean(filepath.Join(filepath.Dir(path), filepath.FromSlash(linkPath)))
				}
				info, err := os.Stat(resolved)
				if err != nil {
					t.Errorf("link %q resolves to %s: %v", match[1], resolved, err)
					continue
				}
				if info.IsDir() {
					if fragment == "" {
						continue
					}
					resolved = filepath.Join(resolved, "README.md")
					if _, err := os.Stat(resolved); err != nil {
						t.Errorf("link %q resolves to directory without README.md: %v", match[1], err)
						continue
					}
				}
				if fragment != "" && filepath.Ext(resolved) == ".md" {
					decoded, err := url.PathUnescape(fragment)
					if err != nil {
						t.Errorf("link %q has invalid escaped anchor: %v", match[1], err)
						continue
					}
					if !markdownAnchors(t, resolved)[decoded] {
						t.Errorf("link %q has no heading anchor %q in %s", match[1], decoded, resolved)
					}
				}
			}
		})
	}
}

func markdownAnchors(t *testing.T, path string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Markdown target %s: %v", path, err)
	}
	anchors := make(map[string]bool)
	seen := make(map[string]int)
	inFence := false
	fence := ""
	for line := range strings.SplitSeq(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			marker := trimmed[:3]
			if !inFence {
				inFence, fence = true, marker
			} else if marker == fence {
				inFence, fence = false, ""
			}
			continue
		}
		if inFence {
			continue
		}
		if match := markdownTitle.FindStringSubmatch(line); match != nil {
			base := githubHeadingSlug(match[1])
			if base != "" {
				slug := base
				if duplicate := seen[base]; duplicate > 0 {
					slug += "-" + strconv.Itoa(duplicate)
				}
				seen[base]++
				anchors[slug] = true
			}
		}
		for _, match := range explicitID.FindAllStringSubmatch(line, -1) {
			anchors[match[1]] = true
		}
	}
	return anchors
}

func TestMarkdownAnchorsIgnoreFencedHeadings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "anchors.md")
	data := []byte("# Real heading\n\n```text\n# Not a heading\n```\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	anchors := markdownAnchors(t, path)
	if !anchors["real-heading"] {
		t.Fatal("real heading was not indexed")
	}
	if anchors["not-a-heading"] {
		t.Fatal("fenced code was indexed as a heading")
	}
}

func githubHeadingSlug(title string) string {
	// This covers the GitHub heading forms used by the repository: lowercase,
	// punctuation removal, and whitespace-to-hyphen conversion. Inline code
	// delimiters are punctuation, so their contents naturally remain.
	var slug strings.Builder
	for _, r := range strings.ToLower(title) {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r), r == '_', r == '-':
			slug.WriteRune(r)
		case unicode.IsSpace(r):
			slug.WriteByte('-')
		}
	}
	return slug.String()
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
		{name: "removed MCP log-level action", pattern: regexp.MustCompile(`\bset_log_level\b`)},
		{name: "removed MCP resource builder", pattern: regexp.MustCompile(`\b(?:mcp\.)?NewResource\s*\(`)},
		{name: "unqualified MCP tool builder", pattern: regexp.MustCompile(`(?:^|[^[:alnum:]_.])NewTool\s*\(`)},
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

func TestReleaseAuthoringRequiresFutureMajorAfterV21(t *testing.T) {
	root := repoRoot(t)

	changelog, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		t.Fatalf("read changelog: %v", err)
	}
	preamble, _, ok := strings.Cut(string(changelog), "\n## [")
	if !ok {
		t.Fatal("CHANGELOG.md has no release entries")
	}

	stub, err := os.ReadFile(filepath.Join(root, "scripts", "changelog-stub.sh"))
	if err != nil {
		t.Fatalf("read changelog stub: %v", err)
	}

	for name, content := range map[string]string{
		"CHANGELOG preamble": preamble,
		"changelog stub":     string(stub),
	} {
		t.Run(name, func(t *testing.T) {
			lower := strings.Join(strings.Fields(strings.ToLower(content)), " ")
			for _, required := range []string{"after v2.1.0", "new major version", "major module path"} {
				if !strings.Contains(lower, required) {
					t.Errorf("must require %q for post-v2.1 release authoring", required)
				}
			}
			for _, stale := range []string{"v1 breakage", "future /v2"} {
				if strings.Contains(lower, stale) {
					t.Errorf("contains stale release guidance %q", stale)
				}
			}
		})
	}
}

func TestMCPRemoteAccessUsesHTTPOverSSHForward(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "MCP_GUIDE.md"))
	if err != nil {
		t.Fatalf("read MCP guide: %v", err)
	}
	content := string(data)

	for _, required := range []string{
		"ssh -N -o StrictHostKeyChecking=yes -L 127.0.0.1:18080:127.0.0.1:8080 prod-server",
		`"url": "http://127.0.0.1:18080/mcp"`,
		"does not add application",
	} {
		if !strings.Contains(content, required) {
			t.Errorf("MCP remote-access guide must contain %q", required)
		}
	}
	if strings.Contains(content, `"command": "ssh"`) {
		t.Error("MCP remote-access guide must not present ssh plus curl as a stdio MCP server")
	}
}

func TestV1MigrationScanTargetsHyperServeNewServer(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "MIGRATING_V2.md"))
	if err != nil {
		t.Fatalf("read v1 migration guide: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, `(?:hyperserve|server|serverpkg)\.NewServer\(`) {
		t.Error("v1 migration scan must target known HyperServe NewServer qualifiers")
	}
	if strings.Contains(content, "|NewServer|") {
		t.Error("v1 migration scan must not flag unrelated app.NewServer or httptest.NewServer calls")
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

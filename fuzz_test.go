package hyperserve

import (
	"reflect"
	"testing"
)

// FuzzCORSOriginMatch tests the CORS allowed-origin matcher against
// adversarial (allowed, origin) pairs. Any panic in matchOrigin or
// resolveAllowedOrigin is a bug; results should always be one of the
// documented shapes ("", false) / (origin, true) / ("*", true).
func FuzzCORSOriginMatch(f *testing.F) {
	seeds := []struct {
		allowed string
		origin  string
	}{
		{"https://example.com", "https://example.com"},
		{"https://*.example.com", "https://api.example.com"},
		{"https://api:*", "https://api:443"},
		{"*", "https://anything"},
		{"", ""},
		{"https://a.com,https://b.com", "https://a.com"},
		{"https://[::1]:8080", "https://[::1]:8080"},
	}
	for _, s := range seeds {
		f.Add(s.allowed, s.origin)
	}
	f.Fuzz(func(t *testing.T, allowed, origin string) {
		cors := normalizeCORSOptions(&CORSOptions{AllowedOrigins: []string{allowed}})
		if cors == nil {
			return
		}
		got, ok := cors.resolveAllowedOrigin(origin)
		// Wildcard never echoes the origin back — that combination would be
		// the bug class the security fix closed (see cors.go normalizeCORSOptions).
		if ok && got != "*" && cors.AllowCredentials {
			if got == "" {
				t.Errorf("non-wildcard credential match returned empty origin (allowed=%q origin=%q)", allowed, origin)
			}
		}
	})
}

// FuzzValidateEmail fuzzes the validator's email rule. It should always
// terminate quickly and never panic regardless of the input string.
func FuzzValidateEmail(f *testing.F) {
	seeds := []string{
		"",
		"a@b.co",
		"@",
		"@b.co",
		"a@",
		"a@b",
		"a@.b",
		"a@b.",
		"x y@b.co",
		"with-space @b.co",
		"a@b.c\n",
		"verylongprefix" + repeat("a", 1024) + "@example.com",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		v := struct {
			E string `validate:"email"`
		}{E: s}
		err1 := Validate(&v)
		err2 := Validate(&v)
		if reflect.TypeOf(err1) != reflect.TypeOf(err2) {
			t.Errorf("Validate(%q) non-deterministic: %v vs %v", s, err1, err2)
		}
	})
}

func repeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for range n {
		out = append(out, s...)
	}
	return string(out)
}

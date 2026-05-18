package server

import (
	"path"
	"slices"
	"strconv"
	"strings"
)

// CORSOptions captures configuration for Cross-Origin Resource Sharing handling.
type CORSOptions struct {
	AllowedOrigins   []string `json:"allowed_origins,omitempty"`
	AllowedMethods   []string `json:"allowed_methods,omitempty"`
	AllowedHeaders   []string `json:"allowed_headers,omitempty"`
	ExposeHeaders    []string `json:"expose_headers,omitempty"`
	AllowCredentials bool     `json:"allow_credentials,omitempty"`
	MaxAgeSeconds    int      `json:"max_age_seconds,omitempty"`
}

var (
	defaultCORSMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	defaultCORSHeaders = []string{"Content-Type", "Authorization"}
	defaultCORSMaxAge  = 600
)

func normalizeCORSOptions(opts *CORSOptions) *CORSOptions {
	if opts == nil {
		return nil
	}

	copy := &CORSOptions{
		AllowedOrigins:   sanitizeTokens(opts.AllowedOrigins, false),
		AllowedMethods:   sanitizeTokens(opts.AllowedMethods, true),
		AllowedHeaders:   sanitizeTokens(opts.AllowedHeaders, false),
		ExposeHeaders:    sanitizeTokens(opts.ExposeHeaders, false),
		AllowCredentials: opts.AllowCredentials,
		MaxAgeSeconds:    opts.MaxAgeSeconds,
	}

	// Refuse the wildcard + credentials combination. The Fetch spec forbids it
	// because reflecting the request Origin while sending credentials defeats
	// the same-origin policy. Drop credentials so the server cannot be
	// configured into a state where any site can read authenticated responses.
	if copy.AllowCredentials && slices.Contains(copy.AllowedOrigins, "*") {
		copy.AllowCredentials = false
		logger.Warn("CORS: AllowCredentials=true is incompatible with AllowedOrigins=[\"*\"] (Fetch spec); credentials disabled. Specify exact origins to allow credentials.")
	}

	if len(copy.AllowedMethods) == 0 {
		copy.AllowedMethods = slices.Clone(defaultCORSMethods)
	}
	if len(copy.AllowedHeaders) == 0 {
		copy.AllowedHeaders = slices.Clone(defaultCORSHeaders)
	}
	if copy.MaxAgeSeconds <= 0 {
		copy.MaxAgeSeconds = defaultCORSMaxAge
	}

	slices.Sort(copy.AllowedOrigins)
	slices.Sort(copy.AllowedMethods)
	slices.Sort(copy.AllowedHeaders)
	slices.Sort(copy.ExposeHeaders)

	return copy
}

func sanitizeTokens(values []string, upper bool) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, raw := range values {
		token := strings.TrimSpace(raw)
		if token == "" {
			continue
		}
		key := token
		if upper {
			key = strings.ToUpper(token)
			token = key
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, token)
	}
	return result
}

func (c *CORSOptions) resolveAllowedOrigin(origin string) (string, bool) {
	if c == nil || len(c.AllowedOrigins) == 0 {
		return "", false
	}
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return "", false
	}

	lowerOrigin := strings.ToLower(origin)
	for _, allowed := range c.AllowedOrigins {
		if allowed == "*" {
			// AllowCredentials with "*" is refused at normalize time; here we
			// can safely return the wildcard.
			return "*", true
		}
		if matchOrigin(allowed, lowerOrigin) {
			return origin, true
		}
	}
	return "", false
}

func matchOrigin(allowed string, originLower string) bool {
	allowed = strings.TrimSpace(allowed)
	if allowed == "" {
		return false
	}

	lowerAllowed := strings.ToLower(allowed)
	if lowerAllowed == originLower {
		return true
	}

	if strings.Contains(lowerAllowed, "*") {
		if ok, err := path.Match(lowerAllowed, originLower); err == nil && ok {
			return true
		}
	}

	if strings.HasSuffix(lowerAllowed, ":*") {
		prefix := strings.TrimSuffix(lowerAllowed, "*")
		return strings.HasPrefix(originLower, prefix)
	}

	return false
}

func joinTokens(tokens []string) string {
	return strings.Join(tokens, ", ")
}

func formatMaxAge(seconds int) string {
	if seconds <= 0 {
		seconds = defaultCORSMaxAge
	}
	return strconv.Itoa(seconds)
}

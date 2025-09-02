package hyperserve

import (
	"net/http"
	"net/url"
	"strings"
)

// defaultCheckOrigin provides a safe default origin check that enforces same-origin policy
func defaultCheckOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// No origin header - could be a non-browser client
		// This is potentially unsafe, so we reject by default
		return false
	}
	
	originURL, err := url.Parse(origin)
	if err != nil {
		return false
	}
	
	// Check if the origin matches the request host
	return equalASCIIFold(originURL.Host, r.Host)
}

// checkOriginWithAllowedList checks if the origin is in the allowed list
func checkOriginWithAllowedList(allowedOrigins []string) func(r *http.Request) bool {
	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return false
		}
		
		// Check against allowed origins
		for _, allowed := range allowedOrigins {
			if allowed == "*" {
				// Wildcard - accept all origins (use with caution!)
				return true
			}
			if allowed == origin {
				return true
			}
			// Support wildcard subdomains like "*.example.com"
			if strings.HasPrefix(allowed, "*.") {
				suffix := allowed[1:] // Remove the "*"
				originURL, err := url.Parse(origin)
				if err != nil {
					continue
				}
				if strings.HasSuffix(originURL.Host, suffix) {
					return true
				}
			}
		}
		
		return false
	}
}

// equalASCIIFold returns true if s1 and s2 are equal, ASCII case-insensitively
func equalASCIIFold(s1, s2 string) bool {
	if len(s1) != len(s2) {
		return false
	}
	for i := 0; i < len(s1); i++ {
		c1 := s1[i]
		c2 := s2[i]
		if c1|0x20 != c2|0x20 {
			// Fast path: if they're different even when lower-cased
			if c1 < 'A' || c1 > 'Z' || c2 < 'A' || c2 > 'Z' {
				// At least one is not a letter, so they must match exactly
				if c1 != c2 {
					return false
				}
			} else {
				// Both are letters, check if they match case-insensitively
				if c1|0x20 != c2|0x20 {
					return false
				}
			}
		}
	}
	return true
}
package websocket

import (
	"net/http"
	"net/url"
	"strings"
)

// DefaultCheckOrigin requires a single HTTP(S) Origin matching the request's
// scheme, host and effective port. TLS determines the request scheme; forwarding
// headers are not trusted. TLS-terminating proxies need an explicit origin policy.
func DefaultCheckOrigin(r *http.Request) bool {
	origins := r.Header.Values("Origin")
	if len(origins) != 1 || origins[0] == "" {
		// No origin header - could be a non-browser client
		// This is potentially unsafe, so we reject by default
		return false
	}

	originURL, err := url.Parse(origins[0])
	if err != nil || originURL.Host == "" || originURL.User != nil ||
		originURL.Path != "" || originURL.RawQuery != "" || originURL.ForceQuery ||
		strings.Contains(origins[0], "#") || originURL.Opaque != "" {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	requestURL := &url.URL{Scheme: scheme, Host: r.Host}
	return originURL.Scheme == scheme && equalASCIIFold(websocketAddress(originURL), websocketAddress(requestURL))
}

// CheckOriginWithAllowedList checks if the origin is in the allowed list
func CheckOriginWithAllowedList(allowedOrigins []string) func(r *http.Request) bool {
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
	for i := range len(s1) {
		c1 := s1[i]
		c2 := s2[i]
		if c1 >= 'A' && c1 <= 'Z' {
			c1 += 'a' - 'A'
		}
		if c2 >= 'A' && c2 <= 'Z' {
			c2 += 'a' - 'A'
		}
		if c1 != c2 {
			return false
		}
	}
	return true
}

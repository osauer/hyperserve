// Package ratelimit provides bounded, per-client HTTP rate limiting.
//
// New creates a middleware gate. Reusing the returned middleware shares one
// quota namespace; calling New again creates an independent namespace. The
// default client identity is the normalized transport peer in Request.RemoteAddr
// and does not trust forwarding headers.
//
// Client storage is finite: zero IdleTTL and MaxClients values select the
// documented defaults in Config. IdleTTL is a minimum retention period; a
// bucket is always retained long enough to refill its full burst so pruning
// cannot reset a slow quota early. Idle buckets are pruned during requests;
// the gate starts no cleanup goroutine. When the pool is full, existing
// clients keep their buckets and a new client receives 429 Too Many Requests.
// Quota rejections also return 429 with Retry-After and RateLimit-Reset derived
// from the token schedule. Capacity rejections use the effective retention as
// a conservative backoff.
//
// Applications behind known reverse proxies can opt in to X-Forwarded-For
// parsing with TrustedProxyClientKey. Other forwarding headers are ignored.
//
// A gate can be placed in front of any standard net/http handler:
//
//	gate, err := ratelimit.New(ratelimit.Config{
//		RequestsPerSecond: 20,
//		Burst:             40,
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//	handler := gate(next)
package ratelimit

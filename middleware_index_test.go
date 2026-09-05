package hyperserve

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestWidePrefixMiddlewareOrder(t *testing.T) {
	for _, universal := range []string{"*", "", "/"} {
		t.Run(fmt.Sprintf("root_%q", universal), func(t *testing.T) {
			var got []string
			reg := newMiddlewareRegistry(nil)
			add := func(prefix string) {
				reg.Add(prefix, MiddlewareStack{func(next http.Handler) http.Handler {
					return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = append(got, prefix); next.ServeHTTP(w, r) })
				}})
			}
			add(universal)
			for i := range 32 {
				add(fmt.Sprintf("/api-%02d", i))
				add(fmt.Sprintf("/api/child-%02d", i))
			}
			for _, p := range []string{"/api", "/api/", "/api/child-01/deep"} {
				add(p)
			}
			mux := http.NewServeMux()
			mux.HandleFunc("/", func(http.ResponseWriter, *http.Request) {})
			h := reg.compile(mux)
			for _, path := range []string{"/unrelated", "/api", "/api/", "/api/users", "/api-01", "/api-01/z", "/api-010", "/api/child-01/deep/x", "/api/child-010"} {
				got = nil
				h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
				want := []string{universal}
				for _, p := range reg.sortedRoutes {
					if p != universal && pathPrefixMatches(path, p) {
						want = append(want, p)
					}
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("%q: got %v want %v", path, got, want)
				}
			}
		})
	}
}

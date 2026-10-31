package hyperserve_test

import (
	"testing"

	"github.com/osauer/hyperserve/v2"
)

func TestCanonicalRootConstructor(t *testing.T) {
	app, err := hyperserve.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := app.Options().Addr; got != ":8080" {
		t.Fatalf("default address = %q, want :8080", got)
	}
}

package mcp

import "testing"

func TestUniqueJSONNestedAndEscapedNames(t *testing.T) {
	for _, input := range []string{`{"a":1,"\u0061":2}`, `{"x":[{"a":1,"a":2}]}`, `{} {}`, `{"x":`, `{"a":{"b":1,"b":2}}`} {
		if validateUniqueJSON([]byte(input)) == nil {
			t.Errorf("accepted %s", input)
		}
	}
	for _, input := range []string{`{"a":1,"b":{"a":2}}`, `[{},null,1,"x"]`, "{\"x\":\"\xff\"}"} {
		if err := validateUniqueJSON([]byte(input)); err != nil {
			t.Errorf("rejected %q: %v", input, err)
		}
	}
}

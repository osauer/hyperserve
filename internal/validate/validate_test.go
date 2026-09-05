package validate

import (
	"errors"
	"strings"
	"testing"
)

func TestUnknownRulesAreRejected(t *testing.T) {
	for _, value := range []any{
		struct {
			Name string `validate:"requird"`
		}{},
		struct {
			Nested struct{} `validate:"requird"`
		}{},
		struct {
			Nested *struct{} `validate:"requird"`
		}{},
	} {
		err := Struct(value)
		if _, ok := errors.AsType[*ValidationError](err); !ok || !strings.Contains(err.Error(), `unknown validation rule "requird"`) {
			t.Errorf("Struct(%T) = %v, want unknown-rule error", value, err)
		}
	}
	value := struct {
		Name string `json:"name" validate:"required" gorm:"column:name" db:"name"`
	}{Name: "Ada"}
	if err := Struct(value); err != nil {
		t.Fatalf("unrelated struct tags affected validation: %v", err)
	}
}

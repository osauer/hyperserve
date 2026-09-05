package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestNumericOneofSchemaMatchesExecution(t *testing.T) {
	type args struct {
		Signed   int64   `json:"signed" validate:"oneof=-9223372036854775808 9223372036854775807"`
		Unsigned uint64  `json:"unsigned" validate:"oneof=0 18446744073709551615"`
		Fraction float32 `json:"fraction" validate:"oneof=0.1 0.2"`
	}
	tool := NewTypedTool("numeric_enum", "", func(_ context.Context, value args) (any, error) {
		return value, nil
	})
	encoded, err := json.Marshal(tool.Schema())
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var schema struct {
		Properties map[string]struct {
			Type string `json:"type"`
			Enum []any  `json:"enum"`
		} `json:"properties"`
	}
	if err := decoder.Decode(&schema); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string][]string{
		"signed":   {"-9223372036854775808", "9223372036854775807"},
		"unsigned": {"0", "18446744073709551615"},
		"fraction": {"0.1", "0.2"},
	} {
		property := schema.Properties[name]
		wantType := "integer"
		if name == "fraction" {
			wantType = "number"
		}
		if property.Type != wantType {
			t.Errorf("%s type = %q, want %q", name, property.Type, wantType)
		}
		if len(property.Enum) != len(want) {
			t.Fatalf("%s enum = %v, want %v", name, property.Enum, want)
		}
		for i, value := range property.Enum {
			number, ok := value.(json.Number)
			if !ok || number.String() != want[i] {
				t.Errorf("%s enum[%d] = %#v, want JSON number %s", name, i, value, want[i])
			}
		}
	}
	for _, value := range []args{
		{Signed: math.MinInt64, Unsigned: math.MaxUint64, Fraction: 0.1},
		{Signed: math.MaxInt64, Unsigned: 0, Fraction: 0.2},
	} {
		got, err := tool.Execute(map[string]any{
			"signed": value.Signed, "unsigned": value.Unsigned, "fraction": value.Fraction,
		})
		if err != nil || got != value {
			t.Errorf("Execute(%+v) = %v, %v", value, got, err)
		}
	}
}

func constructEnumTool[T any]() {
	NewTypedTool("invalid_enum", "", func(_ context.Context, _ T) (any, error) { return nil, nil })
}

func TestNumericOneofRejectsInvalidRegistration(t *testing.T) {
	for _, tt := range []struct {
		name      string
		construct func()
	}{
		{"signed overflow", constructEnumTool[struct {
			N int8 `validate:"oneof=128"`
		}]},
		{"unsigned overflow", constructEnumTool[struct {
			N uint64 `validate:"oneof=18446744073709551616"`
		}]},
		{"negative unsigned", constructEnumTool[struct {
			N uint8 `validate:"oneof=-1"`
		}]},
		{"invalid integer", constructEnumTool[struct {
			N int `validate:"oneof=word"`
		}]},
		{"fractional integer", constructEnumTool[struct {
			N int `validate:"oneof=1.5"`
		}]},
		{"invalid float", constructEnumTool[struct {
			N float64 `validate:"oneof=word"`
		}]},
		{"NaN", constructEnumTool[struct {
			N float64 `validate:"oneof=NaN"`
		}]},
		{"infinity", constructEnumTool[struct {
			N float64 `validate:"oneof=Inf"`
		}]},
		{"float32 overflow", constructEnumTool[struct {
			N float32 `validate:"oneof=1e40"`
		}]},
	} {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if got := recover(); got == nil || !strings.Contains(fmt.Sprint(got), "invalid oneof value") {
					t.Errorf("registration panic = %v, want invalid oneof error", got)
				}
			}()
			tt.construct()
		})
	}
}

func TestTypedToolRejectsUnknownValidationRule(t *testing.T) {
	type args struct {
		Name string `json:"name" validate:"requird"`
	}
	called := false
	tool := NewTypedTool("unknown_rule", "", func(_ context.Context, _ args) (any, error) {
		called = true
		return nil, nil
	})
	_, err := tool.Execute(map[string]any{"name": "Ada"})
	if err == nil || !strings.Contains(err.Error(), `unknown validation rule "requird"`) {
		t.Errorf("Execute error = %v, want unknown-rule error", err)
	}
	if called {
		t.Fatal("invalid validation rule allowed tool execution")
	}
}

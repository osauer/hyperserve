package mcp

import (
	"encoding/json"
	"testing"
)

func TestToolBuilderSnapshotsBuiltTools(t *testing.T) {
	builder := NewTool("lookup").WithDescription("original").
		WithParameter("id", "string", "identifier", true).
		WithExecute(func(map[string]any) (any, error) { return "original", nil })
	first := builder.Build()
	before, err := json.Marshal(first.Schema())
	if err != nil {
		t.Fatal(err)
	}
	builder.WithDescription("updated").
		WithParameter("id", "integer", "new identifier", false).
		WithParameter("extra", "boolean", "extra argument", true).
		WithExecute(func(map[string]any) (any, error) { return "updated", nil })
	second := builder.Build()
	after, err := json.Marshal(first.Schema())
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || first.Description() != "original" {
		t.Fatalf("builder mutation changed the first tool: %s, %s", first.Description(), after)
	}
	for _, tc := range []struct {
		tool Tool
		want string
	}{{first, "original"}, {second, "updated"}} {
		result, err := tc.tool.Execute(nil)
		if err != nil || result != tc.want || tc.tool.Description() != tc.want {
			t.Errorf("tool = (%s, %v, %v), want %s", tc.tool.Description(), result, err, tc.want)
		}
	}
	first.Schema()["properties"].(map[string]any)["id"].(map[string]any)["type"] = "boolean"
	if second.Schema()["properties"].(map[string]any)["id"].(map[string]any)["type"] != "integer" {
		t.Fatal("built tools share parameter metadata")
	}
}

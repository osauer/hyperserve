package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/osauer/hyperserve/internal/validate"
)

// Representative arg type covering the verbs the schema generator supports.
type managePostsArgs struct {
	Action  string   `json:"action" validate:"required,oneof=create update delete list get" mcp:"desc=Action to perform"`
	PostID  string   `json:"post_id,omitempty" mcp:"desc=Post identifier"`
	Title   string   `json:"title,omitempty" validate:"max=200"`
	Content string   `json:"content,omitempty"`
	Tags    []string `json:"tags,omitempty" validate:"max=10"`
	Rating  int      `json:"rating,omitempty" validate:"min=1,max=5"`
}

func TestNewTypedTool_Schema(t *testing.T) {
	tool := NewTypedTool("manage_posts", "Manage blog posts",
		func(_ context.Context, _ managePostsArgs) (any, error) { return nil, nil })

	if got := tool.Name(); got != "manage_posts" {
		t.Errorf("Name() = %q, want %q", got, "manage_posts")
	}
	if got := tool.Description(); got != "Manage blog posts" {
		t.Errorf("Description() = %q", got)
	}

	schema := tool.Schema()
	if schema["type"] != "object" {
		t.Fatalf("type = %v, want object", schema["type"])
	}

	props, _ := schema["properties"].(map[string]any)
	if props == nil {
		t.Fatalf("missing properties")
	}

	// action: required, enum from oneof, description from mcp tag.
	action, _ := props["action"].(map[string]any)
	if action == nil {
		t.Fatalf("missing properties.action")
	}
	if action["type"] != "string" {
		t.Errorf("action.type = %v", action["type"])
	}
	wantEnum := []any{"create", "update", "delete", "list", "get"}
	if !reflect.DeepEqual(action["enum"], wantEnum) {
		t.Errorf("action.enum = %#v, want %#v", action["enum"], wantEnum)
	}
	if action["description"] != "Action to perform" {
		t.Errorf("action.description = %v", action["description"])
	}

	// title: max → maxLength.
	title, _ := props["title"].(map[string]any)
	if title == nil {
		t.Fatalf("missing properties.title")
	}
	if title["maxLength"] != 200 {
		t.Errorf("title.maxLength = %v, want 200", title["maxLength"])
	}
	if _, hasMin := title["minLength"]; hasMin {
		t.Errorf("title should not carry minLength: %#v", title)
	}

	// tags: array with maxItems.
	tags, _ := props["tags"].(map[string]any)
	if tags == nil {
		t.Fatalf("missing properties.tags")
	}
	if tags["type"] != "array" {
		t.Errorf("tags.type = %v", tags["type"])
	}
	if tags["maxItems"] != 10 {
		t.Errorf("tags.maxItems = %v", tags["maxItems"])
	}
	items, _ := tags["items"].(map[string]any)
	if items == nil || items["type"] != "string" {
		t.Errorf("tags.items = %#v", tags["items"])
	}

	// rating: integer with min/max.
	rating, _ := props["rating"].(map[string]any)
	if rating == nil {
		t.Fatalf("missing properties.rating")
	}
	if rating["type"] != "integer" {
		t.Errorf("rating.type = %v", rating["type"])
	}
	if rating["minimum"] != int64(1) {
		t.Errorf("rating.minimum = %#v", rating["minimum"])
	}
	if rating["maximum"] != int64(5) {
		t.Errorf("rating.maximum = %#v", rating["maximum"])
	}

	// required list contains action only.
	req, _ := schema["required"].([]string)
	if !reflect.DeepEqual(req, []string{"action"}) {
		t.Errorf("required = %#v, want [action]", req)
	}
}

func TestNewTypedTool_SchemaRoundTripsThroughJSON(t *testing.T) {
	// MCP clients receive the schema as JSON; sanity-check that ours
	// marshals cleanly and the result holds the same constraints.
	tool := NewTypedTool("manage_posts", "",
		func(_ context.Context, _ managePostsArgs) (any, error) { return nil, nil })
	out, err := json.Marshal(tool.Schema())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	props, _ := decoded["properties"].(map[string]any)
	action, _ := props["action"].(map[string]any)
	if action["description"] != "Action to perform" {
		t.Errorf("description lost in roundtrip: %v", action)
	}
	req, _ := decoded["required"].([]any)
	if len(req) != 1 || req[0] != "action" {
		t.Errorf("required after roundtrip = %v", req)
	}
}

type optionalArgs struct {
	Name string  `json:"name" validate:"required,min=2,max=64"`
	Note *string `json:"note,omitempty"`
}

func TestNewTypedTool_OptionalPointer(t *testing.T) {
	tool := NewTypedTool("optional", "",
		func(_ context.Context, _ optionalArgs) (any, error) { return nil, nil })
	schema := tool.Schema()
	props, _ := schema["properties"].(map[string]any)

	if _, ok := props["note"]; !ok {
		t.Fatalf("pointer field missing from properties: %#v", props)
	}
	note, _ := props["note"].(map[string]any)
	if note["type"] != "string" {
		t.Errorf("note.type = %v", note["type"])
	}

	req, _ := schema["required"].([]string)
	if !reflect.DeepEqual(req, []string{"name"}) {
		t.Errorf("required = %#v, want [name]", req)
	}

	// min/max on a string maps to minLength/maxLength.
	name, _ := props["name"].(map[string]any)
	if name["minLength"] != 2 || name["maxLength"] != 64 {
		t.Errorf("name string bounds = %#v", name)
	}
}

type calcArgs struct {
	A int `json:"a" validate:"required"`
	B int `json:"b" validate:"required"`
}

func TestNewTypedTool_Execute_Success(t *testing.T) {
	tool := NewTypedTool("add", "",
		func(_ context.Context, args calcArgs) (any, error) {
			return args.A + args.B, nil
		})
	got, err := tool.Execute(map[string]any{"a": 2, "b": 3})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if got != 5 {
		t.Errorf("Execute = %v, want 5", got)
	}
}

func TestNewTypedTool_Execute_ContextPropagates(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "hello")
	tool := NewTypedTool("ctx", "",
		func(ctx context.Context, _ calcArgs) (any, error) {
			return ctx.Value(key{}), nil
		})
	ctxTool, ok := tool.(ToolWithContext)
	if !ok {
		t.Fatalf("typed tool does not implement ToolWithContext")
	}
	got, err := ctxTool.ExecuteWithContext(ctx, map[string]any{"a": 1, "b": 2})
	if err != nil {
		t.Fatalf("ExecuteWithContext error: %v", err)
	}
	if got != "hello" {
		t.Errorf("ctx value lost: got %v", got)
	}
}

func TestNewTypedTool_Execute_ValidationError(t *testing.T) {
	tool := NewTypedTool("create_post", "",
		func(_ context.Context, _ managePostsArgs) (any, error) {
			t.Fatal("handler must not run when validation fails")
			return nil, nil
		})
	_, err := tool.Execute(map[string]any{"action": "publish"})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	var verr *validate.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ValidationError, got %T (%v)", err, err)
	}
	if !verr.HasField("action") {
		t.Errorf("expected action to fail: %v", verr)
	}
}

func TestNewTypedTool_Execute_RequiredFieldMissing(t *testing.T) {
	tool := NewTypedTool("create_post", "",
		func(_ context.Context, _ managePostsArgs) (any, error) {
			t.Fatal("handler must not run when validation fails")
			return nil, nil
		})
	_, err := tool.Execute(nil)
	var verr *validate.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ValidationError, got %T (%v)", err, err)
	}
	if !verr.HasField("action") {
		t.Errorf("expected required action to fail: %v", verr)
	}
}

func TestNewTypedTool_Execute_UnknownFieldRejected(t *testing.T) {
	tool := NewTypedTool("create_post", "",
		func(_ context.Context, _ calcArgs) (any, error) { return nil, nil })
	_, err := tool.Execute(map[string]any{"a": 1, "b": 2, "c": 3})
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}

func TestNewTypedTool_Execute_HandlerError(t *testing.T) {
	want := errors.New("boom")
	tool := NewTypedTool("fail", "",
		func(_ context.Context, _ calcArgs) (any, error) { return nil, want })
	_, err := tool.Execute(map[string]any{"a": 1, "b": 2})
	if !errors.Is(err, want) {
		t.Fatalf("handler error not returned verbatim: %v", err)
	}
}

func TestNewTypedTool_PanicsOnNonStruct(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for non-struct T")
		}
	}()
	_ = NewTypedTool[int]("bad", "", func(_ context.Context, _ int) (any, error) { return nil, nil })
}

func TestNewTypedTool_PointerArgsAreUnwrapped(t *testing.T) {
	// T as *S — the schema should still describe S's fields.
	type S struct {
		Name string `json:"name" validate:"required"`
	}
	tool := NewTypedTool[*S]("ptr", "",
		func(_ context.Context, _ *S) (any, error) { return nil, nil })
	props, _ := tool.Schema()["properties"].(map[string]any)
	if _, ok := props["name"]; !ok {
		t.Fatalf("missing name in schema: %#v", tool.Schema())
	}
}

func TestNewTypedTool_NestedStructInlined(t *testing.T) {
	type Inner struct {
		Code string `json:"code" validate:"required,oneof=A B C"`
	}
	type Outer struct {
		Inner Inner  `json:"inner"`
		ID    string `json:"id" validate:"required"`
	}
	tool := NewTypedTool("nested", "",
		func(_ context.Context, _ Outer) (any, error) { return nil, nil })
	schema := tool.Schema()
	props, _ := schema["properties"].(map[string]any)
	inner, _ := props["inner"].(map[string]any)
	if inner == nil || inner["type"] != "object" {
		t.Fatalf("inner missing or wrong type: %#v", props["inner"])
	}
	innerProps, _ := inner["properties"].(map[string]any)
	code, _ := innerProps["code"].(map[string]any)
	if code["type"] != "string" {
		t.Errorf("inner.code.type = %v", code["type"])
	}
	enum, _ := code["enum"].([]any)
	if !reflect.DeepEqual(enum, []any{"A", "B", "C"}) {
		t.Errorf("inner.code.enum = %#v", enum)
	}
}

type post struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Tags  []string `json:"tags,omitempty"`
}

func TestNewTypedTool_OutputSchema_Struct(t *testing.T) {
	tool := NewTypedTool("get_post", "",
		func(_ context.Context, args calcArgs) (post, error) {
			return post{ID: "x"}, nil
		})
	ot, ok := tool.(ToolWithOutputSchema)
	if !ok {
		t.Fatalf("typed tool does not implement ToolWithOutputSchema")
	}
	schema := ot.OutputSchema()
	if schema == nil {
		t.Fatalf("expected non-nil output schema")
	}
	if schema["type"] != "object" {
		t.Errorf("schema.type = %v", schema["type"])
	}
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["id"]; !ok {
		t.Errorf("output schema missing id: %#v", props)
	}
	tags, _ := props["tags"].(map[string]any)
	if tags == nil || tags["type"] != "array" {
		t.Errorf("tags property = %#v", tags)
	}
}

func TestNewTypedTool_OutputSchema_SliceOfStruct(t *testing.T) {
	tool := NewTypedTool("list_posts", "",
		func(_ context.Context, _ struct{}) ([]post, error) { return nil, nil })
	ot, _ := tool.(ToolWithOutputSchema)
	schema := ot.OutputSchema()
	if schema == nil || schema["type"] != "array" {
		t.Fatalf("expected array schema, got %#v", schema)
	}
	items, _ := schema["items"].(map[string]any)
	if items == nil || items["type"] != "object" {
		t.Errorf("items = %#v", items)
	}
}

func TestNewTypedTool_OutputSchema_EmptyStructOmitted(t *testing.T) {
	// `struct{}` returns mean "no payload" — don't advertise an output schema.
	tool := NewTypedTool("delete_post", "",
		func(_ context.Context, _ calcArgs) (struct{}, error) { return struct{}{}, nil })
	ot, _ := tool.(ToolWithOutputSchema)
	if got := ot.OutputSchema(); got != nil {
		t.Errorf("expected nil output schema for struct{}, got %#v", got)
	}
}

func TestNewTypedTool_OutputSchema_AnyOmitted(t *testing.T) {
	// `any` returns carry no compile-time shape; omit outputSchema.
	tool := NewTypedTool("legacy", "",
		func(_ context.Context, _ calcArgs) (any, error) { return nil, nil })
	ot, _ := tool.(ToolWithOutputSchema)
	if got := ot.OutputSchema(); got != nil {
		t.Errorf("expected nil output schema for any, got %#v", got)
	}
}

func TestNewTypedTool_OutputSchema_PrimitiveReturn(t *testing.T) {
	tool := NewTypedTool("count", "",
		func(_ context.Context, _ struct{}) (int, error) { return 0, nil })
	ot, _ := tool.(ToolWithOutputSchema)
	schema := ot.OutputSchema()
	if schema == nil || schema["type"] != "integer" {
		t.Errorf("primitive output schema = %#v", schema)
	}
}

func TestHandleToolsList_EmitsOutputSchema(t *testing.T) {
	h := NewHandler(ServerInfo{Name: "test", Version: "0"})
	h.RegisterTool(NewTypedTool("get_post", "",
		func(_ context.Context, args calcArgs) (post, error) { return post{}, nil }))
	// Also register a builder-based tool — it has no output schema and the
	// JSON field must be omitted, not emitted as null.
	h.RegisterTool(NewTool("legacy").
		WithDescription("").
		WithExecute(func(_ map[string]any) (any, error) { return nil, nil }).
		Build())

	raw := h.ProcessRequest([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	var resp struct {
		Result struct {
			Tools []ToolInfo `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, raw)
	}

	var typed, legacy *ToolInfo
	for i := range resp.Result.Tools {
		switch resp.Result.Tools[i].Name {
		case "get_post":
			typed = &resp.Result.Tools[i]
		case "legacy":
			legacy = &resp.Result.Tools[i]
		}
	}
	if typed == nil || legacy == nil {
		t.Fatalf("missing tools: %#v", resp.Result.Tools)
	}
	if typed.OutputSchema == nil {
		t.Errorf("typed tool: outputSchema not emitted")
	}
	if legacy.OutputSchema != nil {
		t.Errorf("builder tool: outputSchema should be omitted, got %#v", legacy.OutputSchema)
	}

	// Re-marshal and assert the JSON key is absent for the builder tool.
	out, _ := json.Marshal(legacy)
	if bytes.Contains(out, []byte("outputSchema")) {
		t.Errorf("builder tool JSON contains outputSchema: %s", out)
	}
}

func TestNewTypedTool_ValidationErrorMessageFormat(t *testing.T) {
	// Pin the wire shape of validation errors. MCP clients (and our docs)
	// rely on this format; loosen it only with a deliberate version bump.
	type args struct {
		Action string   `json:"action" validate:"required,oneof=create list get delete"`
		Tags   []string `json:"tags,omitempty" validate:"max=2"`
	}
	tool := NewTypedTool("pinned", "",
		func(_ context.Context, _ args) (any, error) { return nil, nil })
	_, err := tool.Execute(map[string]any{"action": "nuke", "tags": []string{"a", "b", "c"}})
	if err == nil {
		t.Fatal("expected validation error")
	}
	got := err.Error()
	// One failing field per Field; ValidationError joins multiple with "; ".
	for _, want := range []string{
		"action: must be one of: create list get delete",
		"tags: length must be <= 2 (got 3)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("error %q missing %q", got, want)
		}
	}
	if !strings.HasPrefix(got, "validation failed: ") {
		t.Errorf("error %q should start with 'validation failed: '", got)
	}
}

func TestNewTypedTool_RegistersThroughHandler(t *testing.T) {
	// End-to-end: register a typed tool, dispatch tools/call through the
	// JSON-RPC engine, assert the result envelope.
	h := NewHandler(ServerInfo{Name: "test", Version: "0.0.0"})
	h.RegisterTool(NewTypedTool("add", "",
		func(_ context.Context, args calcArgs) (any, error) {
			return map[string]any{"sum": args.A + args.B}, nil
		}))

	req := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"add","arguments":{"a":40,"b":2}}}`)
	resp := h.ProcessRequest(req)

	var got map[string]any
	if err := json.Unmarshal(resp, &got); err != nil {
		t.Fatalf("unmarshal response: %v\nraw: %s", err, resp)
	}
	if errField, ok := got["error"]; ok {
		t.Fatalf("rpc error: %#v", errField)
	}
	result, _ := got["result"].(map[string]any)
	if result == nil {
		t.Fatalf("missing result: %s", resp)
	}
	content, _ := result["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("content = %#v", result["content"])
	}
	first, _ := content[0].(map[string]any)
	if first["type"] != "text" {
		t.Errorf("content[0].type = %v", first["type"])
	}
	if first["text"] != `{"sum":42}` {
		t.Errorf("content[0].text = %v", first["text"])
	}
}

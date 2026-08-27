package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/osauer/hyperserve/v2/internal/validate"
)

// TypedToolFunc is the signature an MCP tool implements when registered via
// NewTypedTool. The In value is decoded and validated before the function
// runs; Out is JSON-marshaled by the framework. Use `struct{}` for either
// side when the tool takes no arguments or returns no payload.
type TypedToolFunc[In, Out any] func(ctx context.Context, args In) (Out, error)

// ToolWithOutputSchema is implemented by tools that can describe their
// return shape. The handler's tools/list path checks this interface via
// type assertion and surfaces the result as the `outputSchema` field on
// ToolInfo (MCP spec revision 2025-06-18). Returning nil omits the field.
type ToolWithOutputSchema interface {
	OutputSchema() map[string]any
}

// NewTypedTool returns a Tool whose input schema is derived from In (which
// must be a struct or pointer to a struct). When Out is also a struct (or
// slice/array of struct), an output schema is derived too and emitted as
// `outputSchema` in tools/list. The returned Tool implements
// ToolWithContext, so the per-call timeout enforced by
// Handler.handleToolsCall is propagated to fn.
//
//	type CreatePostArgs struct {
//	    Title  string   `json:"title"  validate:"required,max=200"`
//	    Author string   `json:"author" validate:"required"`
//	    Tags   []string `json:"tags,omitempty" validate:"max=10"`
//	}
//	type Post struct {
//	    ID, Title, Author string
//	    Tags              []string
//	    CreatedAt         time.Time
//	}
//
//	tool := mcp.NewTypedTool("create_post", "Create a new blog post.",
//	    func(ctx context.Context, args CreatePostArgs) (Post, error) {
//	        return blog.create(args)
//	    })
//	srv.RegisterMCPTool(tool)
//
// Type inference at the call site picks both In and Out off the function
// value, so callers don't write the type parameters explicitly.
//
// Schema generation covers strings, numbers, booleans, arrays, and nested
// structs. It derives enums and bounds from oneof, min, max, and len validation
// rules, and descriptions from mcp:"desc=..." tags. Nested structs are inlined;
// $ref, $defs, cross-field rules, and custom validators are not emitted.
//
// Prefer NewTypedTool for new tools because the typed shape keeps the handler
// and its schema together. The NewTool builder remains available for callers
// that need to assemble a schema dynamically.
//
// Panics at registration if In is not a struct type — the panic is
// preferable to silently emitting a schema no client can use.
func NewTypedTool[In, Out any](name, description string, fn TypedToolFunc[In, Out]) Tool {
	if fn == nil {
		panic("mcp.NewTypedTool: fn is nil")
	}
	inT := reflect.TypeFor[In]()
	if inT.Kind() == reflect.Pointer {
		inT = inT.Elem()
	}
	if inT.Kind() != reflect.Struct {
		panic(fmt.Sprintf("mcp.NewTypedTool: In type parameter must be a struct, got %s", inT.Kind()))
	}
	return &typedTool[In, Out]{
		name:         name,
		description:  description,
		schema:       buildObjectSchema(inT),
		outputSchema: deriveOutputSchema(reflect.TypeFor[Out]()),
		fn:           fn,
	}
}

type typedTool[In, Out any] struct {
	name         string
	description  string
	schema       map[string]any
	outputSchema map[string]any
	fn           TypedToolFunc[In, Out]
}

func (t *typedTool[In, Out]) Name() string           { return t.name }
func (t *typedTool[In, Out]) Description() string    { return t.description }
func (t *typedTool[In, Out]) Schema() map[string]any { return t.schema }

// OutputSchema implements ToolWithOutputSchema. Returns nil when the
// generator couldn't describe Out (e.g. Out is `any`, an empty struct, or
// a type the v1 generator declines to emit).
func (t *typedTool[In, Out]) OutputSchema() map[string]any { return t.outputSchema }

// Execute implements Tool. Routed through ExecuteWithContext with a
// background context — the MCP dispatcher always calls the
// ToolWithContext path, so this is here for the rare caller that wants a
// raw Tool reference.
func (t *typedTool[In, Out]) Execute(params map[string]any) (any, error) {
	return t.ExecuteWithContext(context.Background(), params)
}

// ExecuteWithContext implements ToolWithContext.
func (t *typedTool[In, Out]) ExecuteWithContext(ctx context.Context, params map[string]any) (any, error) {
	var args In
	if len(params) > 0 {
		raw, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("encode arguments: %w", err)
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&args); err != nil {
			return nil, fmt.Errorf("decode arguments: %w", err)
		}
	}
	validateTarget := any(&args)
	if reflect.TypeFor[In]().Kind() == reflect.Pointer {
		validateTarget = args
	}
	if err := validate.Struct(validateTarget); err != nil {
		return nil, err
	}
	return t.fn(ctx, args)
}

// deriveOutputSchema returns the schema fragment describing Out, or nil
// when there's nothing useful to advertise. We skip:
//   - the empty interface (`any`) — no compile-time shape to advertise
//   - empty structs — tools that return no payload
//   - types the field-schema generator declines to describe
func deriveOutputSchema(t reflect.Type) map[string]any {
	if t == nil {
		return nil
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() == reflect.Interface && t.NumMethod() == 0 {
		return nil
	}
	if t.Kind() == reflect.Struct && t.NumField() == 0 {
		return nil
	}
	return fieldToSchema(t, "")
}

// buildObjectSchema produces a JSON Schema fragment for a struct type. The
// result is the map[string]any shape MCP clients consume directly — no
// wrapping, no $defs.
func buildObjectSchema(t reflect.Type) map[string]any {
	props := map[string]any{}
	var required []string
	for sf := range t.Fields() {
		if !sf.IsExported() {
			continue
		}
		name, skip := jsonFieldName(sf)
		if skip {
			continue
		}
		fieldSchema := fieldToSchema(sf.Type, sf.Tag)
		if fieldSchema == nil {
			continue
		}
		if desc := mcpDescription(sf.Tag); desc != "" {
			fieldSchema["description"] = desc
		}
		props[name] = fieldSchema
		if hasValidateVerb(sf.Tag.Get("validate"), "required") {
			required = append(required, name)
		}
	}
	schema := map[string]any{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// fieldToSchema returns the schema fragment for a single struct field,
// honouring the validate constraints carried on the tag. Returns nil for
// types the v1 generator declines to describe (functions, channels, …);
// the field is then omitted from the schema rather than emitted as a
// half-typed property.
func fieldToSchema(t reflect.Type, tag reflect.StructTag) map[string]any {
	// Pointers map to the pointee schema; optionality is expressed by
	// absence from `required`, not by a separate JSON Schema construct.
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	validateTag := tag.Get("validate")
	switch t.Kind() {
	case reflect.String:
		s := map[string]any{"type": "string"}
		if vs := oneofValues(validateTag); len(vs) > 0 {
			s["enum"] = toAnySlice(vs)
		}
		applyStringBounds(s, validateTag)
		return s
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		s := map[string]any{"type": "integer"}
		if vs := oneofValues(validateTag); len(vs) > 0 {
			s["enum"] = enumFromStrings(vs, true)
		}
		applyNumericBounds(s, validateTag)
		return s
	case reflect.Float32, reflect.Float64:
		s := map[string]any{"type": "number"}
		if vs := oneofValues(validateTag); len(vs) > 0 {
			s["enum"] = enumFromStrings(vs, false)
		}
		applyNumericBounds(s, validateTag)
		return s
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			// []byte serialises as a base64 string in encoding/json.
			s := map[string]any{"type": "string"}
			applyStringBounds(s, validateTag)
			return s
		}
		s := map[string]any{
			"type":  "array",
			"items": fieldToSchema(t.Elem(), ""),
		}
		applyArrayBounds(s, validateTag)
		return s
	case reflect.Map:
		// JSON objects with arbitrary string keys. Only string-keyed maps
		// round-trip through encoding/json without surprises.
		if t.Key().Kind() != reflect.String {
			return nil
		}
		return map[string]any{
			"type":                 "object",
			"additionalProperties": fieldToSchema(t.Elem(), ""),
		}
	case reflect.Struct:
		return buildObjectSchema(t)
	case reflect.Interface:
		// `any` — emit as untyped. Clients that care can constrain via a
		// concrete type instead.
		return map[string]any{}
	default:
		return nil
	}
}

// jsonFieldName mirrors encoding/json's field-naming rules for the subset
// we care about: explicit json tag wins, "-" skips, ",omitempty" suffix is
// stripped. Returns (name, skip).
func jsonFieldName(sf reflect.StructField) (string, bool) {
	tag := sf.Tag.Get("json")
	if tag == "-" {
		return "", true
	}
	if tag == "" {
		return sf.Name, false
	}
	if comma := strings.Index(tag, ","); comma >= 0 {
		tag = tag[:comma]
	}
	if tag == "" {
		return sf.Name, false
	}
	return tag, false
}

// hasValidateVerb reports whether the given verb appears in a validate tag.
// Verb matching is exact; "required" matches "required" but not
// "required_without".
func hasValidateVerb(tag, verb string) bool {
	if tag == "" {
		return false
	}
	for rule := range strings.SplitSeq(tag, ",") {
		v, _, _ := strings.Cut(strings.TrimSpace(rule), "=")
		if v == verb {
			return true
		}
	}
	return false
}

// validateParam returns the param of the first occurrence of verb in tag,
// e.g. validateParam("min=3,max=10", "max") == "10".
func validateParam(tag, verb string) (string, bool) {
	if tag == "" {
		return "", false
	}
	for rule := range strings.SplitSeq(tag, ",") {
		v, param, hasEq := strings.Cut(strings.TrimSpace(rule), "=")
		if v == verb && hasEq {
			return param, true
		}
	}
	return "", false
}

// oneofValues returns the space-separated options from `oneof=A B C`.
func oneofValues(tag string) []string {
	if param, ok := validateParam(tag, "oneof"); ok && param != "" {
		return strings.Fields(param)
	}
	return nil
}

// enumFromStrings parses oneof options into typed JSON values. For
// integer fields it parses each option as int64; for floats it parses as
// float64. Any parse failure falls back to the raw string list so the
// schema is still emitted (clients will get a type-confused enum, but
// that's better than silently dropping the constraint).
func enumFromStrings(values []string, asInt bool) []any {
	out := make([]any, 0, len(values))
	for _, v := range values {
		if asInt {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return toAnySlice(values)
			}
			out = append(out, n)
			continue
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return toAnySlice(values)
		}
		out = append(out, f)
	}
	return out
}

func toAnySlice(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

func applyStringBounds(s map[string]any, tag string) {
	if v, ok := validateParam(tag, "min"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			s["minLength"] = n
		}
	}
	if v, ok := validateParam(tag, "max"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			s["maxLength"] = n
		}
	}
	if v, ok := validateParam(tag, "len"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			s["minLength"] = n
			s["maxLength"] = n
		}
	}
}

func applyNumericBounds(s map[string]any, tag string) {
	if v, ok := validateParam(tag, "min"); ok {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			s["minimum"] = numericLiteral(n)
		}
	}
	if v, ok := validateParam(tag, "max"); ok {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			s["maximum"] = numericLiteral(n)
		}
	}
}

func applyArrayBounds(s map[string]any, tag string) {
	if v, ok := validateParam(tag, "min"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			s["minItems"] = n
		}
	}
	if v, ok := validateParam(tag, "max"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			s["maxItems"] = n
		}
	}
	if v, ok := validateParam(tag, "len"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			s["minItems"] = n
			s["maxItems"] = n
		}
	}
}

// numericLiteral keeps whole numbers as int so they encode as `5` instead
// of `5` (well, JSON renders them the same, but it makes the in-memory
// shape stable for the schema-gen tests).
func numericLiteral(f float64) any {
	if f == float64(int64(f)) {
		return int64(f)
	}
	return f
}

// mcpDescription returns the description carried by `mcp:"desc=…"`. The
// rest of the tag is reserved for future keys; for v1 we only read desc=.
func mcpDescription(tag reflect.StructTag) string {
	raw := tag.Get("mcp")
	if raw == "" {
		return ""
	}
	if after, ok := strings.CutPrefix(raw, "desc="); ok {
		return after
	}
	return ""
}

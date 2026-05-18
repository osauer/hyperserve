// Package validate implements the tag-driven struct validator used by
// pkg/server (HTTP request binding) and pkg/mcp (typed-tool argument
// binding). It lives in internal/ so both packages can import it without
// creating a cycle between pkg/server and pkg/mcp.
//
// The exported surface — Validate, ValidationError, FieldError — is
// re-exposed by pkg/server via type aliases for callers that consume the
// validator through the public server API.
//
// Supported `validate` tag verbs:
//
//	required           field must be a non-zero value
//	min=N              numeric/length >= N
//	max=N              numeric/length <= N
//	len=N              length exactly N (string/slice/map)
//	email              loose RFC 5322 sanity check on the local-part/domain
//	oneof=A B C        value must equal one of the (space-separated) options
//	url                must parse via net/url and have a scheme + host
package validate

import (
	"fmt"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// FieldError describes one failed validation rule.
type FieldError struct {
	Field   string // JSON-tag name if present, else struct field name.
	Tag     string // The verb that failed (required, min, oneof, …).
	Param   string // The argument to the verb (the N in min=N, etc.).
	Value   any    // The field's actual value.
	Message string // Human-readable rendering.
}

// Error implements error.
func (f *FieldError) Error() string { return f.Message }

// ValidationError is the aggregate error returned by Validate when one or
// more fields fail their rules.
type ValidationError struct {
	Fields []*FieldError
}

// Error implements error. Returns a single line summarising all failures so
// it's safe to log without further formatting.
func (e *ValidationError) Error() string {
	if len(e.Fields) == 0 {
		return "validation failed"
	}
	if len(e.Fields) == 1 {
		return e.Fields[0].Message
	}
	parts := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		parts = append(parts, f.Message)
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

// HasField reports whether the validation error has an entry for `field`
// (matched against FieldError.Field).
func (e *ValidationError) HasField(field string) bool {
	for _, f := range e.Fields {
		if f.Field == field {
			return true
		}
	}
	return false
}

// Struct runs `validate:"..."` rules over dst (must be a pointer to a
// struct or a struct). Returns a *ValidationError when any rule fails, or
// nil otherwise. Nested structs are recursed into; pointers are
// dereferenced.
func Struct(dst any) error {
	v := reflect.ValueOf(dst)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return fmt.Errorf("validate: nil pointer")
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("validate: want struct, got %s", v.Kind())
	}
	verr := &ValidationError{}
	validateStruct(v, "", verr)
	if len(verr.Fields) == 0 {
		return nil
	}
	return verr
}

func validateStruct(v reflect.Value, parent string, errs *ValidationError) {
	t := v.Type()
	for i := range t.NumField() {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		fv := v.Field(i)
		// Recurse into nested structs (or struct pointers) so deeply
		// validated payloads don't need a per-handler walk.
		if fv.Kind() == reflect.Struct {
			validateStruct(fv, FieldName(sf, parent), errs)
			continue
		}
		if fv.Kind() == reflect.Pointer && fv.Elem().Kind() == reflect.Struct {
			if !fv.IsNil() {
				validateStruct(fv.Elem(), FieldName(sf, parent), errs)
			}
		}
		tag := sf.Tag.Get("validate")
		if tag == "" {
			continue
		}
		name := FieldName(sf, parent)
		for rule := range strings.SplitSeq(tag, ",") {
			rule = strings.TrimSpace(rule)
			if rule == "" {
				continue
			}
			verb, param, _ := strings.Cut(rule, "=")
			if msg, ok := runRule(verb, param, fv); !ok {
				errs.Fields = append(errs.Fields, &FieldError{
					Field:   name,
					Tag:     verb,
					Param:   param,
					Value:   fv.Interface(),
					Message: name + ": " + msg,
				})
				break // first failure per field wins
			}
		}
	}
}

// FieldName resolves the wire-level name of a struct field: the json tag if
// present and non-empty, else the Go field name. The optional parent prefix
// produces dotted paths for nested structs.
func FieldName(sf reflect.StructField, parent string) string {
	name := sf.Name
	if tag := sf.Tag.Get("json"); tag != "" {
		if comma := strings.Index(tag, ","); comma >= 0 {
			tag = tag[:comma]
		}
		if tag != "" && tag != "-" {
			name = tag
		}
	}
	if parent != "" {
		return parent + "." + name
	}
	return name
}

func runRule(verb, param string, v reflect.Value) (string, bool) {
	switch verb {
	case "required":
		if isZero(v) {
			return "is required", false
		}
		return "", true
	case "min":
		return numericBound(verb, param, v, false)
	case "max":
		return numericBound(verb, param, v, true)
	case "len":
		want, err := strconv.Atoi(param)
		if err != nil {
			return "invalid len param", false
		}
		got, ok := lengthOf(v)
		if !ok {
			return "len not applicable", false
		}
		if got != want {
			return fmt.Sprintf("length must be %d (got %d)", want, got), false
		}
		return "", true
	case "email":
		if v.Kind() != reflect.String {
			return "email requires string", false
		}
		if v.String() == "" {
			return "", true
		}
		if !looksLikeEmail(v.String()) {
			return "must be a valid email", false
		}
		return "", true
	case "url":
		if v.Kind() != reflect.String {
			return "url requires string", false
		}
		if v.String() == "" {
			return "", true
		}
		u, err := url.Parse(v.String())
		if err != nil || u.Scheme == "" || u.Host == "" {
			return "must be a valid URL", false
		}
		return "", true
	case "oneof":
		if param == "" {
			return "oneof requires options", false
		}
		s := fmt.Sprint(v.Interface())
		if slices.Contains(strings.Fields(param), s) {
			return "", true
		}
		return "must be one of: " + param, false
	default:
		// Unknown verbs are silently passed; this keeps custom tag
		// readers (gorm, db, …) from being mistaken for validate verbs
		// when the user reuses the field for two purposes.
		return "", true
	}
}

func numericBound(verb, param string, v reflect.Value, upper bool) (string, bool) {
	if param == "" {
		return verb + " requires a value", false
	}
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		want, err := strconv.ParseInt(param, 10, 64)
		if err != nil {
			return verb + " param not an integer", false
		}
		got := v.Int()
		if upper && got > want {
			return fmt.Sprintf("must be <= %d (got %d)", want, got), false
		}
		if !upper && got < want {
			return fmt.Sprintf("must be >= %d (got %d)", want, got), false
		}
		return "", true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		want, err := strconv.ParseUint(param, 10, 64)
		if err != nil {
			return verb + " param not an unsigned integer", false
		}
		got := v.Uint()
		if upper && got > want {
			return fmt.Sprintf("must be <= %d (got %d)", want, got), false
		}
		if !upper && got < want {
			return fmt.Sprintf("must be >= %d (got %d)", want, got), false
		}
		return "", true
	case reflect.Float32, reflect.Float64:
		want, err := strconv.ParseFloat(param, 64)
		if err != nil {
			return verb + " param not a number", false
		}
		got := v.Float()
		if upper && got > want {
			return fmt.Sprintf("must be <= %v (got %v)", want, got), false
		}
		if !upper && got < want {
			return fmt.Sprintf("must be >= %v (got %v)", want, got), false
		}
		return "", true
	case reflect.String, reflect.Slice, reflect.Map, reflect.Array:
		want, err := strconv.Atoi(param)
		if err != nil {
			return verb + " param not an integer", false
		}
		got, _ := lengthOf(v)
		if upper && got > want {
			return fmt.Sprintf("length must be <= %d (got %d)", want, got), false
		}
		if !upper && got < want {
			return fmt.Sprintf("length must be >= %d (got %d)", want, got), false
		}
		return "", true
	default:
		return verb + " not applicable to " + v.Kind().String(), false
	}
}

func lengthOf(v reflect.Value) (int, bool) {
	switch v.Kind() {
	case reflect.String, reflect.Slice, reflect.Map, reflect.Array:
		return v.Len(), true
	}
	return 0, false
}

func isZero(v reflect.Value) bool {
	return !v.IsValid() || v.IsZero()
}

// looksLikeEmail is intentionally permissive: the goal is to reject "abc"
// and "@" without trying to be RFC 5322 compliant. Domain validation is
// caller's job (DNS check, allowlist, …).
func looksLikeEmail(s string) bool {
	at := strings.LastIndex(s, "@")
	if at <= 0 || at == len(s)-1 {
		return false
	}
	local, domain := s[:at], s[at+1:]
	if strings.ContainsAny(local, " \t\r\n") {
		return false
	}
	if !strings.Contains(domain, ".") {
		return false
	}
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}
	return true
}

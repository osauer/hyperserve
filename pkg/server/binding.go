// Request binding + struct-tag validation.
//
// This is the one feature Gin gives you that net/http doesn't: parse JSON
// (or form / query) into a Go struct, then validate the fields against
// `validate:"..."` tags, with structured errors when something is wrong.
//
// Design constraints:
//   - Zero new runtime dependencies. The library still ships with one
//     transitive dep (golang.org/x/time). Validators are tag-driven over
//     reflection; no validator/v10, no codegen.
//   - Stdlib-shaped API: takes the *http.Request you already have, writes
//     into a destination struct, returns an error. No middleware-driven
//     side channel; the handler stays in control.
//   - Honest error type: ValidationError carries one entry per failing
//     field so handlers can render JSON 400s without string-parsing.
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
//
// Tags compose left-to-right; the first failure wins for a given field.
// Apply multiple tags with commas: `validate:"required,min=3,max=64"`.
package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// ValidationError is the aggregate error returned by Validate / Bind* when
// one or more fields fail their rules.
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

// BindJSON decodes the request body as JSON into dst, then runs Validate.
// dst must be a non-nil pointer to a struct. Returns ValidationError when
// rules fail, and the wrapped error otherwise (decode error, etc.).
func BindJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return fmt.Errorf("BindJSON: request body is nil")
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20)) // 1 MiB cap
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("BindJSON: %w", err)
	}
	return Validate(dst)
}

// BindQuery decodes URL query parameters into dst (string keys → struct
// fields by json tag or lowercased name). Slices are populated from
// repeated keys. Then runs Validate.
func BindQuery(r *http.Request, dst any) error {
	if err := decodeValues(r.URL.Query(), dst); err != nil {
		return fmt.Errorf("BindQuery: %w", err)
	}
	return Validate(dst)
}

// BindForm decodes application/x-www-form-urlencoded or multipart/form-data
// into dst, then runs Validate.
func BindForm(r *http.Request, dst any) error {
	if err := r.ParseForm(); err != nil {
		return fmt.Errorf("BindForm: parse form: %w", err)
	}
	if err := decodeValues(r.Form, dst); err != nil {
		return fmt.Errorf("BindForm: %w", err)
	}
	return Validate(dst)
}

// Bind picks the decoder based on Content-Type:
//   - application/json → BindJSON
//   - application/x-www-form-urlencoded or multipart/form-data → BindForm
//   - otherwise → BindQuery
func Bind(r *http.Request, dst any) error {
	ct := r.Header.Get("Content-Type")
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = ct[:i]
	}
	ct = strings.TrimSpace(ct)
	switch ct {
	case "application/json":
		return BindJSON(r, dst)
	case "application/x-www-form-urlencoded", "multipart/form-data":
		return BindForm(r, dst)
	default:
		return BindQuery(r, dst)
	}
}

// Validate runs `validate:"..."` rules over dst (must be a pointer to a
// struct or a struct). Returns a *ValidationError when any rule fails, or
// nil otherwise. Nested structs are recursed into; pointers are
// dereferenced.
func Validate(dst any) error {
	v := reflect.ValueOf(dst)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return fmt.Errorf("Validate: nil pointer")
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("Validate: want struct, got %s", v.Kind())
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
			validateStruct(fv, fieldName(sf, parent), errs)
			continue
		}
		if fv.Kind() == reflect.Pointer && fv.Elem().Kind() == reflect.Struct {
			if !fv.IsNil() {
				validateStruct(fv.Elem(), fieldName(sf, parent), errs)
			}
		}
		tag := sf.Tag.Get("validate")
		if tag == "" {
			continue
		}
		name := fieldName(sf, parent)
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

func fieldName(sf reflect.StructField, parent string) string {
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

// decodeValues populates a struct from url.Values, using the same json-tag
// lookup as the validator so the same struct works for both BindJSON and
// BindQuery without separate tag annotations.
func decodeValues(values url.Values, dst any) error {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return fmt.Errorf("decode: dst must be a non-nil pointer")
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("decode: dst must point to a struct")
	}
	t := v.Type()
	for i := range t.NumField() {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		name := sf.Name
		if tag := sf.Tag.Get("json"); tag != "" {
			if comma := strings.Index(tag, ","); comma >= 0 {
				tag = tag[:comma]
			}
			if tag != "" && tag != "-" {
				name = tag
			}
		}
		raw, ok := values[name]
		if !ok {
			// fallback: lowercased field name
			raw, ok = values[strings.ToLower(sf.Name)]
			if !ok {
				continue
			}
		}
		fv := v.Field(i)
		if !fv.CanSet() {
			continue
		}
		if err := assignFromStrings(fv, raw); err != nil {
			return fmt.Errorf("field %s: %w", name, err)
		}
	}
	return nil
}

func assignFromStrings(fv reflect.Value, raw []string) error {
	if len(raw) == 0 {
		return nil
	}
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(raw[0])
	case reflect.Bool:
		b, err := strconv.ParseBool(raw[0])
		if err != nil {
			return err
		}
		fv.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw[0], 10, fv.Type().Bits())
		if err != nil {
			return err
		}
		fv.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw[0], 10, fv.Type().Bits())
		if err != nil {
			return err
		}
		fv.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(raw[0], fv.Type().Bits())
		if err != nil {
			return err
		}
		fv.SetFloat(f)
	case reflect.Slice:
		elemType := fv.Type().Elem()
		slice := reflect.MakeSlice(fv.Type(), len(raw), len(raw))
		for i, s := range raw {
			elem := reflect.New(elemType).Elem()
			if err := assignFromStrings(elem, []string{s}); err != nil {
				return err
			}
			slice.Index(i).Set(elem)
		}
		fv.Set(slice)
	default:
		return fmt.Errorf("unsupported field kind %s", fv.Kind())
	}
	return nil
}

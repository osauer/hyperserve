package server

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
// The validator core lives in internal/validate so pkg/mcp (typed tools)
// can reuse it without creating an import cycle. The types are re-exported
// here via aliases so existing callers don't move.
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

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/osauer/hyperserve/v2/internal/validate"
)

// FieldError describes one failed validation rule. Aliased to the
// internal/validate type so pkg/mcp can produce the same errors when
// validating typed-tool arguments.
type FieldError = validate.FieldError

// ValidationError is the aggregate error returned by Validate / Bind* when
// one or more fields fail their rules. Aliased to internal/validate.
type ValidationError = validate.ValidationError

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
	return validate.Struct(dst)
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

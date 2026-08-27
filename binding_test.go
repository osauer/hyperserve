package hyperserve

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type createUser struct {
	Name  string   `json:"name" validate:"required,min=2,max=64"`
	Email string   `json:"email" validate:"required,email"`
	Age   int      `json:"age" validate:"required,min=13,max=120"`
	Role  string   `json:"role" validate:"oneof=admin user guest"`
	Tags  []string `json:"tags" validate:"max=5"`
}

func TestBindJSON_OK(t *testing.T) {
	body := `{"name":"Ada","email":"ada@example.com","age":36,"role":"admin","tags":["go","mcp"]}`
	r := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	var u createUser
	if err := BindJSON(r, &u); err != nil {
		t.Fatalf("BindJSON: %v", err)
	}
	if u.Name != "Ada" || u.Email != "ada@example.com" || u.Age != 36 || u.Role != "admin" {
		t.Fatalf("decoded wrong: %+v", u)
	}
	if len(u.Tags) != 2 {
		t.Fatalf("tags decoded wrong: %+v", u.Tags)
	}
}

func TestBindJSON_RequiredMissing(t *testing.T) {
	body := `{"email":"a@b.co","age":40}`
	r := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	var u createUser
	err := BindJSON(r, &u)
	if err == nil {
		t.Fatal("expected validation error")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if !verr.HasField("name") {
		t.Fatalf("expected failure for 'name', got %v", verr)
	}
}

func TestBindJSON_OneOfRejects(t *testing.T) {
	body := `{"name":"Ada","email":"ada@x.com","age":40,"role":"superuser"}`
	r := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	var u createUser
	err := BindJSON(r, &u)
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ValidationError, got %v", err)
	}
	if !verr.HasField("role") {
		t.Fatalf("expected failure for 'role', got %v", verr)
	}
}

func TestBindJSON_RejectsUnknownFields(t *testing.T) {
	body := `{"name":"Ada","email":"a@b.co","age":40,"role":"user","oops":1}`
	r := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	var u createUser
	if err := BindJSON(r, &u); err == nil {
		t.Fatal("expected decode error for unknown field")
	}
}

func TestBindJSON_BodyTooLarge(t *testing.T) {
	huge := bytes.Repeat([]byte("a"), 2<<20) // 2 MiB > 1 MiB cap
	body := `{"name":"` + string(huge) + `","email":"a@b.co","age":40,"role":"user"}`
	r := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	var u createUser
	if err := BindJSON(r, &u); err == nil {
		t.Fatal("expected decode error from size-capped body")
	}
}

type searchQuery struct {
	Q    string   `json:"q" validate:"required,min=1,max=100"`
	Tags []string `json:"tag"`
	Page int      `json:"page" validate:"min=1"`
}

func TestBindQuery(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/search?q=go&tag=mcp&tag=sse&page=2", nil)
	var q searchQuery
	if err := BindQuery(r, &q); err != nil {
		t.Fatalf("BindQuery: %v", err)
	}
	if q.Q != "go" || q.Page != 2 || len(q.Tags) != 2 {
		t.Fatalf("decoded wrong: %+v", q)
	}
}

func TestBindQuery_RequiredMissing(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/search?page=1", nil)
	var q searchQuery
	err := BindQuery(r, &q)
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	if !verr.HasField("q") {
		t.Fatalf("expected 'q' to fail, got %v", verr)
	}
}

func TestBindForm(t *testing.T) {
	form := strings.NewReader("name=Ada&email=ada@example.com&age=40&role=user")
	r := httptest.NewRequest(http.MethodPost, "/users", form)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var u createUser
	if err := BindForm(r, &u); err != nil {
		t.Fatalf("BindForm: %v", err)
	}
	if u.Name != "Ada" || u.Email != "ada@example.com" || u.Age != 40 || u.Role != "user" {
		t.Fatalf("decoded wrong: %+v", u)
	}
}

func TestBind_RoutesByContentType(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		body        string
		method      string
		url         string
	}{
		{"json", "application/json", `{"name":"Ada","email":"a@b.co","age":40,"role":"user"}`, http.MethodPost, "/u"},
		{"form", "application/x-www-form-urlencoded", "name=Ada&email=a@b.co&age=40&role=user", http.MethodPost, "/u"},
		{"query", "", "", http.MethodGet, "/u?name=Ada&email=a@b.co&age=40&role=user"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var body *strings.Reader
			if c.body != "" {
				body = strings.NewReader(c.body)
			}
			var r *http.Request
			if body != nil {
				r = httptest.NewRequest(c.method, c.url, body)
			} else {
				r = httptest.NewRequest(c.method, c.url, nil)
			}
			if c.contentType != "" {
				r.Header.Set("Content-Type", c.contentType)
			}
			var u createUser
			if err := Bind(r, &u); err != nil {
				t.Fatalf("Bind: %v", err)
			}
			if u.Name != "Ada" {
				t.Fatalf("name not bound, got %+v", u)
			}
		})
	}
}

type nested struct {
	Outer createUser `json:"outer"`
	Note  string     `json:"note" validate:"max=10"`
}

func TestValidate_NestedStructs(t *testing.T) {
	n := nested{
		Outer: createUser{Name: "A", Email: "a@b.co", Age: 30, Role: "user"},
		Note:  "ok",
	}
	err := Validate(&n)
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	if !verr.HasField("outer.name") {
		t.Fatalf("expected outer.name min violation, got %v", verr)
	}
}

func TestValidate_EmailEdgeCases(t *testing.T) {
	cases := []struct {
		email string
		ok    bool
	}{
		{"ok@example.com", true},
		{"", false},          // required-or-empty handled separately; this hits email rule
		{"missingat", false}, // no @
		{"a@b", false},       // no . in domain
		{"@example.com", false},
		{"x@.com", false},
		{"x@example.", false},
	}
	for _, c := range cases {
		t.Run(c.email, func(t *testing.T) {
			s := struct {
				E string `validate:"email"`
			}{E: c.email}
			err := Validate(&s)
			if c.email == "" {
				// email tag passes empty (so it combines with required)
				if err != nil {
					t.Fatalf("empty email should pass email-only rule: %v", err)
				}
				return
			}
			if c.ok && err != nil {
				t.Fatalf("expected pass for %q, got %v", c.email, err)
			}
			if !c.ok && err == nil {
				t.Fatalf("expected fail for %q, got nil", c.email)
			}
		})
	}
}

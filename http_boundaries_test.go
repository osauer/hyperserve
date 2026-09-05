package hyperserve

import (
	"bytes"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLoggingCopyLimitedReader(t *testing.T) {
	data := []byte("copied through Write")
	recorder := httptest.NewRecorder()
	w := &loggingResponseWriter{ResponseWriter: recorder, statusCode: http.StatusOK}
	n, err := io.Copy(w, io.LimitReader(bytes.NewReader(data), int64(len(data))))
	if err != nil || n != int64(len(data)) || w.bytesWritten != len(data) || !bytes.Equal(recorder.Body.Bytes(), data) {
		t.Fatalf("copy = %d, %v; counted %d; body %q", n, err, w.bytesWritten, recorder.Body.String())
	}
}

func TestLoggingServeContentHTTP2(t *testing.T) {
	app, err := New(WithLogLevel("INFO"))
	if err != nil {
		t.Fatal(err)
	}
	data := strings.Repeat("HTTP/2 file content\n", 100)
	app.HandleFunc("/file", func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "file.txt", time.Time{}, strings.NewReader(data))
	})
	server := httptest.NewUnstartedServer(app.Handler())
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client := server.Client()
	client.Timeout = 3 * time.Second
	response, err := client.Get(server.URL + "/file")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.ProtoMajor != 2 || string(body) != data {
		t.Fatalf("response: protocol %s, bytes %d, error %v", response.Proto, len(body), err)
	}
}

func TestQueryAndFormExcludeJSONFields(t *testing.T) {
	for _, key := range []string{"Internal", "internal"} {
		for _, form := range []bool{false, true} {
			var dst struct {
				Internal bool   `json:"-"`
				Name     string `json:"name"`
			}
			values := key + "=true&name=Ada"
			r := httptest.NewRequest(http.MethodPost, "/?"+values, strings.NewReader(values))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			bind := BindQuery
			if form {
				bind = BindForm
			}
			if err := bind(r, &dst); err != nil || dst.Internal || dst.Name != "Ada" {
				t.Fatalf("form=%t key=%s: %+v, %v", form, key, dst, err)
			}
		}
	}
}

func TestFloatBoundsRejectNaN(t *testing.T) {
	for _, value := range []string{"NaN", "+Inf", "-Inf", "5"} {
		for _, form := range []bool{false, true} {
			var dst struct {
				Value float64 `json:"value" validate:"required,min=1,max=10"`
			}
			r := httptest.NewRequest(http.MethodPost, "/?value="+strings.ReplaceAll(value, "+", "%2B"), strings.NewReader("value="+strings.ReplaceAll(value, "+", "%2B")))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			bind := BindQuery
			if form {
				bind = BindForm
			}
			if err := bind(r, &dst); (err == nil) != (value == "5") {
				t.Fatalf("value=%s form=%t error=%v", value, form, err)
			}
		}
	}
	for _, tag := range []string{`validate:"min=1"`, `validate:"max=10"`, `validate:"min=NaN"`, `validate:"max=NaN"`} {
		typeOf := reflect.StructOf([]reflect.StructField{{Name: "Value", Type: reflect.TypeFor[float64](), Tag: reflect.StructTag(tag)}})
		dst := reflect.New(typeOf)
		dst.Elem().Field(0).SetFloat(math.NaN())
		if err := Validate(dst.Interface()); err == nil {
			t.Fatalf("NaN accepted by %s", tag)
		}
		dst.Elem().Field(0).SetFloat(5)
		if err := Validate(dst.Interface()); (err != nil) != strings.Contains(tag, "NaN") {
			t.Fatalf("bound %s: %v", tag, err)
		}
	}
}

func TestSSEDataLineEndings(t *testing.T) {
	for _, ending := range []string{"\n", "\r\n", "\r"} {
		data := strings.Join([]string{"first", "", "event: injected", "data: tail"}, ending)
		for _, value := range []any{data, []byte(data)} {
			got := (&SSEMessage{Event: "message", Data: value}).String()
			want := "event: message\ndata: first\ndata: \ndata: event: injected\ndata: data: tail\n\n"
			if got != want {
				t.Fatalf("ending=%q type=%T: %q", ending, value, got)
			}
		}
	}
}

package hyperserve_test

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	hs "github.com/osauer/hyperserve/v2"
)

func TestStaticMountIndex(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("index witness"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, mount := range []string{"/", "/static/"} {
		t.Run(mount, func(t *testing.T) {
			app, err := hs.New(hs.WithStaticDir(root))
			if err != nil {
				t.Fatal(err)
			}
			defer app.Shutdown(context.Background())
			if err := app.HandleStatic(mount); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{mount, mount + "index.html"} {
				w := httptest.NewRecorder()
				app.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
				t.Logf("mount=%s path=%s status=%d body=%q", mount, path, w.Code, w.Body.String())
				if w.Code != http.StatusOK || w.Body.String() != "index witness" {
					t.Errorf("documented index request failed")
				}
			}
		})
	}
}

func TestMultipartFieldsBind(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("name", "Ada"); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	req := httptest.NewRequest("POST", "/", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	var dst struct {
		Name string `json:"name" validate:"required"`
	}
	defer func() {
		if req.MultipartForm != nil {
			req.MultipartForm.RemoveAll()
		}
	}()
	if err := req.ParseMultipartForm(0); err != nil {
		t.Fatal(err)
	}
	err := hs.BindForm(req, &dst)
	t.Logf("name=%q error=%v", dst.Name, err)
	if err != nil || dst.Name != "Ada" {
		t.Error("documented multipart form is not decoded")
	}
}

// BindForm has always left multipart parsing and its budget to the caller.
func TestMultipartBindingPreservesCallerParsing(t *testing.T) {
	for _, parsed := range []bool{false, true} {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		writer.WriteField("name", "body")
		file, err := writer.CreateFormFile("file", "large.txt")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(make([]byte, 11<<20)); err != nil {
			t.Fatal(err)
		}
		writer.Close()
		original := append([]byte(nil), body.Bytes()...)
		req := httptest.NewRequest("POST", "/?name=query", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		if parsed {
			req.Body = http.MaxBytesReader(nil, req.Body, 12<<20)
			if err := req.ParseMultipartForm(0); err != nil {
				t.Fatal(err)
			}
			defer req.MultipartForm.RemoveAll()
		}
		var dst struct {
			Name string `json:"name"`
		}
		if err := hs.BindForm(req, &dst); err != nil {
			t.Fatalf("parsed=%v: %v", parsed, err)
		}
		if dst.Name != "query" {
			t.Fatalf("parsed=%v: caller form precedence changed: %q", parsed, dst.Name)
		}
		if !parsed {
			remaining, err := io.ReadAll(req.Body)
			if err != nil || !bytes.Equal(remaining, original) {
				t.Fatalf("BindForm consumed or limited the caller's multipart body: %v", err)
			}
		} else if req.MultipartForm.File["file"][0].Size != 11<<20 {
			t.Fatal("caller-parsed file lost")
		}
	}
}

package hyperserve_test

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	hs "github.com/osauer/hyperserve"
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
	err := hs.BindForm(req, &dst)
	t.Logf("name=%q error=%v", dst.Name, err)
	if err != nil || dst.Name != "Ada" {
		t.Error("documented multipart form is not decoded")
	}
}

func TestMultipartBindingBoundsFileBodies(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "large.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(make([]byte, 11<<20)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	defer func() {
		if req.MultipartForm != nil {
			req.MultipartForm.RemoveAll()
		}
	}()
	var dst struct {
		Name string `json:"name"`
	}
	if err := hs.BindForm(req, &dst); err == nil {
		t.Fatal("multipart file exceeded the standard form body bound")
	}
}

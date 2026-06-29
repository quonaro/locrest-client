package httpclient

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %q", r.Method)
		}
		w.Write([]byte("hello"))
	}))
	defer srv.Close()

	body, err := Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("body = %q", body)
	}
}

func TestGetError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadRequest)
	}))
	defer srv.Close()

	_, err := Get(srv.URL)
	if err == nil {
		t.Fatal("expected error")
	}
	httpErr, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("expected *HTTPError, got %T", err)
	}
	if httpErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", httpErr.StatusCode)
	}
	if !strings.Contains(httpErr.Body, "nope") {
		t.Fatalf("body = %q", httpErr.Body)
	}
}

func TestPostSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %q", r.Method)
		}
		ct := r.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "application/json") {
			t.Fatalf("Content-Type = %q", ct)
		}
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		w.Write(body)
	}))
	defer srv.Close()

	body, err := Post(srv.URL, []byte(`{"ok":true}`))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if !strings.Contains(string(body), `"ok":true`) {
		t.Fatalf("body = %q", body)
	}
}

func TestPostError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("boom"))
	}))
	defer srv.Close()

	_, err := Post(srv.URL, []byte(""))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error = %v", err)
	}
}

func TestHTTPErrorString(t *testing.T) {
	e := &HTTPError{StatusCode: 404, Body: "not found"}
	want := fmt.Sprintf("HTTP %d: not found", http.StatusNotFound)
	if e.Error() != want {
		t.Fatalf("Error() = %q, want %q", e.Error(), want)
	}
}

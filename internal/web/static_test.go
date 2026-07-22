package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func staticServer(t *testing.T) http.Handler {
	t.Helper()
	sub, err := staticSubFS()
	if err != nil {
		t.Fatalf("staticSubFS: %v", err)
	}
	return newStaticHandler(sub)
}

func TestStaticServesWithETag(t *testing.T) {
	h := staticServer(t)

	req := httptest.NewRequest(http.MethodGet, "/static/style.css", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("Etag") == "" {
		t.Error("expected an ETag so browsers can revalidate")
	}
	if rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", rec.Header().Get("Cache-Control"))
	}
	if rec.Body.Len() == 0 {
		t.Error("expected the stylesheet body")
	}
}

func TestStaticReturns304WhenETagMatches(t *testing.T) {
	h := staticServer(t)

	// First request to learn the ETag.
	first := httptest.NewRecorder()
	h.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/static/app.js", nil))
	etag := first.Header().Get("Etag")
	if etag == "" {
		t.Fatal("no ETag on first response")
	}

	// Second request presenting that ETag should be told nothing changed.
	req := httptest.NewRequest(http.MethodGet, "/static/app.js", nil)
	req.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304 when the ETag matches", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Error("a 304 must not resend the body")
	}
}

func TestStaticETagDiffersByFile(t *testing.T) {
	h := staticServer(t).(*staticHandler)

	css := h.etags["style.css"]
	js := h.etags["app.js"]
	if css == "" || js == "" {
		t.Fatal("expected ETags for both assets")
	}
	if css == js {
		t.Error("different files should hash to different ETags")
	}
}

func TestStaticMissingFileIs404(t *testing.T) {
	h := staticServer(t)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/does-not-exist.css", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestStaticRejectsTraversal(t *testing.T) {
	h := staticServer(t)

	for _, p := range []string{"/static/../handler.go", "/static/../../go.mod"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code == http.StatusOK {
			t.Errorf("%s should not be served", p)
		}
	}
}

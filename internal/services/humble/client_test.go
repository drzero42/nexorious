package humble

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/time/rate"
)

// newTestClient points a Client at the given test server with an unlimited rate
// limiter so tests don't sleep.
func newTestClient(srv *httptest.Server) *Client {
	c := NewClient()
	c.SetHTTPClient(srv.Client())
	c.SetBaseURL(srv.URL)
	c.SetLimiter(rate.NewLimiter(rate.Inf, 1))
	return c
}

func TestVerify_SendsCookieAndHeader(t *testing.T) {
	var gotCookie, gotRequestedBy, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		gotRequestedBy = r.Header.Get("X-Requested-By")
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	if err := newTestClient(srv).Verify(context.Background(), "cookie123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotCookie != "_simpleauth_sess=cookie123" {
		t.Errorf("Cookie header = %q, want %q", gotCookie, "_simpleauth_sess=cookie123")
	}
	if gotRequestedBy != "hb_android_app" {
		t.Errorf("X-Requested-By = %q, want hb_android_app", gotRequestedBy)
	}
	if gotPath != "/api/v1/user/order" {
		t.Errorf("path = %q, want /api/v1/user/order", gotPath)
	}
}

func TestVerify_401ReturnsErrCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	err := newTestClient(srv).Verify(context.Background(), "bad")
	if !errors.Is(err, ErrCredentials) {
		t.Errorf("expected ErrCredentials on 401, got %v", err)
	}
}

func TestListGamekeys_DecodesAndFiltersEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"gamekey":"AAA"},{"gamekey":""},{"gamekey":"BBB"}]`))
	}))
	defer srv.Close()

	keys, err := newTestClient(srv).ListGamekeys(context.Background(), "cookie")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 2 || keys[0] != "AAA" || keys[1] != "BBB" {
		t.Errorf("keys = %v, want [AAA BBB]", keys)
	}
}

func TestGetOrder_DecodesNestedStructure(t *testing.T) {
	const orderJSON = `{
	  "gamekey": "GK1",
	  "subproducts": [
	    {
	      "machine_name": "aquaria",
	      "human_name": "Aquaria",
	      "downloads": [
	        {"platform":"windows","machine_name":"aquaria_win","download_struct":[{"url":{"web":"https://dl.example/aquaria.zip"}}]}
	      ]
	    },
	    {
	      "machine_name": "world_of_goo_ebook",
	      "human_name": "World of Goo (ebook)",
	      "downloads": [
	        {"platform":"ebook","machine_name":"wog_pdf","download_struct":[{"url":{"web":"https://dl.example/wog.pdf"}}]}
	      ]
	    }
	  ]
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("all_tpkds") != "true" {
			t.Errorf("expected all_tpkds=true, got query %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(orderJSON))
	}))
	defer srv.Close()

	order, err := newTestClient(srv).GetOrder(context.Background(), "cookie", "GK1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if order.Gamekey != "GK1" || len(order.Subproducts) != 2 {
		t.Fatalf("decoded order = %+v", order)
	}
	sp := order.Subproducts[0]
	if sp.MachineName != "aquaria" || sp.HumanName != "Aquaria" {
		t.Errorf("subproduct[0] = %+v", sp)
	}
	if len(sp.Downloads) != 1 || sp.Downloads[0].Platform != "windows" ||
		sp.Downloads[0].DownloadStruct[0].URL.Web != "https://dl.example/aquaria.zip" {
		t.Errorf("download not decoded: %+v", sp.Downloads)
	}
}

func TestGetOrder_403ReturnsErrCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := newTestClient(srv).GetOrder(context.Background(), "bad", "GK1")
	if !errors.Is(err, ErrCredentials) {
		t.Errorf("expected ErrCredentials on 403, got %v", err)
	}
}

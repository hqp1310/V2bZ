package panel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ZicBoard/ZicNode/conf"
)

func TestReportCertStatusReturnsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/server/UniProxy/cert/report" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		http.Error(w, "cert callback rejected", http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := New(&conf.NodeConfig{APIHost: server.URL, NodeID: 1, Key: "token"})
	if err != nil {
		t.Fatal(err)
	}

	err = client.ReportCertStatus(context.Background(), &CertReport{Status: "ok", Target: "tls.example.com"})
	if err == nil {
		t.Fatal("expected HTTP status error")
	}
	if !strings.Contains(err.Error(), "status=500") || !strings.Contains(err.Error(), "cert callback rejected") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReportCertStatusAcceptsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/server/UniProxy/cert/report" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":true}`))
	}))
	defer server.Close()

	client, err := New(&conf.NodeConfig{APIHost: server.URL, NodeID: 1, Key: "token"})
	if err != nil {
		t.Fatal(err)
	}

	if err := client.ReportCertStatus(context.Background(), &CertReport{Status: "ok", Target: "tls.example.com"}); err != nil {
		t.Fatalf("ReportCertStatus returned error: %v", err)
	}
}

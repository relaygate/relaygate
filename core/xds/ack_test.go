package xds

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWaitEnvoyAppliedAdminUnreachable(t *testing.T) {
	t.Parallel()
	err := WaitEnvoyApplied("http://127.0.0.1:1", "v1", 200*time.Millisecond)
	if err == nil {
		t.Fatal("expected error when admin unreachable")
	}
}

func TestWaitEnvoyAppliedSeesVersion(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/stats", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "cluster_manager.cds.update_rejected: 0\nlistener_manager.lds.update_rejected: 0\n")
	})
	mux.HandleFunc("/config_dump", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"version_info":"42"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	if err := WaitEnvoyApplied(srv.URL, "42", 2*time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestWaitEnvoyAppliedRejectIncrease(t *testing.T) {
	t.Parallel()
	n := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/stats", func(w http.ResponseWriter, _ *http.Request) {
		// First read = baseline 0; subsequent reads bump rejected.
		fmt.Fprintf(w, "cluster_manager.cds.update_rejected: %d\nlistener_manager.lds.update_rejected: 0\n", n)
		if n == 0 {
			n = 1
		}
	})
	mux.HandleFunc("/config_dump", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"version_info":"old"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	err := WaitEnvoyApplied(srv.URL, "new", 2*time.Second)
	if err == nil {
		t.Fatal("expected reject error")
	}
}

func TestParseStatsCounter(t *testing.T) {
	t.Parallel()
	body := "foo: 1\ncluster_manager.cds.update_rejected: 3\n"
	n, ok := parseStatsCounter(body, "cluster_manager.cds.update_rejected")
	if !ok || n != 3 {
		t.Fatalf("got %d ok=%v", n, ok)
	}
}

package agent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPullOnceStatusErrorsAreActionable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status int
		body   string
		want   string
	}{
		{
			status: http.StatusNotFound,
			body:   `{"error":"尚无已发布的配置版本。请先在主控执行 relaygate fleet publish"}`,
			want:   "fleet publish",
		},
		{
			status: http.StatusUnauthorized,
			body:   `{"error":"未授权的代理令牌"}`,
			want:   "令牌",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.body, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			client := &Client{ControlURL: srv.URL, Token: "t", HTTP: srv.Client()}
			_, err := client.PullOnce(t.TempDir())
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
			// A missing publish must never be reported as a bad token.
			if tc.status == http.StatusNotFound && strings.Contains(err.Error(), "令牌") {
				t.Fatalf("404 misreported as token failure: %v", err)
			}
		})
	}
}

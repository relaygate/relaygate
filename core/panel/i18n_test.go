package panel

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveLang(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := resolveLang(req); got != langDefault {
		t.Fatalf("default lang=%s want %s", got, langDefault)
	}

	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if got := resolveLang(req); got != langEnglish {
		t.Fatalf("accept-language en=%s", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: langCookie, Value: "en"})
	req.Header.Set("Accept-Language", "zh-CN")
	if got := resolveLang(req); got != langEnglish {
		t.Fatalf("cookie should win: %s", got)
	}
}

func TestBundleT(t *testing.T) {
	b, err := loadEmbeddedBundle()
	if err != nil {
		t.Fatal(err)
	}
	zh := b.T(langChinese, "error.standby")
	en := b.T(langEnglish, "error.standby")
	if zh == "" || en == "" || zh == en {
		t.Fatalf("expected different translations: zh=%q en=%q", zh, en)
	}
	got := b.T(langEnglish, "error.confirm_typed", "ROLLBACK")
	if !strings.Contains(got, "ROLLBACK") {
		t.Fatalf("format failed: %q", got)
	}
}

func TestLangSwitchSetsCookie(t *testing.T) {
	srv, _, _ := setupPanel(t)
	h := srv.Handler()
	req := httptest.NewRequest(http.MethodGet, "/lang?set=en&next=/login", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status=%d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("location=%s", loc)
	}
	var gotLang string
	for _, c := range rec.Result().Cookies() {
		if c.Name == langCookie {
			gotLang = c.Value
		}
	}
	if gotLang != langEnglish {
		t.Fatalf("lang cookie=%q", gotLang)
	}
}

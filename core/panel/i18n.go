package panel

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	langCookie  = "panel_lang"
	langDefault = "zh-CN"
	langEnglish = "en"
	langChinese = "zh-CN"
)

// Bundle holds per-language message catalogs loaded from frontend/i18n/*.json.
type Bundle struct {
	cats map[string]map[string]string
}

func loadBundle(frontendDir string) (*Bundle, error) {
	dir := filepath.Join(frontendDir, "i18n")
	b := &Bundle{cats: map[string]map[string]string{}}
	for _, lang := range []string{langEnglish, langChinese} {
		path := filepath.Join(dir, lang+".json")
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("load i18n %s: %w", path, err)
		}
		cat := map[string]string{}
		if err := json.Unmarshal(raw, &cat); err != nil {
			return nil, fmt.Errorf("parse i18n %s: %w", path, err)
		}
		b.cats[lang] = cat
	}
	return b, nil
}

func normalizeLang(raw string) string {
	s := strings.TrimSpace(strings.ToLower(raw))
	s = strings.ReplaceAll(s, "_", "-")
	if s == "" {
		return ""
	}
	switch {
	case s == "en" || strings.HasPrefix(s, "en-"):
		return langEnglish
	case s == "zh" || s == "zh-cn" || s == "zh-hans" || strings.HasPrefix(s, "zh-cn") || strings.HasPrefix(s, "zh-hans"):
		return langChinese
	default:
		return ""
	}
}

func resolveLang(r *http.Request) string {
	if r == nil {
		return langDefault
	}
	if c, err := r.Cookie(langCookie); err == nil {
		if lang := normalizeLang(c.Value); lang != "" {
			return lang
		}
	}
	if lang := normalizeLang(r.URL.Query().Get("lang")); lang != "" {
		return lang
	}
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		tag := strings.TrimSpace(strings.Split(part, ";")[0])
		if lang := normalizeLang(tag); lang != "" {
			return lang
		}
	}
	return langDefault
}

func (b *Bundle) T(lang, key string, args ...any) string {
	if b == nil {
		return key
	}
	if cat := b.cats[lang]; cat != nil {
		if msg, ok := cat[key]; ok {
			return formatMsg(msg, args...)
		}
	}
	if lang != langChinese {
		if cat := b.cats[langChinese]; cat != nil {
			if msg, ok := cat[key]; ok {
				return formatMsg(msg, args...)
			}
		}
	}
	if lang != langEnglish {
		if cat := b.cats[langEnglish]; cat != nil {
			if msg, ok := cat[key]; ok {
				return formatMsg(msg, args...)
			}
		}
	}
	return key
}

func formatMsg(msg string, args ...any) string {
	if len(args) == 0 {
		return msg
	}
	// Use a non-constant format indirection so go vet does not treat i18n keys as printf formats.
	format := msg + ""
	return fmt.Sprintf(format, args...)
}

func (b *Bundle) TFunc(lang string) func(string, ...any) string {
	return func(key string, args ...any) string {
		return b.T(lang, key, args...)
	}
}

func (s *Server) t(r *http.Request, key string, args ...any) string {
	return s.i18n.T(resolveLang(r), key, args...)
}

func setLangCookie(w http.ResponseWriter, lang string) {
	http.SetCookie(w, &http.Cookie{
		Name:     langCookie,
		Value:    lang,
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60,
		SameSite: http.SameSiteLaxMode,
		HttpOnly: true,
	})
}

func safeNextPath(raw string) string {
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "/"
	}
	if strings.ContainsAny(raw, "\r\n") {
		return "/"
	}
	return raw
}

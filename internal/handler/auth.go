package handler

import (
	"net/http"

	"suyasuya/internal/domain"
	"suyasuya/internal/platform"
)

func (h *Handler) Login(w http.ResponseWriter, r *http.Request, _ domain.User) error {
	var in domain.LoginInput
	if e := decode(w, r, &in); e != nil {
		return e
	}
	token, e := h.service.Login(r.Context(), in)
	if e != nil {
		return e
	}
	sessionCookie(w, r, token, 7*86400)
	return reply(w, map[string]bool{"ok": true})
}
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request, u domain.User) error {
	c, _ := r.Cookie("suyasuya_session")
	if e := h.service.Logout(r.Context(), u, c.Value); e != nil {
		return e
	}
	sessionCookie(w, r, "", -1)
	return reply(w, map[string]bool{"ok": true})
}
func (h *Handler) Me(w http.ResponseWriter, r *http.Request, u domain.User) error {
	return reply(w, h.service.Me(u))
}
func sessionCookie(w http.ResponseWriter, r *http.Request, token string, age int) {
	http.SetCookie(w, &http.Cookie{Name: "suyasuya_session", Value: token, Path: "/", HttpOnly: true, Secure: secureCookie(r), SameSite: http.SameSiteStrictMode, MaxAge: age})
}

// secureCookie は接続ごとに Secure を決める。tunnel 経由（HTTPS）なら立てて session を
// 平文へ漏らさず、http://localhost:8080 では立てない（立てると cookie が保存されない）。
// cloudflared が X-Forwarded-Proto を付けるのでそれで判別する。
// APP_SECURE_COOKIE=true は常時 Secure を強制する上書き。
func secureCookie(r *http.Request) bool {
	return platform.Env("APP_SECURE_COOKIE", "false") == "true" ||
		r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

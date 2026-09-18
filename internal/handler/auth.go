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
	sessionCookie(w, token, 7*86400)
	return reply(w, map[string]bool{"ok": true})
}
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request, u domain.User) error {
	c, _ := r.Cookie("suyasuya_session")
	if e := h.service.Logout(r.Context(), u, c.Value); e != nil {
		return e
	}
	sessionCookie(w, "", -1)
	return reply(w, map[string]bool{"ok": true})
}
func (h *Handler) Me(w http.ResponseWriter, r *http.Request, u domain.User) error {
	return reply(w, h.service.Me(u))
}
func sessionCookie(w http.ResponseWriter, token string, age int) {
	http.SetCookie(w, &http.Cookie{Name: "suyasuya_session", Value: token, Path: "/", HttpOnly: true, Secure: platform.Env("APP_SECURE_COOKIE", "false") == "true", SameSite: http.SameSiteStrictMode, MaxAge: age})
}

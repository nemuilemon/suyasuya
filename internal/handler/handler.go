package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"suyasuya/internal/domain"
	"suyasuya/internal/platform"
	"suyasuya/internal/service"
)

type Handler struct{ service *service.Control }
type endpoint func(http.ResponseWriter, *http.Request, domain.User) error

func New(service *service.Control) *Handler               { return &Handler{service: service} }
func (h *Handler) Protected(fn endpoint) http.HandlerFunc { return h.wrap(true, fn) }
func (h *Handler) Public(fn endpoint) http.HandlerFunc    { return h.wrap(false, fn) }
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if h.service.Health(ctx) != nil {
		http.Error(w, "unavailable", 503)
		return
	}
	w.Write([]byte("ok"))
}
func (a *Handler) wrap(auth bool, h endpoint) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			if r.Header.Get("X-Suyasuya") != "1" {
				http.Error(w, "invalid request", 403)
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				u, e := url.Parse(origin)
				if e != nil || u.Host != r.Host {
					http.Error(w, "invalid origin", 403)
					return
				}
			}
		}
		var user domain.User
		if auth {
			c, e := r.Cookie("suyasuya_session")
			if e != nil {
				http.Error(w, "ログインしてください", 401)
				return
			}
			user, e = a.service.Authenticate(r.Context(), c.Value)
			if e != nil {
				http.Error(w, "ログインしてください", 401)
				return
			}
		}
		if e := h(w, r, user); e != nil {
			var ae domain.Error
			if errors.As(e, &ae) {
				http.Error(w, ae.Message, ae.Code)
			} else {
				log.Printf("%s %s: %s", r.Method, r.URL.Path, platform.SafeError(e))
				http.Error(w, "処理に失敗しました。時間をおいて再試行してください。", 500)
			}
		}
	}
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return domain.Invalid("入力形式を確認してください")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return domain.Invalid("余分な入力があります")
	}
	return nil
}
func reply(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(v)
}

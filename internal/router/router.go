package router

import (
	"net/http"

	"suyasuya/internal/handler"
	"suyasuya/internal/web"
)

// New owns the control application's HTTP route table and middleware.
func New(h *handler.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.Health)
	mux.HandleFunc("POST /api/login", h.Public(h.Login))
	mux.HandleFunc("POST /api/logout", h.Protected(h.Logout))
	mux.HandleFunc("GET /api/me", h.Protected(h.Me))
	mux.HandleFunc("GET /api/records", h.Protected(h.Records))
	mux.HandleFunc("POST /api/records", h.Protected(h.SaveRecord))
	mux.HandleFunc("GET /api/records/{id}/history", h.Protected(h.History))
	mux.HandleFunc("GET /api/jobs", h.Protected(h.Jobs))
	mux.HandleFunc("DELETE /api/jobs", h.Protected(h.DeleteJobs))
	mux.HandleFunc("POST /api/jobs/{id}/retry", h.Protected(h.Retry))
	mux.HandleFunc("POST /api/jobs/{id}/cancel", h.Protected(h.CancelJob))
	mux.HandleFunc("POST /api/uploads", h.Protected(h.CreateUpload))
	mux.HandleFunc("POST /api/uploads/{id}/complete", h.Protected(h.CompleteUpload))
	mux.HandleFunc("GET /api/uploads/{id}/view", h.Protected(h.ViewUpload))
	mux.HandleFunc("GET /api/settings", h.Protected(h.Settings))
	mux.HandleFunc("PUT /api/settings", h.Protected(h.SaveSettings))
	mux.HandleFunc("GET /api/pvt", h.Protected(h.PVTList))
	mux.HandleFunc("POST /api/pvt", h.Protected(h.PVTSave))
	mux.HandleFunc("GET /api/prompts", h.Protected(h.Prompts))
	mux.HandleFunc("POST /api/prompts", h.Protected(h.SavePrompt))
	mux.Handle("/", web.Handler())
	return security(mux)
}
func security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' blob: https://storage.googleapis.com; connect-src 'self' https://storage.googleapis.com; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

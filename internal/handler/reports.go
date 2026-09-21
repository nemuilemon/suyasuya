package handler

import (
	"net/http"
	"suyasuya/internal/domain"
	"suyasuya/internal/service"
)

func (h *Handler) Analytics(w http.ResponseWriter, r *http.Request, u domain.User) error {
	out, err := h.service.Analytics(r.Context(), u, r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		return err
	}
	return reply(w, out)
}
func (h *Handler) Reports(w http.ResponseWriter, r *http.Request, u domain.User) error {
	out, err := h.service.Reports(r.Context(), u)
	if err != nil {
		return err
	}
	return reply(w, out)
}
func (h *Handler) GenerateReport(w http.ResponseWriter, r *http.Request, u domain.User) error {
	var in service.ReportRequest
	if err := decode(w, r, &in); err != nil {
		return err
	}
	out, err := h.service.GenerateReport(r.Context(), u, in)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	return reply(w, out)
}

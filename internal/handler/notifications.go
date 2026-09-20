package handler

import (
	"net/http"
	"suyasuya/internal/domain"
)

func (h *Handler) Notifications(w http.ResponseWriter, r *http.Request, u domain.User) error {
	out, err := h.service.Notifications(r.Context(), u)
	if err != nil {
		return err
	}
	return reply(w, out)
}
func (h *Handler) TestNotification(w http.ResponseWriter, r *http.Request, u domain.User) error {
	var in struct {
		ID string `json:"submission_id"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	out, err := h.service.TestNotification(r.Context(), u, in.ID)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	return reply(w, out)
}

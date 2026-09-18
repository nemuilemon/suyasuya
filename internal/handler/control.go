package handler

import (
	"net/http"

	"suyasuya/internal/domain"
)

func (h *Handler) Records(w http.ResponseWriter, r *http.Request, u domain.User) error {
	out, e := h.service.Records(r.Context(), u)
	if e != nil {
		return e
	}
	return reply(w, out)
}

func (h *Handler) History(w http.ResponseWriter, r *http.Request, u domain.User) error {
	out, e := h.service.History(r.Context(), u, r.PathValue("id"))
	if e != nil {
		return e
	}
	return reply(w, out)
}

func (h *Handler) SaveRecord(w http.ResponseWriter, r *http.Request, u domain.User) error {
	var in domain.RecordInput
	if e := decode(w, r, &in); e != nil {
		return e
	}
	out, e := h.service.SaveRecord(r.Context(), u, in)
	if e != nil {
		return e
	}
	return reply(w, out)
}

func (h *Handler) Settings(w http.ResponseWriter, r *http.Request, u domain.User) error {
	out, e := h.service.Settings(r.Context(), u)
	if e != nil {
		return e
	}
	return reply(w, out)
}

func (h *Handler) SaveSettings(w http.ResponseWriter, r *http.Request, u domain.User) error {
	var in domain.SettingsInput
	if e := decode(w, r, &in); e != nil {
		return e
	}
	out, e := h.service.SaveSettings(r.Context(), u, in)
	if e != nil {
		return e
	}
	return reply(w, out)
}

func (h *Handler) Prompts(w http.ResponseWriter, r *http.Request, u domain.User) error {
	out, e := h.service.Prompts(r.Context(), u)
	if e != nil {
		return e
	}
	return reply(w, out)
}

func (h *Handler) SavePrompt(w http.ResponseWriter, r *http.Request, u domain.User) error {
	var in domain.PromptInput
	if e := decode(w, r, &in); e != nil {
		return e
	}
	out, e := h.service.SavePrompt(r.Context(), u, in)
	if e != nil {
		return e
	}
	return reply(w, out)
}

func (h *Handler) CreateUpload(w http.ResponseWriter, r *http.Request, u domain.User) error {
	var in domain.UploadInput
	if e := decode(w, r, &in); e != nil {
		return e
	}
	out, e := h.service.CreateUpload(r.Context(), u, in)
	if e != nil {
		return e
	}
	return reply(w, out)
}

func (h *Handler) CompleteUpload(w http.ResponseWriter, r *http.Request, u domain.User) error {
	out, e := h.service.CompleteUpload(r.Context(), u, r.PathValue("id"))
	if e != nil {
		return e
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	return reply(w, out)
}

func (h *Handler) ViewUpload(w http.ResponseWriter, r *http.Request, u domain.User) error {
	out, e := h.service.ViewUpload(r.Context(), u, r.PathValue("id"))
	if e != nil {
		return e
	}
	return reply(w, out)
}

func (h *Handler) PVTList(w http.ResponseWriter, r *http.Request, u domain.User) error {
	out, e := h.service.PVTList(r.Context(), u)
	if e != nil {
		return e
	}
	return reply(w, out)
}

func (h *Handler) PVTSave(w http.ResponseWriter, r *http.Request, u domain.User) error {
	var in domain.PVTInput
	if e := decode(w, r, &in); e != nil {
		return e
	}
	out, e := h.service.PVTSave(r.Context(), u, in)
	if e != nil {
		return e
	}
	return reply(w, out)
}

func (h *Handler) Jobs(w http.ResponseWriter, r *http.Request, u domain.User) error {
	out, e := h.service.Jobs(r.Context(), u)
	if e != nil {
		return e
	}
	return reply(w, out)
}

func (h *Handler) Retry(w http.ResponseWriter, r *http.Request, u domain.User) error {
	out, e := h.service.Retry(r.Context(), u, r.PathValue("id"))
	if e != nil {
		return e
	}
	return reply(w, out)
}

func (h *Handler) CancelJob(w http.ResponseWriter, r *http.Request, u domain.User) error {
	out, e := h.service.CancelJob(r.Context(), u, r.PathValue("id"))
	if e != nil {
		return e
	}
	return reply(w, out)
}

package httpapi

import (
	"hwops/internal/domain"
	"net/http"
)

func (h *handler) createIncident(w http.ResponseWriter, r *http.Request) {
	var in domain.IncidentInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.app.CreateIncident(r.Context(), in, r.Header.Get("Idempotency-Key"))
	result(w, http.StatusCreated, out, err)
}
func (h *handler) createRun(w http.ResponseWriter, r *http.Request) {
	var in struct{}
	if !decode(w, r, &in) {
		return
	}
	out, err := h.app.CreateRun(r.Context(), r.PathValue("id"))
	result(w, http.StatusAccepted, out, err)
}
func (h *handler) run(w http.ResponseWriter, r *http.Request) {
	out, err := h.app.Run(r.Context(), r.PathValue("id"))
	result(w, http.StatusOK, out, err)
}
func (h *handler) resumeRun(w http.ResponseWriter, r *http.Request) {
	var in domain.RunResumeInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.app.ResumeRun(r.Context(), r.PathValue("id"), in)
	result(w, http.StatusAccepted, out, err)
}
func (h *handler) cancelRun(w http.ResponseWriter, r *http.Request) {
	var in domain.RunResumeInput
	if !decode(w, r, &in) {
		return
	}
	out, err := h.app.CancelRun(r.Context(), r.PathValue("id"), in)
	result(w, http.StatusOK, out, err)
}

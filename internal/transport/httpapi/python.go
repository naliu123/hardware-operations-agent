package httpapi

import (
	"mime"
	"net/http"
	"strings"
	"time"
)

func (h *handler) pythonExecution(w http.ResponseWriter, r *http.Request) {
	e, err := h.app.PythonExecution(r.Context(), r.PathValue("id"))
	result(w, http.StatusOK, e, err)
}

func (h *handler) artifact(w http.ResponseWriter, r *http.Request) {
	a, file, err := h.app.Artifact(r.Context(), r.PathValue("id"))
	if err != nil {
		result(w, 0, nil, err)
		return
	}
	defer file.Close()
	disposition := "attachment"
	// Only inert raster images can be displayed inline from private outputs.
	if a.MediaType == "image/png" || a.MediaType == "image/jpeg" || a.MediaType == "image/webp" {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", a.MediaType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": strings.TrimSpace(a.Name)}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	http.ServeContent(w, r, a.Name, time.Time{}, file)
}

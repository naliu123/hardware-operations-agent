package httpapi

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"hwops/internal/attachments"
	"hwops/internal/domain"
)

func (h *handler) uploadAttachment(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, domain.AttachmentFileLimit+(1<<20))
	defer r.Body.Close()
	reader, err := r.MultipartReader()
	if err != nil {
		result(w, 0, nil, fmtInvalid("请求应为 multipart/form-data。"))
		return
	}
	part, err := reader.NextPart()
	if err != nil || part.FormName() != "file" || strings.TrimSpace(part.FileName()) == "" {
		result(w, 0, nil, fmtInvalid("multipart 请求必须包含一个 file 文件字段。"))
		return
	}
	attachment, err := h.app.UploadAttachment(r.Context(), r.PathValue("id"), part.FileName(), part)
	part.Close()
	if err == nil {
		// A single upload maps to one immutable attachment identity. Additional
		// multipart fields are not interpreted as files or message references.
		if extra, nextErr := reader.NextPart(); nextErr == nil {
			extra.Close()
		}
	}
	result(w, http.StatusCreated, attachment, err)
}

func fmtInvalid(message string) error {
	return fmt.Errorf("%w: %s", domain.ErrInvalid, message)
}

func (h *handler) attachment(w http.ResponseWriter, r *http.Request) {
	value, err := h.app.Attachment(r.Context(), r.PathValue("id"))
	result(w, http.StatusOK, value, err)
}

func (h *handler) attachmentContent(w http.ResponseWriter, r *http.Request) {
	value, file, err := h.app.AttachmentContent(r.Context(), r.PathValue("id"))
	if err != nil {
		result(w, 0, nil, err)
		return
	}
	defer file.Close()
	inline := value.MediaType == "image/png" || value.MediaType == "image/jpeg" ||
		value.MediaType == "image/webp" || value.MediaType == "application/pdf" ||
		strings.HasPrefix(value.MediaType, "text/plain")
	servePrivateFile(w, r, file, value.Name, value.MediaType, inline)
}

func (h *handler) attachmentPage(w http.ResponseWriter, r *http.Request) {
	page, err := strconv.Atoi(r.PathValue("page"))
	if err != nil || page < 1 {
		result(w, 0, nil, domain.ErrNotFound)
		return
	}
	value, err := h.app.AttachmentPage(r.Context(), r.PathValue("id"), page)
	result(w, http.StatusOK, value, err)
}

func (h *handler) attachmentAsset(w http.ResponseWriter, r *http.Request) {
	value, file, err := h.app.AttachmentAsset(r.Context(), r.PathValue("id"), r.PathValue("asset"))
	if err != nil {
		result(w, 0, nil, err)
		return
	}
	defer file.Close()
	servePrivateFile(w, r, file, value.Name, value.MediaType, true)
}

func servePrivateFile(w http.ResponseWriter, r *http.Request, content io.ReadSeeker, name, mediaType string, inline bool) {
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Content-Disposition", attachments.ContentDisposition(name, inline))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	http.ServeContent(w, r, name, time.Time{}, content)
}

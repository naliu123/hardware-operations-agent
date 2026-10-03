package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"hwops/internal/application"
	"hwops/internal/domain"
	"hwops/internal/knowledge"
)

type handler struct {
	app *application.App
}

func New(app *application.App, token string) http.Handler {
	h := &handler{app: app}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		reply(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /v1/knowledge/revisions", h.createRevision)
	mux.HandleFunc("POST /v1/knowledge/search", h.search)
	mux.HandleFunc("GET /v1/knowledge/revisions/{id}", h.revision)
	mux.HandleFunc("GET /v1/knowledge/revisions/{id}/fragments/{fragment}", h.fragment)
	mux.HandleFunc("POST /v1/knowledge/revisions/{id}/publication", h.publish)
	mux.HandleFunc("POST /v1/knowledge/version-policies", h.createVersionPolicy)
	mux.HandleFunc("GET /v1/knowledge/version-policies/{id}", h.versionPolicy)
	mux.HandleFunc("POST /v1/conversations", h.conversation)
	mux.HandleFunc("POST /v1/conversations/{id}/messages", h.message)
	mux.HandleFunc("GET /v1/responses/{id}", h.response)
	mux.HandleFunc("GET /v1/responses/{id}/events", h.events)
	mux.HandleFunc("PUT /v1/devices/{id}", h.putDevice)
	mux.HandleFunc("POST /v1/devices/{id}/observations", h.observe)
	mux.HandleFunc("GET /v1/devices/resolve", h.resolveDevice)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" && (token == "" || subtle.ConstantTimeCompare(
			[]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1) {
			reply(w, http.StatusUnauthorized, domain.Failure{Code: "UNAUTHORIZED", Message: "缺少有效访问令牌。"})
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func decode(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		reply(w, http.StatusBadRequest, domain.Failure{Code: "INVALID_INPUT", Message: "请求应为有效 JSON，且仅含支持字段。"})
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		reply(w, http.StatusBadRequest, domain.Failure{Code: "INVALID_INPUT", Message: "仅允许一个 JSON 对象。"})
		return false
	}
	return true
}

func result(w http.ResponseWriter, status int, value any, err error) {
	if err == nil {
		reply(w, status, value)
		return
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		reply(w, http.StatusNotFound, domain.Failure{Code: "NOT_FOUND", Message: "记录不存在。"})
	case errors.Is(err, domain.ErrInvalid):
		reply(w, http.StatusBadRequest, domain.Failure{Code: "INVALID_INPUT", Message: err.Error()})
	case errors.Is(err, domain.ErrConflict):
		reply(w, http.StatusConflict, domain.Failure{Code: "CONFLICT", Message: err.Error()})
	default:
		reply(w, http.StatusInternalServerError, domain.Failure{Code: "INTERNAL_ERROR", Message: "处理失败。"})
	}
}

func (h *handler) createRevision(w http.ResponseWriter, r *http.Request) {
	var input domain.RevisionInput
	if !decode(w, r, &input) {
		return
	}
	revision, err := h.app.CreateRevision(r.Context(), input)
	result(w, http.StatusCreated, revision, err)
}

func (h *handler) search(w http.ResponseWriter, r *http.Request) {
	var input knowledge.SearchInput
	if !decode(w, r, &input) {
		return
	}
	found, err := h.app.Search(r.Context(), input)
	result(w, http.StatusOK, found, err)
}

func (h *handler) createVersionPolicy(w http.ResponseWriter, r *http.Request) {
	var input domain.VersionPolicyInput
	if !decode(w, r, &input) {
		return
	}
	policy, err := h.app.CreateVersionPolicy(r.Context(), input)
	result(w, http.StatusCreated, policy, err)
}

func (h *handler) versionPolicy(w http.ResponseWriter, r *http.Request) {
	policy, err := h.app.VersionPolicy(r.Context(), r.PathValue("id"))
	result(w, http.StatusOK, policy, err)
}

func (h *handler) revision(w http.ResponseWriter, r *http.Request) {
	revision, err := h.app.Revision(r.Context(), r.PathValue("id"))
	result(w, http.StatusOK, revision, err)
}

func (h *handler) fragment(w http.ResponseWriter, r *http.Request) {
	revision, err := h.app.Revision(r.Context(), r.PathValue("id"))
	if err != nil {
		result(w, 0, nil, err)
		return
	}
	for _, fragment := range revision.Fragments {
		if fragment.ID == r.PathValue("fragment") {
			reply(w, http.StatusOK, struct {
				domain.Fragment
				DocumentID        string               `json:"document_id"`
				Title             string               `json:"title"`
				Source            string               `json:"source"`
				DocumentContext   string               `json:"document_context,omitempty"`
				PublicationStatus string               `json:"publication_status"`
				Applicability     domain.Applicability `json:"applicability"`
			}{fragment, revision.DocumentID, revision.Title, revision.Source, knowledge.DocumentContext(revision), revision.Status, revision.Applicability})
			return
		}
	}
	result(w, 0, nil, domain.ErrNotFound)
}

func (h *handler) publish(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Decision string `json:"decision"`
	}
	if !decode(w, r, &input) {
		return
	}
	revision, err := h.app.Publish(r.Context(), r.PathValue("id"), input.Decision)
	result(w, http.StatusOK, revision, err)
}

func (h *handler) conversation(w http.ResponseWriter, r *http.Request) {
	var input struct{}
	if !decode(w, r, &input) {
		return
	}
	conversation, err := h.app.CreateConversation(r.Context(), "local-operator")
	result(w, http.StatusCreated, conversation, err)
}

func (h *handler) message(w http.ResponseWriter, r *http.Request) {
	var input domain.MessageInput
	if !decode(w, r, &input) {
		return
	}
	response, err := h.app.Submit(r.Context(), r.PathValue("id"), input, r.Header.Get("Idempotency-Key"))
	result(w, http.StatusAccepted, response, err)
}

func (h *handler) putDevice(w http.ResponseWriter, r *http.Request) {
	var input domain.DeviceInput
	if !decode(w, r, &input) {
		return
	}
	device, err := h.app.PutDevice(r.Context(), r.PathValue("id"), input)
	result(w, http.StatusOK, device, err)
}

func (h *handler) resolveDevice(w http.ResponseWriter, r *http.Request) {
	resolution, err := h.app.ResolveDevice(r.Context(), r.URL.Query().Get("q"))
	result(w, http.StatusOK, resolution, err)
}

func (h *handler) response(w http.ResponseWriter, r *http.Request) {
	response, err := h.app.Response(r.Context(), r.PathValue("id"))
	result(w, http.StatusOK, response, err)
}

func (h *handler) observe(w http.ResponseWriter, r *http.Request) {
	var input domain.ObservationRequest
	if !decode(w, r, &input) {
		return
	}
	observation, err := h.app.Observe(r.Context(), r.PathValue("id"), input)
	result(w, http.StatusOK, observation, err)
}

package httpapi

import (
	"crypto/subtle"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hwops/internal/application"
	"hwops/internal/domain"
	"hwops/internal/identity"
	"hwops/internal/observability"
)

const sessionCookie = "__Host-hwops"

type UsersConfig struct {
	Origin              string
	StaticDir           string
	TracePrivateContent bool
}

func NewUsers(app *application.App, accounts *identity.Service, config UsersConfig) (http.Handler, error) {
	u, err := url.Parse(config.Origin)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" ||
		(u.Scheme != "https" && !(u.Scheme == "http" && u.Hostname() == "localhost")) {
		return nil, errors.New("users mode requires HWOPS_PUBLIC_ORIGIN as an HTTPS origin (http://localhost allowed for development)")
	}
	if accounts == nil {
		return nil, errors.New("users mode requires identity")
	}
	h := &handler{app: app, identity: accounts, origin: config.Origin}
	mux := h.routes()
	mux.HandleFunc("POST /v1/auth/login", h.login)
	mux.HandleFunc("POST /v1/auth/logout", h.logout)
	mux.HandleFunc("GET /v1/auth/me", h.me)
	mux.HandleFunc("GET /v1/admin/users", h.listUsers)
	mux.HandleFunc("POST /v1/admin/users", h.createUser)
	mux.HandleFunc("PATCH /v1/admin/users/{id}", h.updateUser)
	mux.HandleFunc("POST /v1/admin/users/{id}/password-reset", h.resetPassword)
	if config.StaticDir != "" {
		if _, err = os.Stat(filepath.Join(config.StaticDir, "index.html")); err != nil {
			return nil, errors.New("frontend build is missing; run npm ci && npm run build in web")
		}
		files := http.FileServer(http.Dir(config.StaticDir))
		mux.Handle("GET /assets/", files)
		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, filepath.Join(config.StaticDir, "index.html"))
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' blob: data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if !strings.HasPrefix(r.URL.Path, "/v1/") {
			mux.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		if !config.TracePrivateContent {
			r = r.WithContext(observability.WithoutContent(r.Context()))
		}
		// No shared bearer-token bypass exists in this deployment.
		if r.Header.Get("Authorization") != "" {
			result(w, 0, nil, domain.ErrUnauthorized)
			return
		}
		mutates := r.Method != http.MethodGet && r.Method != http.MethodHead
		if mutates && r.Header.Get("Origin") != h.origin {
			result(w, 0, nil, domain.ErrForbidden)
			return
		}
		if r.URL.Path == "/v1/auth/login" && r.Method == http.MethodPost {
			mux.ServeHTTP(w, r)
			return
		}
		session, err := h.authenticate(r)
		if err != nil {
			result(w, 0, nil, err)
			return
		}
		if mutates && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(session.CSRFToken)) != 1 {
			result(w, 0, nil, domain.ErrForbidden)
			return
		}
		r = r.WithContext(domain.WithUser(r.Context(), session.User))
		if strings.HasPrefix(r.URL.Path, "/v1/incidents") || strings.HasPrefix(r.URL.Path, "/v1/runs") ||
			strings.HasPrefix(r.URL.Path, "/v1/alert-events") {
			result(w, 0, nil, domain.ErrNotFound)
			return
		}
		managed := strings.HasPrefix(r.URL.Path, "/v1/admin/") ||
			(mutates && (strings.HasPrefix(r.URL.Path, "/v1/devices/") ||
				(strings.HasPrefix(r.URL.Path, "/v1/knowledge/") && r.URL.Path != "/v1/knowledge/search")))
		if managed && session.User.Role != "ADMIN" {
			result(w, 0, nil, domain.ErrForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	}), nil
}

func (h *handler) authenticate(r *http.Request) (identity.Session, error) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return identity.Session{}, domain.ErrUnauthorized
	}
	return h.identity.Authenticate(r.Context(), cookie.Value)
}

func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Username) > 64 || len(in.Password) > 72 {
		result(w, 0, nil, domain.ErrUnauthorized)
		return
	}
	// Trust the socket peer, never arbitrary X-Forwarded-For headers.
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	session, err := h.identity.Login(r.Context(), in.Username, in.Password, host)
	if err != nil {
		result(w, 0, nil, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: session.Token, Path: "/", Secure: true,
		HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: int(identity.SessionLifetime / time.Second)})
	reply(w, http.StatusOK, session)
}

func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	cookie, _ := r.Cookie(sessionCookie)
	err := h.identity.Logout(r.Context(), cookie.Value)
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", Secure: true, HttpOnly: true,
		SameSite: http.SameSiteStrictMode, MaxAge: -1})
	result(w, http.StatusOK, map[string]bool{"logged_out": true}, err)
}

func (h *handler) me(w http.ResponseWriter, r *http.Request) {
	session, err := h.authenticate(r)
	result(w, http.StatusOK, session, err)
}

func (h *handler) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.identity.List(r.Context(), domain.Owner(r.Context()), r.URL.Query().Get("after"))
	result(w, http.StatusOK, map[string]any{"users": users}, err)
}

func (h *handler) createUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if !decode(w, r, &in) {
		return
	}
	u, err := h.identity.CreateUser(r.Context(), domain.Owner(r.Context()), in.Username, in.Password, in.Role, false)
	result(w, http.StatusCreated, u, err)
}

func (h *handler) updateUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Active *bool `json:"active"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Active == nil {
		result(w, 0, nil, domain.ErrInvalid)
		return
	}
	u, err := h.identity.Update(r.Context(), domain.Owner(r.Context()), r.PathValue("id"), in.Active, nil)
	result(w, http.StatusOK, u, err)
}

func (h *handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	u, err := h.identity.Update(r.Context(), domain.Owner(r.Context()), r.PathValue("id"), nil, &in.Password)
	result(w, http.StatusOK, u, err)
}

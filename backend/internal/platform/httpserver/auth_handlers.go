package httpserver

import (
	"log"
	"net/http"
	"time"
)

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
}

type userResponse struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Role     string `json:"role"`
	Status   string `json:"status"`
}

func (h *handlers) registerAuth(mux *http.ServeMux) {
	mux.Handle("POST /api/v1/auth/register", loginRateLimit(h.deps.Redis)(http.HandlerFunc(h.register)))
	mux.HandleFunc("POST /api/v1/auth/verify-email", h.verifyEmail)
	mux.Handle("POST /api/v1/auth/login", loginRateLimit(h.deps.Redis)(http.HandlerFunc(h.login)))
	mux.Handle("POST /api/v1/auth/logout", h.auth()(http.HandlerFunc(h.logout)))
	mux.Handle("GET /api/v1/auth/me", h.auth()(http.HandlerFunc(h.me)))
	mux.Handle("POST /api/v1/auth/password/reset-request", loginRateLimit(h.deps.Redis)(http.HandlerFunc(h.requestPasswordReset)))
	mux.HandleFunc("POST /api/v1/auth/password/reset-confirm", h.confirmPasswordReset)
}

func (h *handlers) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	u, err := h.deps.Auth.RegisterStudent(r.Context(), req.Email, req.Password, req.FullName)
	if err != nil {
		if u == nil {
			writeError(w, err)
			return
		}
		log.Printf("registro creado pero con advertencia: %v", err)
	}
	writeJSON(w, http.StatusCreated, userResponse{ID: u.ID.String(), Email: u.Email, FullName: u.FullName, Role: string(u.Role), Status: string(u.Status)})
}

type tokenRequest struct {
	Token string `json:"token"`
}

func (h *handlers) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var req tokenRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.deps.Auth.VerifyEmail(r.Context(), req.Token); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "verified"})
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *handlers) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	token, u, err := h.deps.Auth.Login(r.Context(), req.Email, req.Password, r.UserAgent(), clientIP(r))
	if err != nil {
		writeError(w, err)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.deps.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(30 * 24 * time.Hour),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user":  userResponse{ID: u.ID.String(), Email: u.Email, FullName: u.FullName, Role: string(u.Role), Status: string(u.Status)},
	})
}

func (h *handlers) logout(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r)
	if err := h.deps.Auth.Logout(r.Context(), token); err != nil {
		writeError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookieName, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (h *handlers) me(w http.ResponseWriter, r *http.Request) {
	u, _ := UserFromContext(r.Context())
	writeJSON(w, http.StatusOK, userResponse{ID: u.ID.String(), Email: u.Email, FullName: u.FullName, Role: string(u.Role), Status: string(u.Status)})
}

type emailRequest struct {
	Email string `json:"email"`
}

func (h *handlers) requestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var req emailRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	// Siempre responde 202: evita enumeración de cuentas.
	_ = h.deps.Auth.RequestPasswordReset(r.Context(), req.Email)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "if_exists_email_sent"})
}

type resetConfirmRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

func (h *handlers) confirmPasswordReset(w http.ResponseWriter, r *http.Request) {
	var req resetConfirmRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.deps.Auth.ConfirmPasswordReset(r.Context(), req.Token, req.NewPassword); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "password_updated"})
}

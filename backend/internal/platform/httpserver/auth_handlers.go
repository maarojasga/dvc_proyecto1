package httpserver

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
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
	mux.Handle("POST /api/v1/auth/resend-verification", loginRateLimit(h.deps.Redis)(http.HandlerFunc(h.resendVerification)))
	mux.Handle("POST /api/v1/auth/login", loginRateLimit(h.deps.Redis)(http.HandlerFunc(h.login)))
	mux.Handle("POST /api/v1/auth/logout", h.auth()(http.HandlerFunc(h.logout)))
	mux.Handle("GET /api/v1/auth/me", h.auth()(http.HandlerFunc(h.me)))
	mux.Handle("GET /api/v1/auth/sessions", h.auth()(http.HandlerFunc(h.listSessions)))
	mux.Handle("DELETE /api/v1/auth/sessions", h.auth()(http.HandlerFunc(h.revokeOtherSessions)))
	mux.Handle("DELETE /api/v1/auth/sessions/{sessionId}", h.auth()(http.HandlerFunc(h.revokeSession)))
	mux.Handle("POST /api/v1/auth/password/reset-request", loginRateLimit(h.deps.Redis)(http.HandlerFunc(h.requestPasswordReset)))
	mux.HandleFunc("POST /api/v1/auth/password/reset-confirm", h.confirmPasswordReset)
}

// register da de alta un estudiante.
//
// Responde 202 siempre que los datos sean válidos, también cuando el correo
// ya estaba registrado y sin devolver el recurso creado: cualquiera de las
// dos cosas convertiría el formulario en un oráculo de qué cuentas existen.
func (h *handlers) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.deps.Auth.RegisterStudent(r.Context(), req.Email, req.Password, req.FullName); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, messageResponse{
		Message: "Si el correo está disponible, enviamos un enlace de verificación.",
	})
}

type messageResponse struct {
	Message string `json:"message"`
}

// resendVerification reenvía el enlace de verificación. Responde igual exista
// o no la cuenta.
func (h *handlers) resendVerification(w http.ResponseWriter, r *http.Request) {
	var req emailRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.deps.Auth.ResendVerification(r.Context(), req.Email); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, messageResponse{
		Message: "Si la cuenta existe y sigue pendiente, enviamos el enlace.",
	})
}

type sessionResponse struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
	IPAddress string `json:"ip_address,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
	Current   bool   `json:"current"`
}

// listSessions muestra las sesiones activas para poder revocarlas.
func (h *handlers) listSessions(w http.ResponseWriter, r *http.Request) {
	u, _ := UserFromContext(r.Context())
	current, _ := SessionFromContext(r.Context())

	sessions, err := h.deps.Auth.ListSessions(r.Context(), u)
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]sessionResponse, 0, len(sessions))
	for _, s := range sessions {
		items = append(items, sessionResponse{
			ID:        s.ID.String(),
			CreatedAt: s.CreatedAt.Format(time.RFC3339),
			ExpiresAt: s.ExpiresAt.Format(time.RFC3339),
			IPAddress: s.IPAddress,
			UserAgent: s.UserAgent,
			Current:   current != nil && s.ID == current.ID,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// revokeSession cierra una sesión concreta del propio usuario. Revocar la
// actual equivale a cerrar sesión, así que también se limpia la cookie.
func (h *handlers) revokeSession(w http.ResponseWriter, r *http.Request) {
	u, _ := UserFromContext(r.Context())
	current, _ := SessionFromContext(r.Context())

	sessionID, err := uuid.Parse(r.PathValue("sessionId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	if err := h.deps.Auth.RevokeSession(r.Context(), u, sessionID); err != nil {
		writeError(w, err)
		return
	}
	if current != nil && sessionID == current.ID {
		h.clearSessionCookie(w)
		h.clearCSRFCookie(w)
	}
	w.WriteHeader(http.StatusNoContent)
}

// revokeOtherSessions cierra todas las sesiones salvo la actual.
func (h *handlers) revokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	u, _ := UserFromContext(r.Context())
	current, _ := SessionFromContext(r.Context())
	if current == nil {
		writeError(w, ErrUnauthenticated)
		return
	}
	revoked, err := h.deps.Auth.RevokeOtherSessions(r.Context(), u, current)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"revoked": revoked})
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

	expires := time.Now().Add(user.DefaultSessionTTL)
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.deps.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
	})
	if err := h.issueCSRFCookie(w, expires); err != nil {
		writeError(w, err)
		return
	}

	// El token de sesión no viaja en el cuerpo: solo vive en la cookie
	// httpOnly. Devolverlo dejaría que cualquier script de la página lo
	// leyera, que es justo lo que httpOnly evita.
	writeJSON(w, http.StatusOK, map[string]any{
		"user":       userResponse{ID: u.ID.String(), Email: u.Email, FullName: u.FullName, Role: string(u.Role), Status: string(u.Status)},
		"expires_at": expires.UTC().Format(time.RFC3339),
	})
}

func (h *handlers) logout(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r)
	if err := h.deps.Auth.Logout(r.Context(), token); err != nil {
		writeError(w, err)
		return
	}
	h.clearSessionCookie(w)
	h.clearCSRFCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

// clearSessionCookie invalida la cookie en el navegador. Repite Secure y
// SameSite del alta: un borrado con atributos distintos puede no reemplazar
// la cookie original y dejarla viva en el cliente.
func (h *handlers) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.deps.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
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

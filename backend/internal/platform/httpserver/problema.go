// Package httpserver expone el dominio mediante HTTP/REST (routing,
// middlewares de auth, rate limiting, CSRF) siguiendo /api/v1.
package httpserver

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
)

// Problema es el error uniforme de la API, segun RFC 9457 (problem details).
type Problema struct {
	Type     string              `json:"type"`
	Title    string              `json:"title"`
	Status   int                 `json:"status"`
	Detail   string              `json:"detail,omitempty"`
	Instance string              `json:"instance,omitempty"`
	Errors   map[string][]string `json:"errors,omitempty"`
}

// TipoProblemaBase es el prefijo de los URI que identifican cada tipo de error.
const TipoProblemaBase = "https://mooc.local/problemas/"

func responderProblema(w http.ResponseWriter, r *http.Request, p Problema) {
	if p.Type == "" {
		p.Type = TipoProblemaBase + "error"
	}
	if p.Instance == "" {
		p.Instance = r.URL.Path
	}
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

// responderJSON escribe una respuesta correcta.
func responderJSON(w http.ResponseWriter, estado int, cuerpo any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(estado)
	if cuerpo != nil {
		_ = json.NewEncoder(w).Encode(cuerpo)
	}
}

// problemaDeDominio traduce un error de dominio al problem details que le
// corresponde.
//
// Los errores inesperados se responden como 500 sin detalle: el mensaje
// interno va al log, no al cliente.
func problemaDeDominio(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	switch {
	case errors.Is(err, user.ErrCorreoInvalido):
		responderProblema(w, r, Problema{
			Type: TipoProblemaBase + "validacion", Status: http.StatusUnprocessableEntity,
			Title: "Los datos enviados no son validos",
			Errors: map[string][]string{
				"correo": {"El correo no tiene un formato valido."},
			},
		})
	case errors.Is(err, user.ErrNombreInvalido):
		responderProblema(w, r, Problema{
			Type: TipoProblemaBase + "validacion", Status: http.StatusUnprocessableEntity,
			Title: "Los datos enviados no son validos",
			Errors: map[string][]string{
				"nombre": {"El nombre es obligatorio."},
			},
		})
	case errors.Is(err, user.ErrClaveInsegura):
		responderProblema(w, r, Problema{
			Type: TipoProblemaBase + "validacion", Status: http.StatusUnprocessableEntity,
			Title: "Los datos enviados no son validos",
			Errors: map[string][]string{
				"clave": {"La clave debe tener entre 10 y 128 caracteres."},
			},
		})
	case errors.Is(err, user.ErrCredenciales):
		responderProblema(w, r, Problema{
			Type: TipoProblemaBase + "credenciales", Status: http.StatusUnauthorized,
			Title:  "Correo o clave incorrectos",
			Detail: "Revisa los datos e intentalo de nuevo.",
		})
	case errors.Is(err, user.ErrCorreoNoVerificado):
		responderProblema(w, r, Problema{
			Type: TipoProblemaBase + "correo-no-verificado", Status: http.StatusForbidden,
			Title:  "La cuenta aun no verifica su correo",
			Detail: "Busca el mensaje de verificacion o pide que se reenvie.",
		})
	case errors.Is(err, user.ErrCuentaSuspendida):
		responderProblema(w, r, Problema{
			Type: TipoProblemaBase + "cuenta-suspendida", Status: http.StatusForbidden,
			Title: "La cuenta esta suspendida",
		})
	case errors.Is(err, user.ErrTokenInvalido):
		responderProblema(w, r, Problema{
			Type: TipoProblemaBase + "token-invalido", Status: http.StatusBadRequest,
			Title:  "El enlace no es valido o ya fue utilizado",
			Detail: "Solicita uno nuevo.",
		})
	case errors.Is(err, user.ErrSesionInvalida):
		responderNoAutenticado(w, r)
	case errors.Is(err, user.ErrNoEncontrado):
		responderProblema(w, r, Problema{
			Type: TipoProblemaBase + "no-encontrado", Status: http.StatusNotFound,
			Title: "No encontrado",
		})
	default:
		log.ErrorContext(r.Context(), "error no controlado",
			"error", err, "ruta", r.URL.Path, "peticion", IDPeticion(r.Context()))
		responderProblema(w, r, Problema{
			Type: TipoProblemaBase + "interno", Status: http.StatusInternalServerError,
			Title: "Error interno del servidor",
		})
	}
}

func responderNoAutenticado(w http.ResponseWriter, r *http.Request) {
	responderProblema(w, r, Problema{
		Type: TipoProblemaBase + "no-autenticado", Status: http.StatusUnauthorized,
		Title: "Se requiere una sesion iniciada",
	})
}

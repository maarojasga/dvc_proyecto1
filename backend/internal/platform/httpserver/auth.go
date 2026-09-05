package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
)

// tamMaximoCuerpo acota el cuerpo de las peticiones de identidad. Ninguna
// necesita mas, y el limite evita que un cliente agote memoria.
const tamMaximoCuerpo = 16 << 10 // 16 KiB

// leerJSON decodifica el cuerpo con limite de tamano y rechazando campos
// desconocidos, para que un typo en el cliente no pase inadvertido.
func leerJSON(w http.ResponseWriter, r *http.Request, destino any) error {
	r.Body = http.MaxBytesReader(w, r.Body, tamMaximoCuerpo)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(destino); err != nil {
		return err
	}
	// Un segundo objeto JSON en el mismo cuerpo es una peticion malformada.
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("el cuerpo debe contener un unico objeto JSON")
	}
	return nil
}

func responderCuerpoInvalido(w http.ResponseWriter, r *http.Request) {
	responderProblema(w, r, Problema{
		Type: TipoProblemaBase + "cuerpo-invalido", Status: http.StatusBadRequest,
		Title: "El cuerpo de la peticion no es un JSON valido",
	})
}

func origenDe(r *http.Request) user.Origen {
	return user.Origen{IP: ipCliente(r), UserAgent: r.UserAgent()}
}

type peticionRegistro struct {
	Correo string `json:"correo"`
	Nombre string `json:"nombre"`
	Clave  string `json:"clave"`
}

// registro da de alta un estudiante.
//
// Responde 202 siempre que los datos sean validos, incluso si el correo ya
// estaba registrado: el formulario no debe servir para averiguar que cuentas
// existen.
func (s *Servidor) registro(w http.ResponseWriter, r *http.Request) {
	var in peticionRegistro
	if err := leerJSON(w, r, &in); err != nil {
		responderCuerpoInvalido(w, r)
		return
	}
	err := s.usuarios.Registrar(r.Context(), user.EntradaRegistro{
		Correo: in.Correo, Nombre: in.Nombre, Clave: in.Clave,
	}, origenDe(r))
	if err != nil {
		problemaDeDominio(w, r, s.log, err)
		return
	}
	responderJSON(w, http.StatusAccepted, map[string]string{
		"mensaje": "Si el correo esta disponible, enviamos un enlace de verificacion.",
	})
}

type peticionToken struct {
	Token string `json:"token"`
}

// verificarCorreo consume el enlace de verificacion y activa la cuenta.
func (s *Servidor) verificarCorreo(w http.ResponseWriter, r *http.Request) {
	var in peticionToken
	if err := leerJSON(w, r, &in); err != nil {
		responderCuerpoInvalido(w, r)
		return
	}
	if err := s.usuarios.VerificarCorreo(r.Context(), in.Token, origenDe(r)); err != nil {
		problemaDeDominio(w, r, s.log, err)
		return
	}
	responderJSON(w, http.StatusOK, map[string]string{
		"mensaje": "Cuenta verificada. Ya puedes iniciar sesion.",
	})
}

type peticionCorreo struct {
	Correo string `json:"correo"`
}

// reenviarVerificacion vuelve a enviar el enlace de verificacion.
func (s *Servidor) reenviarVerificacion(w http.ResponseWriter, r *http.Request) {
	var in peticionCorreo
	if err := leerJSON(w, r, &in); err != nil {
		responderCuerpoInvalido(w, r)
		return
	}
	if err := s.usuarios.ReenviarVerificacion(r.Context(), in.Correo, origenDe(r)); err != nil {
		problemaDeDominio(w, r, s.log, err)
		return
	}
	responderJSON(w, http.StatusAccepted, map[string]string{
		"mensaje": "Si la cuenta existe y esta pendiente, enviamos el enlace.",
	})
}

type peticionLogin struct {
	Correo string `json:"correo"`
	Clave  string `json:"clave"`
}

// respuestaUsuario es la vista publica del usuario autenticado.
type respuestaUsuario struct {
	ID     string   `json:"id"`
	Correo string   `json:"correo"`
	Nombre string   `json:"nombre"`
	Estado string   `json:"estado"`
	Roles  []string `json:"roles"`
}

func vistaUsuario(u *user.Usuario) respuestaUsuario {
	roles := make([]string, 0, len(u.Roles))
	for _, r := range u.Roles {
		roles = append(roles, string(r))
	}
	return respuestaUsuario{
		ID: u.ID.String(), Correo: u.Correo, Nombre: u.Nombre,
		Estado: string(u.Estado), Roles: roles,
	}
}

// login abre una sesion y emite las cookies de sesion y anti-CSRF.
//
// El token de sesion solo viaja en la cookie httpOnly: no se devuelve en el
// cuerpo, para que ningun script de la pagina pueda leerlo.
func (s *Servidor) login(w http.ResponseWriter, r *http.Request) {
	var in peticionLogin
	if err := leerJSON(w, r, &in); err != nil {
		responderCuerpoInvalido(w, r)
		return
	}
	ses, token, u, err := s.usuarios.IniciarSesion(r.Context(), user.EntradaLogin{
		Correo: in.Correo, Clave: in.Clave,
	}, origenDe(r))
	if err != nil {
		problemaDeDominio(w, r, s.log, err)
		return
	}

	csrf, _, err := user.NuevoToken()
	if err != nil {
		problemaDeDominio(w, r, s.log, err)
		return
	}
	s.fijarCookieSesion(w, token, ses.ExpiraEn)
	s.fijarCookieCSRF(w, csrf, ses.ExpiraEn)

	responderJSON(w, http.StatusOK, map[string]any{
		"usuario":  vistaUsuario(u),
		"expiraEn": ses.ExpiraEn.UTC().Format(time.RFC3339),
	})
}

// logout revoca la sesion en curso y limpia las cookies.
func (s *Servidor) logout(w http.ResponseWriter, r *http.Request) {
	a, _ := autenticacionDe(r.Context())
	if err := s.usuarios.CerrarSesion(r.Context(), a, origenDe(r)); err != nil {
		problemaDeDominio(w, r, s.log, err)
		return
	}
	s.borrarCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

// sesionActual devuelve el usuario de la sesion en curso.
func (s *Servidor) sesionActual(w http.ResponseWriter, r *http.Request) {
	a, _ := autenticacionDe(r.Context())
	u, err := s.usuarios.Perfil(r.Context(), a.UsuarioID)
	if err != nil {
		problemaDeDominio(w, r, s.log, err)
		return
	}
	responderJSON(w, http.StatusOK, map[string]any{
		"usuario":  vistaUsuario(u),
		"sesionId": a.SesionID.String(),
	})
}

type respuestaSesion struct {
	ID          string `json:"id"`
	CreadaEn    string `json:"creadaEn"`
	UltimoUsoEn string `json:"ultimoUsoEn"`
	ExpiraEn    string `json:"expiraEn"`
	IP          string `json:"ip,omitempty"`
	UserAgent   string `json:"userAgent,omitempty"`
	Actual      bool   `json:"actual"`
}

// listarSesiones muestra las sesiones activas para poder revocarlas.
func (s *Servidor) listarSesiones(w http.ResponseWriter, r *http.Request) {
	a, _ := autenticacionDe(r.Context())
	sesiones, err := s.usuarios.ListarSesiones(r.Context(), a)
	if err != nil {
		problemaDeDominio(w, r, s.log, err)
		return
	}
	items := make([]respuestaSesion, 0, len(sesiones))
	for _, ses := range sesiones {
		items = append(items, respuestaSesion{
			ID:          ses.ID.String(),
			CreadaEn:    ses.CreadaEn.UTC().Format(time.RFC3339),
			UltimoUsoEn: ses.UltimoUsoEn.UTC().Format(time.RFC3339),
			ExpiraEn:    ses.ExpiraEn.UTC().Format(time.RFC3339),
			IP:          ses.IP,
			UserAgent:   ses.UserAgent,
			Actual:      ses.ID == a.SesionID,
		})
	}
	// Coleccion paginada por cursor: aqui cabe entera, asi que el cursor
	// siguiente es nulo.
	responderJSON(w, http.StatusOK, map[string]any{
		"items":           items,
		"siguienteCursor": nil,
	})
}

// revocarSesion cierra una sesion concreta del propio usuario.
func (s *Servidor) revocarSesion(w http.ResponseWriter, r *http.Request) {
	a, _ := autenticacionDe(r.Context())
	sesionID, err := uuid.Parse(r.PathValue("sesionID"))
	if err != nil {
		responderProblema(w, r, Problema{
			Type: TipoProblemaBase + "validacion", Status: http.StatusBadRequest,
			Title: "El identificador de sesion no es valido",
		})
		return
	}
	if err := s.usuarios.RevocarSesion(r.Context(), a, sesionID, origenDe(r)); err != nil {
		problemaDeDominio(w, r, s.log, err)
		return
	}
	// Revocar la propia sesion equivale a cerrarla.
	if sesionID == a.SesionID {
		s.borrarCookies(w)
	}
	w.WriteHeader(http.StatusNoContent)
}

// revocarOtrasSesiones cierra todas las sesiones salvo la actual.
func (s *Servidor) revocarOtrasSesiones(w http.ResponseWriter, r *http.Request) {
	a, _ := autenticacionDe(r.Context())
	revocadas, err := s.usuarios.RevocarOtrasSesiones(r.Context(), a, origenDe(r))
	if err != nil {
		problemaDeDominio(w, r, s.log, err)
		return
	}
	responderJSON(w, http.StatusOK, map[string]int{"revocadas": revocadas})
}

// solicitarRecuperacion envia el enlace de recuperacion de clave.
func (s *Servidor) solicitarRecuperacion(w http.ResponseWriter, r *http.Request) {
	var in peticionCorreo
	if err := leerJSON(w, r, &in); err != nil {
		responderCuerpoInvalido(w, r)
		return
	}
	if err := s.usuarios.SolicitarRecuperacion(r.Context(), in.Correo, origenDe(r)); err != nil {
		problemaDeDominio(w, r, s.log, err)
		return
	}
	responderJSON(w, http.StatusAccepted, map[string]string{
		"mensaje": "Si el correo corresponde a una cuenta, enviamos el enlace.",
	})
}

type peticionConfirmarRecuperacion struct {
	Token string `json:"token"`
	Clave string `json:"clave"`
}

// confirmarRecuperacion fija la nueva clave y cierra todas las sesiones.
func (s *Servidor) confirmarRecuperacion(w http.ResponseWriter, r *http.Request) {
	var in peticionConfirmarRecuperacion
	if err := leerJSON(w, r, &in); err != nil {
		responderCuerpoInvalido(w, r)
		return
	}
	if err := s.usuarios.ConfirmarRecuperacion(r.Context(), in.Token, in.Clave, origenDe(r)); err != nil {
		problemaDeDominio(w, r, s.log, err)
		return
	}
	s.borrarCookies(w)
	responderJSON(w, http.StatusOK, map[string]string{
		"mensaje": "Clave actualizada. Inicia sesion de nuevo.",
	})
}

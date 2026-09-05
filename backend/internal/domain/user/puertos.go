package user

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Los puertos describen lo que el dominio necesita del exterior. Los
// adaptadores viven en internal/platform y son los unicos que conocen
// PostgreSQL, Redis, SMTP o el framework HTTP.

// RepositorioUsuarios persiste cuentas.
type RepositorioUsuarios interface {
	// Crear devuelve ErrCorreoRegistrado si el correo ya existe.
	Crear(ctx context.Context, u *Usuario) error
	PorID(ctx context.Context, id uuid.UUID) (*Usuario, error)
	// PorCorreo devuelve ErrNoEncontrado si no hay cuenta con ese correo.
	PorCorreo(ctx context.Context, correo string) (*Usuario, error)
	Activar(ctx context.Context, id uuid.UUID, verificadoEn time.Time) error
	CambiarClave(ctx context.Context, id uuid.UUID, claveHash string, ahora time.Time) error
}

// RepositorioSesiones persiste sesiones revocables.
type RepositorioSesiones interface {
	Crear(ctx context.Context, s Sesion, tokenHash []byte) error
	// PorTokenHashConUsuario resuelve la cookie de sesion en una sola
	// consulta: devuelve la sesion y el titular con sus roles.
	PorTokenHashConUsuario(ctx context.Context, tokenHash []byte) (Sesion, *Usuario, error)
	ListarActivas(ctx context.Context, usuarioID uuid.UUID, ahora time.Time) ([]Sesion, error)
	Revocar(ctx context.Context, id uuid.UUID, ahora time.Time) error
	// RevocarTodas revoca las sesiones activas del usuario. Si excepto no es
	// nil, esa sesion sobrevive. Devuelve los identificadores revocados para
	// que el llamador pueda purgarlos de la cache.
	RevocarTodas(ctx context.Context, usuarioID uuid.UUID, excepto *uuid.UUID, ahora time.Time) ([]uuid.UUID, error)
	TocarUltimoUso(ctx context.Context, id uuid.UUID, ahora time.Time) error
}

// RepositorioTokens persiste los tokens de un solo uso.
type RepositorioTokens interface {
	Crear(ctx context.Context, t TokenUnicoUso) error
	PorHash(ctx context.Context, proposito Proposito, hash []byte) (TokenUnicoUso, error)
	MarcarUsado(ctx context.Context, id uuid.UUID, ahora time.Time) error
	// InvalidarPendientes marca como usados los tokens vigentes de ese
	// proposito, para que emitir uno nuevo anule los anteriores.
	InvalidarPendientes(ctx context.Context, usuarioID uuid.UUID, proposito Proposito, ahora time.Time) error
}

// Evento es una entrada de la bitacora inmutable.
type Evento struct {
	ActorID   *uuid.UUID
	Accion    string
	Entidad   string
	EntidadID string
	IP        string
	UserAgent string
	Datos     map[string]any
}

// Auditor escribe la bitacora. Registrar nunca debe abortar la operacion
// auditada: los adaptadores registran el fallo y continuan.
type Auditor interface {
	Registrar(ctx context.Context, e Evento)
}

// Hasher deriva y verifica hashes de clave.
type Hasher interface {
	Hash(clave string) (string, error)
	// Verificar compara en tiempo constante. Devuelve false sin error cuando
	// la clave no coincide; el error se reserva para hashes ilegibles.
	Verificar(clave, hash string) (bool, error)
}

// Notificador entrega los correos transaccionales.
type Notificador interface {
	EnviarVerificacionCorreo(ctx context.Context, destino, nombre, token string) error
	EnviarRecuperacionClave(ctx context.Context, destino, nombre, token string) error
}

// SesionCacheada es la instantanea de autorizacion que se guarda en Redis
// para no consultar PostgreSQL en cada peticion.
type SesionCacheada struct {
	SesionID  uuid.UUID
	UsuarioID uuid.UUID
	Estado    Estado
	Roles     []Rol
	ExpiraEn  time.Time
}

// CacheSesiones acelera la resolucion de la cookie de sesion.
//
// Es una cache, no la fuente de verdad: PostgreSQL manda. Revocar y suspender
// borran la entrada, y como Redis es compartido por todas las instancias el
// efecto es inmediato en el cluster completo. Si la cache falla, el servicio
// recurre a la base y el comportamiento sigue siendo correcto.
type CacheSesiones interface {
	Guardar(ctx context.Context, tokenHash []byte, s SesionCacheada)
	Buscar(ctx context.Context, tokenHash []byte) (SesionCacheada, bool)
	// OlvidarSesiones purga por identificador de sesion, que es lo que
	// conoce quien revoca (el token en claro no se guarda en ninguna parte).
	OlvidarSesiones(ctx context.Context, sesionIDs []uuid.UUID)
	OlvidarUsuario(ctx context.Context, usuarioID uuid.UUID)
}

// Reloj permite fijar el tiempo en las pruebas.
type Reloj func() time.Time

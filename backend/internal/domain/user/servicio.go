package user

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Origen describe de donde viene una peticion. Se guarda en la auditoria.
type Origen struct {
	IP        string
	UserAgent string
}

// Autenticacion es lo que el middleware necesita para autorizar: quien es y
// que puede hacer. No incluye datos de perfil.
type Autenticacion struct {
	SesionID  uuid.UUID
	UsuarioID uuid.UUID
	Estado    Estado
	Roles     []Rol
}

// TieneRol informa si la sesion autenticada tiene el rol indicado.
func (a Autenticacion) TieneRol(rol Rol) bool {
	for _, r := range a.Roles {
		if r == rol {
			return true
		}
	}
	return false
}

// Servicio implementa los casos de uso de identidad.
type Servicio struct {
	usuarios  RepositorioUsuarios
	sesiones  RepositorioSesiones
	tokens    RepositorioTokens
	auditoria Auditor
	claves    Hasher
	correos   Notificador
	cache     CacheSesiones
	ahora     Reloj

	// hashSenuelo iguala el costo de un login con correo inexistente al de
	// uno con correo real, para que el tiempo de respuesta no delate que
	// cuentas existen.
	hashSenuelo string
}

// Dependencias agrupa los puertos que necesita el servicio.
type Dependencias struct {
	Usuarios  RepositorioUsuarios
	Sesiones  RepositorioSesiones
	Tokens    RepositorioTokens
	Auditoria Auditor
	Claves    Hasher
	Correos   Notificador
	Cache     CacheSesiones
	Ahora     Reloj
}

// NuevoServicio construye el servicio. Cache y Ahora son opcionales.
func NuevoServicio(d Dependencias) (*Servicio, error) {
	if d.Usuarios == nil || d.Sesiones == nil || d.Tokens == nil {
		return nil, errors.New("user: faltan repositorios obligatorios")
	}
	if d.Claves == nil || d.Correos == nil || d.Auditoria == nil {
		return nil, errors.New("user: faltan hasher, notificador o auditor")
	}
	if d.Cache == nil {
		d.Cache = cacheNula{}
	}
	if d.Ahora == nil {
		d.Ahora = time.Now
	}
	s := &Servicio{
		usuarios: d.Usuarios, sesiones: d.Sesiones, tokens: d.Tokens,
		auditoria: d.Auditoria, claves: d.Claves, correos: d.Correos,
		cache: d.Cache, ahora: d.Ahora,
	}
	senuelo, _, err := NuevoToken()
	if err != nil {
		return nil, err
	}
	if s.hashSenuelo, err = d.Claves.Hash(senuelo); err != nil {
		return nil, err
	}
	return s, nil
}

// EntradaRegistro son los datos del registro publico.
type EntradaRegistro struct {
	Correo string
	Nombre string
	Clave  string
}

// Registrar crea una cuenta de estudiante pendiente de verificacion y envia el
// correo con el enlace.
//
// El registro publico solo produce estudiantes: los profesores y los
// administradores se crean por administracion. Si el correo ya existe, la
// operacion no falla ni lo revela; se audita y se responde igual que un alta
// exitosa, para no convertir el formulario en un oraculo de cuentas.
func (s *Servicio) Registrar(ctx context.Context, in EntradaRegistro, o Origen) error {
	correo := NormalizarCorreo(in.Correo)
	if err := ValidarCorreo(correo); err != nil {
		return err
	}
	if err := ValidarNombre(in.Nombre); err != nil {
		return err
	}
	if err := ValidarClave(in.Clave); err != nil {
		return err
	}

	hash, err := s.claves.Hash(in.Clave)
	if err != nil {
		return err
	}

	ahora := s.ahora()
	u := &Usuario{
		ID:            uuid.New(),
		Correo:        correo,
		Nombre:        in.Nombre,
		ClaveHash:     hash,
		Estado:        EstadoPendiente,
		Roles:         []Rol{RolEstudiante},
		CreadoEn:      ahora,
		ActualizadoEn: ahora,
	}

	switch err := s.usuarios.Crear(ctx, u); {
	case errors.Is(err, ErrCorreoRegistrado):
		s.auditoria.Registrar(ctx, Evento{
			Accion: "registro.correo_duplicado", Entidad: "usuario",
			IP: o.IP, UserAgent: o.UserAgent,
			Datos: map[string]any{"correo": correo},
		})
		// Si la cuenta existente sigue pendiente, se reenvia la verificacion.
		// Sin esto, un fallo al enviar el primer correo dejaria la cuenta
		// bloqueada para siempre: reintentar el alta caeria una y otra vez en
		// esta rama y nunca volveria a emitirse el enlace. La respuesta al
		// cliente no cambia, asi que sigue sin revelar si el correo existia.
		return s.ReenviarVerificacion(ctx, correo, o)
	case err != nil:
		return err
	}

	s.auditoria.Registrar(ctx, Evento{
		ActorID: &u.ID, Accion: "registro.creado", Entidad: "usuario",
		EntidadID: u.ID.String(), IP: o.IP, UserAgent: o.UserAgent,
		Datos: map[string]any{"correo": correo, "rol": string(RolEstudiante)},
	})

	return s.emitirVerificacion(ctx, u)
}

// emitirVerificacion invalida los tokens de verificacion pendientes, emite uno
// nuevo y lo envia por correo.
func (s *Servicio) emitirVerificacion(ctx context.Context, u *Usuario) error {
	claro, err := s.emitirToken(ctx, u, PropositoVerificacionCorreo)
	if err != nil {
		return err
	}
	return s.correos.EnviarVerificacionCorreo(ctx, u.Correo, u.NombreVisible(), claro)
}

func (s *Servicio) emitirToken(ctx context.Context, u *Usuario, p Proposito) (string, error) {
	ahora := s.ahora()
	if err := s.tokens.InvalidarPendientes(ctx, u.ID, p, ahora); err != nil {
		return "", err
	}
	claro, hash, err := NuevoToken()
	if err != nil {
		return "", err
	}
	t := TokenUnicoUso{
		ID: uuid.New(), UsuarioID: u.ID, Proposito: p, Hash: hash,
		CreadoEn: ahora, ExpiraEn: ahora.Add(p.Vigencia()),
	}
	if err := s.tokens.Crear(ctx, t); err != nil {
		return "", err
	}
	return claro, nil
}

// ReenviarVerificacion vuelve a enviar el enlace de verificacion. Responde sin
// error aunque el correo no exista o la cuenta ya este activa.
func (s *Servicio) ReenviarVerificacion(ctx context.Context, correo string, o Origen) error {
	correo = NormalizarCorreo(correo)
	u, err := s.usuarios.PorCorreo(ctx, correo)
	switch {
	case errors.Is(err, ErrNoEncontrado):
		return nil
	case err != nil:
		return err
	case u.Estado != EstadoPendiente:
		s.auditoria.Registrar(ctx, Evento{
			ActorID: &u.ID, Accion: "verificacion.reenvio_innecesario",
			Entidad: "usuario", EntidadID: u.ID.String(),
			IP: o.IP, UserAgent: o.UserAgent,
		})
		return nil
	}
	return s.emitirVerificacion(ctx, u)
}

// VerificarCorreo consume el token del enlace y activa la cuenta.
func (s *Servicio) VerificarCorreo(ctx context.Context, tokenPlano string, o Origen) error {
	ahora := s.ahora()
	t, err := s.tokenUtilizable(ctx, PropositoVerificacionCorreo, tokenPlano, ahora)
	if err != nil {
		return err
	}
	if err := s.tokens.MarcarUsado(ctx, t.ID, ahora); err != nil {
		return err
	}
	if err := s.usuarios.Activar(ctx, t.UsuarioID, ahora); err != nil {
		return err
	}
	s.auditoria.Registrar(ctx, Evento{
		ActorID: &t.UsuarioID, Accion: "verificacion.confirmada", Entidad: "usuario",
		EntidadID: t.UsuarioID.String(), IP: o.IP, UserAgent: o.UserAgent,
	})
	return nil
}

func (s *Servicio) tokenUtilizable(ctx context.Context, p Proposito, claro string, ahora time.Time) (TokenUnicoUso, error) {
	if claro == "" {
		return TokenUnicoUso{}, ErrTokenInvalido
	}
	t, err := s.tokens.PorHash(ctx, p, HashToken(claro))
	switch {
	case errors.Is(err, ErrNoEncontrado):
		return TokenUnicoUso{}, ErrTokenInvalido
	case err != nil:
		return TokenUnicoUso{}, err
	case !t.Utilizable(ahora):
		return TokenUnicoUso{}, ErrTokenInvalido
	}
	return t, nil
}

// EntradaLogin son las credenciales del inicio de sesion.
type EntradaLogin struct {
	Correo string
	Clave  string
}

// IniciarSesion valida las credenciales y abre una sesion revocable. Devuelve
// el token en claro, que el llamador debe entregar solo como cookie.
func (s *Servicio) IniciarSesion(ctx context.Context, in EntradaLogin, o Origen) (Sesion, string, *Usuario, error) {
	correo := NormalizarCorreo(in.Correo)
	u, err := s.usuarios.PorCorreo(ctx, correo)
	if errors.Is(err, ErrNoEncontrado) {
		// Se verifica igual contra un hash senuelo para que el tiempo de
		// respuesta no distinga un correo inexistente de una clave errada.
		_, _ = s.claves.Verificar(in.Clave, s.hashSenuelo)
		s.auditarLoginFallido(ctx, nil, correo, "correo_inexistente", o)
		return Sesion{}, "", nil, ErrCredenciales
	}
	if err != nil {
		return Sesion{}, "", nil, err
	}

	coincide, err := s.claves.Verificar(in.Clave, u.ClaveHash)
	if err != nil {
		return Sesion{}, "", nil, err
	}
	if !coincide {
		s.auditarLoginFallido(ctx, &u.ID, correo, "clave_incorrecta", o)
		return Sesion{}, "", nil, ErrCredenciales
	}
	if err := u.PuedeIniciarSesion(); err != nil {
		s.auditarLoginFallido(ctx, &u.ID, correo, string(u.Estado), o)
		return Sesion{}, "", nil, err
	}

	ahora := s.ahora()
	claro, hash, err := NuevoToken()
	if err != nil {
		return Sesion{}, "", nil, err
	}
	ses := Sesion{
		ID: uuid.New(), UsuarioID: u.ID,
		CreadaEn: ahora, ExpiraEn: ahora.Add(VigenciaSesion), UltimoUsoEn: ahora,
		IP: o.IP, UserAgent: o.UserAgent,
	}
	if err := s.sesiones.Crear(ctx, ses, hash); err != nil {
		return Sesion{}, "", nil, err
	}
	s.cache.Guardar(ctx, hash, SesionCacheada{
		SesionID: ses.ID, UsuarioID: u.ID, Estado: u.Estado,
		Roles: u.Roles, ExpiraEn: ses.ExpiraEn,
	})
	s.auditoria.Registrar(ctx, Evento{
		ActorID: &u.ID, Accion: "sesion.iniciada", Entidad: "sesion",
		EntidadID: ses.ID.String(), IP: o.IP, UserAgent: o.UserAgent,
	})
	return ses, claro, u, nil
}

func (s *Servicio) auditarLoginFallido(ctx context.Context, actor *uuid.UUID, correo, motivo string, o Origen) {
	s.auditoria.Registrar(ctx, Evento{
		ActorID: actor, Accion: "sesion.intento_fallido", Entidad: "usuario",
		IP: o.IP, UserAgent: o.UserAgent,
		Datos: map[string]any{"correo": correo, "motivo": motivo},
	})
}

// Autenticar resuelve la cookie de sesion. Consulta primero la cache y cae a
// PostgreSQL, que es la fuente de verdad.
func (s *Servicio) Autenticar(ctx context.Context, tokenPlano string) (Autenticacion, error) {
	if tokenPlano == "" {
		return Autenticacion{}, ErrSesionInvalida
	}
	ahora := s.ahora()
	hash := HashToken(tokenPlano)

	if c, ok := s.cache.Buscar(ctx, hash); ok {
		if ahora.Before(c.ExpiraEn) && c.Estado == EstadoActivo {
			return Autenticacion{
				SesionID: c.SesionID, UsuarioID: c.UsuarioID,
				Estado: c.Estado, Roles: c.Roles,
			}, nil
		}
		s.cache.OlvidarSesiones(ctx, []uuid.UUID{c.SesionID})
	}

	ses, u, err := s.sesiones.PorTokenHashConUsuario(ctx, hash)
	switch {
	case errors.Is(err, ErrNoEncontrado):
		return Autenticacion{}, ErrSesionInvalida
	case err != nil:
		return Autenticacion{}, err
	case !ses.Activa(ahora):
		return Autenticacion{}, ErrSesionInvalida
	}
	if err := u.PuedeIniciarSesion(); err != nil {
		return Autenticacion{}, ErrSesionInvalida
	}

	s.cache.Guardar(ctx, hash, SesionCacheada{
		SesionID: ses.ID, UsuarioID: u.ID, Estado: u.Estado,
		Roles: u.Roles, ExpiraEn: ses.ExpiraEn,
	})
	if err := s.sesiones.TocarUltimoUso(ctx, ses.ID, ahora); err != nil {
		return Autenticacion{}, err
	}
	return Autenticacion{
		SesionID: ses.ID, UsuarioID: u.ID, Estado: u.Estado, Roles: u.Roles,
	}, nil
}

// Perfil devuelve los datos del usuario autenticado.
func (s *Servicio) Perfil(ctx context.Context, usuarioID uuid.UUID) (*Usuario, error) {
	return s.usuarios.PorID(ctx, usuarioID)
}

// CerrarSesion revoca la sesion en curso.
func (s *Servicio) CerrarSesion(ctx context.Context, a Autenticacion, o Origen) error {
	return s.revocar(ctx, a.UsuarioID, a.SesionID, "sesion.cerrada", o)
}

// RevocarSesion revoca una sesion del propio usuario.
//
// Una sesion ajena responde igual que una inexistente: no se confirma que el
// identificador exista para otra cuenta.
func (s *Servicio) RevocarSesion(ctx context.Context, a Autenticacion, sesionID uuid.UUID, o Origen) error {
	activas, err := s.sesiones.ListarActivas(ctx, a.UsuarioID, s.ahora())
	if err != nil {
		return err
	}
	for _, ses := range activas {
		if ses.ID == sesionID {
			return s.revocar(ctx, a.UsuarioID, sesionID, "sesion.revocada", o)
		}
	}
	return ErrNoEncontrado
}

func (s *Servicio) revocar(ctx context.Context, usuarioID, sesionID uuid.UUID, accion string, o Origen) error {
	if err := s.sesiones.Revocar(ctx, sesionID, s.ahora()); err != nil {
		return err
	}
	s.cache.OlvidarSesiones(ctx, []uuid.UUID{sesionID})
	s.auditoria.Registrar(ctx, Evento{
		ActorID: &usuarioID, Accion: accion, Entidad: "sesion",
		EntidadID: sesionID.String(), IP: o.IP, UserAgent: o.UserAgent,
	})
	return nil
}

// RevocarOtrasSesiones cierra todas las sesiones del usuario salvo la actual.
func (s *Servicio) RevocarOtrasSesiones(ctx context.Context, a Autenticacion, o Origen) (int, error) {
	revocadas, err := s.sesiones.RevocarTodas(ctx, a.UsuarioID, &a.SesionID, s.ahora())
	if err != nil {
		return 0, err
	}
	s.cache.OlvidarSesiones(ctx, revocadas)
	s.auditoria.Registrar(ctx, Evento{
		ActorID: &a.UsuarioID, Accion: "sesion.revocadas_otras", Entidad: "usuario",
		EntidadID: a.UsuarioID.String(), IP: o.IP, UserAgent: o.UserAgent,
		Datos: map[string]any{"revocadas": len(revocadas)},
	})
	return len(revocadas), nil
}

// ListarSesiones devuelve las sesiones activas del usuario.
func (s *Servicio) ListarSesiones(ctx context.Context, a Autenticacion) ([]Sesion, error) {
	return s.sesiones.ListarActivas(ctx, a.UsuarioID, s.ahora())
}

// SolicitarRecuperacion envia el enlace de recuperacion. No revela si el
// correo existe: responde sin error en todos los casos.
func (s *Servicio) SolicitarRecuperacion(ctx context.Context, correo string, o Origen) error {
	correo = NormalizarCorreo(correo)
	u, err := s.usuarios.PorCorreo(ctx, correo)
	switch {
	case errors.Is(err, ErrNoEncontrado):
		s.auditoria.Registrar(ctx, Evento{
			Accion: "recuperacion.solicitada_inexistente", Entidad: "usuario",
			IP: o.IP, UserAgent: o.UserAgent,
			Datos: map[string]any{"correo": correo},
		})
		return nil
	case err != nil:
		return err
	case u.Estado == EstadoSuspendido:
		return nil
	}

	claro, err := s.emitirToken(ctx, u, PropositoRecuperacionClave)
	if err != nil {
		return err
	}
	s.auditoria.Registrar(ctx, Evento{
		ActorID: &u.ID, Accion: "recuperacion.solicitada", Entidad: "usuario",
		EntidadID: u.ID.String(), IP: o.IP, UserAgent: o.UserAgent,
	})
	return s.correos.EnviarRecuperacionClave(ctx, u.Correo, u.NombreVisible(), claro)
}

// ConfirmarRecuperacion fija la nueva clave y cierra todas las sesiones del
// usuario, porque quien pide recuperar puede estar expulsando a un intruso.
func (s *Servicio) ConfirmarRecuperacion(ctx context.Context, tokenPlano, nuevaClave string, o Origen) error {
	if err := ValidarClave(nuevaClave); err != nil {
		return err
	}
	ahora := s.ahora()
	t, err := s.tokenUtilizable(ctx, PropositoRecuperacionClave, tokenPlano, ahora)
	if err != nil {
		return err
	}
	hash, err := s.claves.Hash(nuevaClave)
	if err != nil {
		return err
	}
	if err := s.tokens.MarcarUsado(ctx, t.ID, ahora); err != nil {
		return err
	}
	if err := s.usuarios.CambiarClave(ctx, t.UsuarioID, hash, ahora); err != nil {
		return err
	}
	revocadas, err := s.sesiones.RevocarTodas(ctx, t.UsuarioID, nil, ahora)
	if err != nil {
		return err
	}
	s.cache.OlvidarSesiones(ctx, revocadas)
	s.cache.OlvidarUsuario(ctx, t.UsuarioID)
	s.auditoria.Registrar(ctx, Evento{
		ActorID: &t.UsuarioID, Accion: "recuperacion.confirmada", Entidad: "usuario",
		EntidadID: t.UsuarioID.String(), IP: o.IP, UserAgent: o.UserAgent,
		Datos: map[string]any{"sesiones_revocadas": len(revocadas)},
	})
	return nil
}

// cacheNula es la implementacion por defecto cuando no hay Redis configurado.
type cacheNula struct{}

func (cacheNula) Guardar(context.Context, []byte, SesionCacheada) {}
func (cacheNula) Buscar(context.Context, []byte) (SesionCacheada, bool) {
	return SesionCacheada{}, false
}
func (cacheNula) OlvidarSesiones(context.Context, []uuid.UUID) {}
func (cacheNula) OlvidarUsuario(context.Context, uuid.UUID)    {}

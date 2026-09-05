package redisclient

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
)

// CacheSesiones guarda en Redis la instantanea de autorizacion de cada sesion
// para no consultar PostgreSQL en cada peticion. Implementa
// user.CacheSesiones.
//
// Mantiene tres claves por sesion:
//
//	sesion:tok:<hash>  instantanea, con TTL igual a la vigencia de la sesion
//	sesion:id:<id>     hash del token, para poder purgar sabiendo solo el id
//	sesion:usr:<id>    conjunto de sesiones del usuario, para purgarlas juntas
//
// Como Redis es compartido por todas las instancias de la API, borrar aqui
// surte efecto inmediato en el cluster completo: eso es lo que hace que
// revocar una sesion sea instantaneo y no dependa de esperar un TTL.
type CacheSesiones struct {
	c   *redis.Client
	log *slog.Logger
}

// NuevaCacheSesiones construye la cache.
func NuevaCacheSesiones(c *redis.Client, log *slog.Logger) *CacheSesiones {
	return &CacheSesiones{c: c, log: log}
}

type instantanea struct {
	SesionID  uuid.UUID `json:"sesion_id"`
	UsuarioID uuid.UUID `json:"usuario_id"`
	Estado    string    `json:"estado"`
	Roles     []string  `json:"roles"`
	ExpiraEn  time.Time `json:"expira_en"`
}

func claveToken(hash []byte) string    { return "sesion:tok:" + hex.EncodeToString(hash) }
func claveSesion(id uuid.UUID) string  { return "sesion:id:" + id.String() }
func claveUsuario(id uuid.UUID) string { return "sesion:usr:" + id.String() }

// Guardar escribe la instantanea con TTL igual a lo que le quede a la sesion.
func (s *CacheSesiones) Guardar(ctx context.Context, tokenHash []byte, ses user.SesionCacheada) {
	ttl := time.Until(ses.ExpiraEn)
	if ttl <= 0 {
		return
	}
	roles := make([]string, 0, len(ses.Roles))
	for _, r := range ses.Roles {
		roles = append(roles, string(r))
	}
	crudo, err := json.Marshal(instantanea{
		SesionID: ses.SesionID, UsuarioID: ses.UsuarioID,
		Estado: string(ses.Estado), Roles: roles, ExpiraEn: ses.ExpiraEn,
	})
	if err != nil {
		s.log.Error("cache de sesiones: instantanea no serializable", "error", err)
		return
	}

	hashHex := hex.EncodeToString(tokenHash)
	tuberia := s.c.TxPipeline()
	tuberia.Set(ctx, claveToken(tokenHash), crudo, ttl)
	tuberia.Set(ctx, claveSesion(ses.SesionID), hashHex, ttl)
	tuberia.SAdd(ctx, claveUsuario(ses.UsuarioID), ses.SesionID.String())
	tuberia.Expire(ctx, claveUsuario(ses.UsuarioID), ttl)
	if _, err := tuberia.Exec(ctx); err != nil {
		s.log.Warn("cache de sesiones: no se pudo guardar", "error", err)
	}
}

// Buscar devuelve la instantanea si sigue en cache.
func (s *CacheSesiones) Buscar(ctx context.Context, tokenHash []byte) (user.SesionCacheada, bool) {
	crudo, err := s.c.Get(ctx, claveToken(tokenHash)).Bytes()
	if err != nil {
		if err != redis.Nil {
			s.log.Warn("cache de sesiones: lectura fallida", "error", err)
		}
		return user.SesionCacheada{}, false
	}
	var i instantanea
	if err := json.Unmarshal(crudo, &i); err != nil {
		return user.SesionCacheada{}, false
	}
	roles := make([]user.Rol, 0, len(i.Roles))
	for _, r := range i.Roles {
		roles = append(roles, user.Rol(r))
	}
	return user.SesionCacheada{
		SesionID: i.SesionID, UsuarioID: i.UsuarioID,
		Estado: user.Estado(i.Estado), Roles: roles, ExpiraEn: i.ExpiraEn,
	}, true
}

// OlvidarSesiones purga las sesiones indicadas.
func (s *CacheSesiones) OlvidarSesiones(ctx context.Context, ids []uuid.UUID) {
	if len(ids) == 0 {
		return
	}
	ctx = context.WithoutCancel(ctx)
	claves := make([]string, 0, len(ids))
	for _, id := range ids {
		claves = append(claves, claveSesion(id))
	}
	hashes, err := s.c.MGet(ctx, claves...).Result()
	if err != nil {
		s.log.Warn("cache de sesiones: no se pudo purgar", "error", err)
		return
	}
	aBorrar := make([]string, 0, len(ids)*2)
	aBorrar = append(aBorrar, claves...)
	for _, h := range hashes {
		hashHex, ok := h.(string)
		if !ok {
			continue
		}
		aBorrar = append(aBorrar, "sesion:tok:"+hashHex)
	}
	if err := s.c.Del(ctx, aBorrar...).Err(); err != nil {
		s.log.Warn("cache de sesiones: no se pudo borrar", "error", err)
	}
}

// OlvidarUsuario purga todas las sesiones cacheadas del usuario. Se usa al
// suspender una cuenta o cambiar su clave.
func (s *CacheSesiones) OlvidarUsuario(ctx context.Context, usuarioID uuid.UUID) {
	ctx = context.WithoutCancel(ctx)
	miembros, err := s.c.SMembers(ctx, claveUsuario(usuarioID)).Result()
	if err != nil {
		s.log.Warn("cache de sesiones: no se pudo listar el usuario", "error", err)
		return
	}
	ids := make([]uuid.UUID, 0, len(miembros))
	for _, m := range miembros {
		if id, err := uuid.Parse(m); err == nil {
			ids = append(ids, id)
		}
	}
	s.OlvidarSesiones(ctx, ids)
	if err := s.c.Del(ctx, claveUsuario(usuarioID)).Err(); err != nil {
		s.log.Warn("cache de sesiones: no se pudo borrar el indice", "error", err)
	}
}

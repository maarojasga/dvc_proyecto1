package postgres

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
)

// Auditor escribe la bitacora inmutable. Implementa user.Auditor.
type Auditor struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

// NuevoAuditor construye el auditor.
func NuevoAuditor(pool *pgxpool.Pool, log *slog.Logger) *Auditor {
	return &Auditor{pool: pool, log: log}
}

// Registrar inserta el evento.
//
// No devuelve error a proposito: perder una entrada de bitacora no debe
// tumbar la operacion auditada. El fallo se reporta por el log, donde la
// alerta de observabilidad puede recogerlo.
func (a *Auditor) Registrar(ctx context.Context, e user.Evento) {
	datos := e.Datos
	if datos == nil {
		datos = map[string]any{}
	}
	crudo, err := json.Marshal(datos)
	if err != nil {
		a.log.Error("auditoria: datos no serializables", "accion", e.Accion, "error", err)
		return
	}
	// La escritura no debe cancelarse si el cliente corta la conexion: el
	// hecho auditado ya ocurrio.
	ctx = context.WithoutCancel(ctx)
	if _, err := a.pool.Exec(ctx, `
		INSERT INTO auditoria (actor_id, accion, entidad, entidad_id, ip, user_agent, datos)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		e.ActorID, e.Accion, nulo(e.Entidad), nulo(e.EntidadID), nulo(e.IP), nulo(e.UserAgent), crudo,
	); err != nil {
		a.log.Error("auditoria: no se pudo registrar el evento", "accion", e.Accion, "error", err)
	}
}

// nulo convierte la cadena vacia en NULL, para no llenar la bitacora de
// cadenas vacias indistinguibles de un dato ausente.
func nulo(s string) any {
	if s == "" {
		return nil
	}
	return s
}

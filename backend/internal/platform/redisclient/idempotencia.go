package redisclient

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// VigenciaIdempotencia es cuanto se recuerda una clave Idempotency-Key.
const VigenciaIdempotencia = 24 * time.Hour

// RespuestaGuardada es la respuesta que se repite ante una clave repetida.
type RespuestaGuardada struct {
	Estado int               `json:"estado"`
	Cuerpo []byte            `json:"cuerpo"`
	Tipo   string            `json:"tipo"`
	Extras map[string]string `json:"extras,omitempty"`
}

// ErrEnCurso indica que otra peticion con la misma clave sigue ejecutandose.
var ErrEnCurso = errors.New("idempotencia: peticion en curso")

// AlmacenIdempotencia recuerda las respuestas de las operaciones marcadas con
// Idempotency-Key, de modo que reintentar no produce efectos repetidos.
type AlmacenIdempotencia struct{ c *redis.Client }

// NuevoAlmacenIdempotencia construye el almacen.
func NuevoAlmacenIdempotencia(c *redis.Client) *AlmacenIdempotencia {
	return &AlmacenIdempotencia{c: c}
}

const marcaEnCurso = "en_curso"

// Reservar intenta apropiarse de la clave.
//
// Devuelve la respuesta previa si la operacion ya termino, ErrEnCurso si otra
// peticion identica sigue en vuelo, o (nil, nil) si esta peticion es la
// primera y debe ejecutarse.
func (a *AlmacenIdempotencia) Reservar(ctx context.Context, clave string) (*RespuestaGuardada, error) {
	clave = "idem:" + clave
	tomada, err := a.c.SetNX(ctx, clave, marcaEnCurso, VigenciaIdempotencia).Result()
	if err != nil {
		return nil, err
	}
	if tomada {
		return nil, nil
	}

	crudo, err := a.c.Get(ctx, clave).Bytes()
	if err != nil {
		return nil, err
	}
	if string(crudo) == marcaEnCurso {
		return nil, ErrEnCurso
	}
	var r RespuestaGuardada
	if err := json.Unmarshal(crudo, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// Guardar fija la respuesta definitiva de la clave.
func (a *AlmacenIdempotencia) Guardar(ctx context.Context, clave string, r RespuestaGuardada) error {
	crudo, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return a.c.Set(ctx, "idem:"+clave, crudo, VigenciaIdempotencia).Err()
}

// Liberar suelta la reserva cuando la operacion falla, para que el cliente
// pueda reintentarla con la misma clave.
func (a *AlmacenIdempotencia) Liberar(ctx context.Context, clave string) {
	_ = a.c.Del(context.WithoutCancel(ctx), "idem:"+clave).Err()
}

package httpserver

import (
	"context"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
)

type claveContexto int

const (
	claveIDPeticion claveContexto = iota
	claveAutenticacion
)

// conIDPeticion adjunta el identificador de correlacion al contexto.
func conIDPeticion(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, claveIDPeticion, id)
}

// IDPeticion devuelve el identificador de correlacion de la peticion, si lo hay.
func IDPeticion(ctx context.Context) string {
	id, _ := ctx.Value(claveIDPeticion).(string)
	return id
}

func conAutenticacion(ctx context.Context, a user.Autenticacion) context.Context {
	return context.WithValue(ctx, claveAutenticacion, a)
}

// autenticacionDe devuelve la sesion autenticada, si la peticion trae una.
func autenticacionDe(ctx context.Context) (user.Autenticacion, bool) {
	a, ok := ctx.Value(claveAutenticacion).(user.Autenticacion)
	return a, ok
}

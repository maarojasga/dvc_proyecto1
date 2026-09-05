// Package user contiene las entidades y reglas de negocio de usuarios, roles
// (administrador, profesor, estudiante) y sesiones, sin depender de HTTP,
// base de datos ni proveedor cloud.
package user

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// Rol es uno de los tres roles globales de la plataforma.
type Rol string

const (
	RolAdministrador Rol = "administrador"
	RolProfesor      Rol = "profesor"
	RolEstudiante    Rol = "estudiante"
)

// Valido informa si el rol es uno de los reconocidos por la plataforma.
func (r Rol) Valido() bool {
	switch r {
	case RolAdministrador, RolProfesor, RolEstudiante:
		return true
	}
	return false
}

// Estado es la situacion de la cuenta.
//
// Una cuenta nace pendiente y solo pasa a activa cuando se verifica el correo.
type Estado string

const (
	EstadoPendiente  Estado = "pendiente"
	EstadoActivo     Estado = "activo"
	EstadoSuspendido Estado = "suspendido"
)

// Usuario es una cuenta de la plataforma.
type Usuario struct {
	ID                 uuid.UUID
	Correo             string
	Nombre             string
	ClaveHash          string
	Estado             Estado
	Roles              []Rol
	CorreoVerificadoEn *time.Time
	CreadoEn           time.Time
	ActualizadoEn      time.Time
}

// TieneRol informa si el usuario tiene el rol indicado.
func (u *Usuario) TieneRol(rol Rol) bool {
	for _, r := range u.Roles {
		if r == rol {
			return true
		}
	}
	return false
}

// PuedeIniciarSesion aplica las reglas de estado al inicio de sesion. Devuelve
// un error de dominio describiendo el impedimento, o nil si puede entrar.
func (u *Usuario) PuedeIniciarSesion() error {
	switch u.Estado {
	case EstadoActivo:
		return nil
	case EstadoPendiente:
		return ErrCorreoNoVerificado
	case EstadoSuspendido:
		return ErrCuentaSuspendida
	default:
		return ErrCuentaSuspendida
	}
}

// NombreVisible recorta el nombre y cae al correo si viene vacio.
func (u *Usuario) NombreVisible() string {
	if n := strings.TrimSpace(u.Nombre); n != "" {
		return n
	}
	return u.Correo
}

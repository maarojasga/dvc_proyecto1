package user

import (
	"time"

	"github.com/google/uuid"
)

// Vigencias del MVP.
const (
	VigenciaSesion             = 14 * 24 * time.Hour
	VigenciaVerificacionCorreo = 24 * time.Hour
	VigenciaRecuperacionClave  = time.Hour
)

// Sesion es una sesion iniciada, revocable en cualquier momento.
//
// El token en claro solo existe en la cookie del cliente: aqui se guarda su
// hash, de modo que leer la base no permite suplantar a nadie.
type Sesion struct {
	ID          uuid.UUID
	UsuarioID   uuid.UUID
	CreadaEn    time.Time
	ExpiraEn    time.Time
	UltimoUsoEn time.Time
	RevocadaEn  *time.Time
	IP          string
	UserAgent   string
}

// Activa informa si la sesion sigue siendo utilizable en el instante dado.
func (s Sesion) Activa(ahora time.Time) bool {
	return s.RevocadaEn == nil && ahora.Before(s.ExpiraEn)
}

// Proposito distingue los tokens de un solo uso.
type Proposito string

const (
	PropositoVerificacionCorreo Proposito = "verificacion_correo"
	PropositoRecuperacionClave  Proposito = "recuperacion_clave"
)

// Vigencia devuelve cuanto dura un token segun su proposito.
func (p Proposito) Vigencia() time.Duration {
	if p == PropositoRecuperacionClave {
		return VigenciaRecuperacionClave
	}
	return VigenciaVerificacionCorreo
}

// TokenUnicoUso respalda la verificacion de correo y la recuperacion de clave.
type TokenUnicoUso struct {
	ID        uuid.UUID
	UsuarioID uuid.UUID
	Proposito Proposito
	Hash      []byte
	CreadoEn  time.Time
	ExpiraEn  time.Time
	UsadoEn   *time.Time
}

// Utilizable informa si el token sigue sirviendo: ni usado ni expirado.
func (t TokenUnicoUso) Utilizable(ahora time.Time) bool {
	return t.UsadoEn == nil && ahora.Before(t.ExpiraEn)
}

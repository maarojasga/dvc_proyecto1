// Package passwords deriva y verifica hashes de clave con Argon2id.
package passwords

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Parametros de derivacion. Los valores por defecto siguen la recomendacion de
// OWASP para Argon2id (19 MiB, 2 iteraciones, paralelismo 1).
type Parametros struct {
	Memoria     uint32 // en KiB
	Iteraciones uint32
	Paralelismo uint8
	LongitudSal uint32
	LongitudRes uint32
}

// ParametrosPorDefecto es la configuracion usada en produccion.
func ParametrosPorDefecto() Parametros {
	return Parametros{
		Memoria: 19 * 1024, Iteraciones: 2, Paralelismo: 1,
		LongitudSal: 16, LongitudRes: 32,
	}
}

// ParametrosDePrueba abarata la derivacion en pruebas, donde el costo solo
// suma segundos sin aportar seguridad.
func ParametrosDePrueba() Parametros {
	return Parametros{
		Memoria: 1024, Iteraciones: 1, Paralelismo: 1,
		LongitudSal: 16, LongitudRes: 32,
	}
}

// Argon2 implementa el puerto user.Hasher.
type Argon2 struct{ p Parametros }

// Nuevo construye el hasher con los parametros dados.
func Nuevo(p Parametros) *Argon2 { return &Argon2{p: p} }

// ErrHashIlegible indica que el hash almacenado no tiene el formato esperado.
var ErrHashIlegible = errors.New("passwords: hash con formato invalido")

// Hash deriva la clave y devuelve la codificacion estandar de PHC, que lleva
// los parametros incrustados para poder recalibrarlos sin migrar la base.
func (a *Argon2) Hash(clave string) (string, error) {
	sal := make([]byte, a.p.LongitudSal)
	if _, err := rand.Read(sal); err != nil {
		return "", err
	}
	suma := argon2.IDKey([]byte(clave), sal, a.p.Iteraciones, a.p.Memoria, a.p.Paralelismo, a.p.LongitudRes)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, a.p.Memoria, a.p.Iteraciones, a.p.Paralelismo,
		base64.RawStdEncoding.EncodeToString(sal),
		base64.RawStdEncoding.EncodeToString(suma),
	), nil
}

// Verificar compara en tiempo constante usando los parametros del propio hash,
// de modo que las claves antiguas siguen validandose tras recalibrar.
func (a *Argon2) Verificar(clave, hash string) (bool, error) {
	partes := strings.Split(hash, "$")
	if len(partes) != 6 || partes[1] != "argon2id" {
		return false, ErrHashIlegible
	}

	var version int
	if _, err := fmt.Sscanf(partes[2], "v=%d", &version); err != nil {
		return false, ErrHashIlegible
	}
	if version != argon2.Version {
		return false, ErrHashIlegible
	}

	var memoria, iteraciones uint32
	var paralelismo uint8
	if _, err := fmt.Sscanf(partes[3], "m=%d,t=%d,p=%d", &memoria, &iteraciones, &paralelismo); err != nil {
		return false, ErrHashIlegible
	}

	sal, err := base64.RawStdEncoding.DecodeString(partes[4])
	if err != nil {
		return false, ErrHashIlegible
	}
	esperado, err := base64.RawStdEncoding.DecodeString(partes[5])
	if err != nil {
		return false, ErrHashIlegible
	}

	calculado := argon2.IDKey([]byte(clave), sal, iteraciones, memoria, paralelismo, uint32(len(esperado)))
	return subtle.ConstantTimeCompare(calculado, esperado) == 1, nil
}

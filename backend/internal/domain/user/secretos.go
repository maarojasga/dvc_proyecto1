package user

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
)

// LongitudTokenBytes es la entropia de los tokens de sesion y de un solo uso.
const LongitudTokenBytes = 32

// NuevoToken genera un token aleatorio y devuelve su forma en claro (la que
// viaja al usuario en la cookie o en el enlace del correo) junto con el hash
// que se persiste. El valor en claro nunca se guarda.
func NuevoToken() (claro string, hash []byte, err error) {
	crudo := make([]byte, LongitudTokenBytes)
	if _, err := rand.Read(crudo); err != nil {
		return "", nil, err
	}
	claro = base64.RawURLEncoding.EncodeToString(crudo)
	return claro, HashToken(claro), nil
}

// HashToken deriva el hash almacenable de un token en claro.
//
// SHA-256 sin sal es suficiente aqui, a diferencia de las claves: el token
// tiene 256 bits de entropia, asi que no admite fuerza bruta ni diccionario.
func HashToken(claro string) []byte {
	suma := sha256.Sum256([]byte(claro))
	return suma[:]
}

// TokensIguales compara dos hashes en tiempo constante.
func TokensIguales(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

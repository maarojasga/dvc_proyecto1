// Package security agrupa primitivas criptográficas de bajo nivel (hash de
// contraseñas, generación y hash de tokens opacos de sesión/verificación)
// usadas por la capa de aplicación.
package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	return string(b), err
}

func ComparePassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// GenerateOpaqueToken produce un token aleatorio no adivinable para
// sesiones, verificación de correo y reseteo de contraseña. Solo su hash se
// persiste; el valor en texto plano se entrega una única vez al cliente.
func GenerateOpaqueToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func HashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

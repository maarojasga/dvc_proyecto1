package user

import "errors"

// Errores de dominio. La capa HTTP los traduce a problem details; ninguno
// lleva detalles que permitan enumerar cuentas.
var (
	ErrCorreoInvalido     = errors.New("el correo no tiene un formato valido")
	ErrNombreInvalido     = errors.New("el nombre es obligatorio")
	ErrClaveInsegura      = errors.New("la clave no cumple la politica minima")
	ErrCredenciales       = errors.New("correo o clave incorrectos")
	ErrCorreoNoVerificado = errors.New("la cuenta aun no verifica su correo")
	ErrCuentaSuspendida   = errors.New("la cuenta esta suspendida")
	ErrTokenInvalido      = errors.New("el token no es valido o ya fue utilizado")
	ErrSesionInvalida     = errors.New("la sesion no es valida o expiro")
	ErrNoEncontrado       = errors.New("no encontrado")
	ErrCorreoRegistrado   = errors.New("el correo ya esta registrado")
	ErrRolNoValido        = errors.New("el rol no es valido")
)

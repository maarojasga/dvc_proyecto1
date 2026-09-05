package user

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPuedeIniciarSesionSegunEstado(t *testing.T) {
	casos := []struct {
		estado   Estado
		esperado error
	}{
		{EstadoActivo, nil},
		{EstadoPendiente, ErrCorreoNoVerificado},
		{EstadoSuspendido, ErrCuentaSuspendida},
	}
	for _, c := range casos {
		u := &Usuario{Estado: c.estado}
		if err := u.PuedeIniciarSesion(); err != c.esperado {
			t.Errorf("estado %q: se obtuvo %v, se esperaba %v", c.estado, err, c.esperado)
		}
	}
}

func TestTieneRol(t *testing.T) {
	u := &Usuario{Roles: []Rol{RolEstudiante}}
	if !u.TieneRol(RolEstudiante) {
		t.Error("el estudiante deberia tener su propio rol")
	}
	if u.TieneRol(RolProfesor) || u.TieneRol(RolAdministrador) {
		t.Error("un estudiante no deberia tener rol de profesor ni de administrador")
	}
}

func TestSesionActiva(t *testing.T) {
	ahora := time.Now()
	vigente := Sesion{ExpiraEn: ahora.Add(time.Hour)}
	if !vigente.Activa(ahora) {
		t.Error("una sesion sin revocar y sin expirar deberia estar activa")
	}

	expirada := Sesion{ExpiraEn: ahora.Add(-time.Minute)}
	if expirada.Activa(ahora) {
		t.Error("una sesion expirada no deberia estar activa")
	}

	revocada := Sesion{ExpiraEn: ahora.Add(time.Hour), RevocadaEn: &ahora}
	if revocada.Activa(ahora) {
		t.Error("una sesion revocada no deberia estar activa")
	}
}

func TestTokenUtilizable(t *testing.T) {
	ahora := time.Now()
	vigente := TokenUnicoUso{ExpiraEn: ahora.Add(time.Hour)}
	if !vigente.Utilizable(ahora) {
		t.Error("un token vigente y sin usar deberia ser utilizable")
	}
	usado := TokenUnicoUso{ExpiraEn: ahora.Add(time.Hour), UsadoEn: &ahora}
	if usado.Utilizable(ahora) {
		t.Error("un token ya usado no deberia ser utilizable")
	}
	expirado := TokenUnicoUso{ExpiraEn: ahora.Add(-time.Second)}
	if expirado.Utilizable(ahora) {
		t.Error("un token expirado no deberia ser utilizable")
	}
}

func TestVigenciasPorProposito(t *testing.T) {
	if got := PropositoVerificacionCorreo.Vigencia(); got != VigenciaVerificacionCorreo {
		t.Errorf("verificacion: %v, se esperaba %v", got, VigenciaVerificacionCorreo)
	}
	// La recuperacion de clave dura menos que la verificacion: la ventana de
	// abuso de un enlace que cambia credenciales debe ser mas corta.
	if PropositoRecuperacionClave.Vigencia() >= PropositoVerificacionCorreo.Vigencia() {
		t.Error("el token de recuperacion deberia expirar antes que el de verificacion")
	}
}

func TestNombreVisibleCaeAlCorreo(t *testing.T) {
	u := &Usuario{ID: uuid.New(), Correo: "ana@example.com", Nombre: "   "}
	if got := u.NombreVisible(); got != "ana@example.com" {
		t.Errorf("NombreVisible() = %q; se esperaba el correo", got)
	}
}

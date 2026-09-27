package iframe_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/iframe"
)

func listaDePrueba() iframe.Lista {
	return iframe.Lista{
		{Host: "www.youtube-nocookie.com", Permisos: "fullscreen; picture-in-picture"},
		{Host: "vimeo.com", IncluirSubdominios: true, Permisos: "fullscreen"},
		{Host: "docs.example.edu"},
	}
}

func TestUnHostDeLaListaSeAutorizaConSusPermisos(t *testing.T) {
	d, err := listaDePrueba().Autorizar("https://www.youtube-nocookie.com/embed/abc123")
	if err != nil {
		t.Fatalf("debería autorizarse: %v", err)
	}
	if d.Permisos != "fullscreen; picture-in-picture" {
		t.Errorf("permisos inesperados: %q", d.Permisos)
	}
}

func TestUnHostFueraDeLaListaSeRechaza(t *testing.T) {
	_, err := listaDePrueba().Autorizar("https://sitio-cualquiera.com/embed")
	if !errors.Is(err, iframe.ErrHostNoAutorizado) {
		t.Fatalf("se esperaba ErrHostNoAutorizado, llegó %v", err)
	}
}

// El caso que hace inútil una lista blanca mal escrita: un atacante registra
// un dominio que termina igual que el autorizado.
func TestUnDominioQueSoloTerminaIgualNoEsSubdominio(t *testing.T) {
	casos := []string{
		"https://malicioso-vimeo.com/x",
		"https://vimeo.com.atacante.net/x",
		"https://notvimeo.com/x",
	}
	for _, u := range casos {
		if _, err := listaDePrueba().Autorizar(u); !errors.Is(err, iframe.ErrHostNoAutorizado) {
			t.Errorf("%s no debería autorizarse, llegó %v", u, err)
		}
	}
}

func TestLosSubdominiosSoloPasanSiLaEntradaLoDeclara(t *testing.T) {
	if _, err := listaDePrueba().Autorizar("https://player.vimeo.com/video/1"); err != nil {
		t.Errorf("vimeo declara subdominios, debería autorizarse: %v", err)
	}
	// docs.example.edu no los declara.
	if _, err := listaDePrueba().Autorizar("https://otro.docs.example.edu/x"); !errors.Is(err, iframe.ErrHostNoAutorizado) {
		t.Errorf("un subdominio no declarado no debería autorizarse, llegó %v", err)
	}
}

// Un host exacto debe ganar a la regla de subdominios que también lo cubre,
// para poder darle permisos distintos.
func TestElHostExactoGanaALaReglaDeSubdominios(t *testing.T) {
	lista := iframe.Lista{
		{Host: "vimeo.com", IncluirSubdominios: true, Permisos: "fullscreen"},
		{Host: "player.vimeo.com", Permisos: "fullscreen; autoplay"},
	}
	d, err := lista.Autorizar("https://player.vimeo.com/video/1")
	if err != nil {
		t.Fatalf("debería autorizarse: %v", err)
	}
	if d.Permisos != "fullscreen; autoplay" {
		t.Errorf("ganó la entrada equivocada: %q", d.Permisos)
	}
}

func TestSoloSeAdmiteHTTPS(t *testing.T) {
	for _, u := range []string{"http://vimeo.com/x", "javascript:alert(1)", "data:text/html,<script>"} {
		_, err := listaDePrueba().Autorizar(u)
		if err == nil {
			t.Errorf("%s no debería autorizarse", u)
		}
		if errors.Is(err, iframe.ErrHostNoAutorizado) {
			t.Errorf("%s debería fallar por esquema, no por host", u)
		}
	}
}

// El sandbox es contención: comprueba que no se cuelan las concesiones que
// convertirían un embed en un secuestro de la página.
func TestElSandboxNoConcedeLoQueSecuestrariaLaPagina(t *testing.T) {
	s := iframe.Sandbox()
	for _, prohibido := range []string{"allow-top-navigation", "allow-modals", "allow-downloads", "allow-pointer-lock"} {
		if strings.Contains(s, prohibido) {
			t.Errorf("el sandbox concede %q", prohibido)
		}
	}
	for _, necesario := range []string{"allow-scripts", "allow-same-origin"} {
		if !strings.Contains(s, necesario) {
			t.Errorf("sin %q no carga ningún embed real", necesario)
		}
	}
}

func TestNormalizarHostAceptaURLYDominioSuelto(t *testing.T) {
	casos := map[string]string{
		"https://Player.Vimeo.com/video/1": "player.vimeo.com",
		"  VIMEO.com  ":                    "vimeo.com",
	}
	for entrada, esperado := range casos {
		got, err := iframe.NormalizarHost(entrada)
		if err != nil {
			t.Errorf("%q: %v", entrada, err)
			continue
		}
		if got != esperado {
			t.Errorf("%q dio %q, se esperaba %q", entrada, got, esperado)
		}
	}
}

func TestNormalizarHostRechazaBasura(t *testing.T) {
	for _, entrada := range []string{"", "localhost", "con espacio.com", "example.com/ruta", "user@example.com"} {
		if got, err := iframe.NormalizarHost(entrada); err == nil {
			t.Errorf("%q no debería aceptarse, dio %q", entrada, got)
		}
	}
}

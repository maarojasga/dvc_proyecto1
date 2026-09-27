// Package iframe decide qué contenido de terceros puede incrustarse en un
// curso y con cuántos privilegios.
//
// Un iframe ejecuta código ajeno dentro del origen de la plataforma, así que
// el control tiene tres capas y ninguna sustituye a las otras:
//
//   - La **lista blanca** decide si el destino se incrusta siquiera. Es la
//     única capa que impide que el contenido llegue al navegador.
//   - El **sandbox** recorta lo que ese contenido puede hacer una vez
//     cargado: sin él, el marco puede navegar la ventana principal, abrir
//     descargas o enviar formularios en nombre de quien mira.
//   - La **política de permisos** (`allow`) gobierna el acceso a cámara,
//     micrófono, geolocalización y demás. Su valor por defecto es no conceder
//     nada: cada host de la lista declara lo que de verdad necesita.
//
// El paquete no conoce HTTP ni SQL: recibe la lista y responde qué se permite.
package iframe

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/google/uuid"
)

var (
	// ErrURLInvalida indica un destino que ni siquiera es una URL usable.
	ErrURLInvalida = errors.New("iframe: la URL no es válida")
	// ErrEsquemaNoPermitido rechaza todo lo que no sea HTTPS.
	ErrEsquemaNoPermitido = errors.New("iframe: solo se admite https")
	// ErrHostNoAutorizado indica un destino fuera de la lista blanca.
	ErrHostNoAutorizado = errors.New("iframe: el dominio no está en la lista de destinos autorizados")
)

// Destino es una entrada de la lista blanca.
type Destino struct {
	ID uuid.UUID
	// Host es el dominio autorizado, en minúsculas y sin puerto.
	Host string
	// IncluirSubdominios extiende la autorización a los subdominios de Host.
	// Va explícito y no por omisión: autorizar "*.example.com" cuando solo se
	// quería "docs.example.com" regala cualquier subdominio que el tercero
	// cree después, incluido el que aloje contenido de usuarios.
	IncluirSubdominios bool
	// Permisos son las funciones que este destino puede pedir, en la sintaxis
	// del atributo `allow` ("fullscreen; picture-in-picture"). Vacío es lo
	// normal y significa que no se concede ninguna.
	Permisos string
	// Descripcion explica para qué se autorizó, que es lo que hace revisable
	// la lista meses después.
	Descripcion string
}

// SandboxPorDefecto son los permisos que se devuelven a todo marco incrustado.
//
// Es una lista de concesiones sobre un sandbox que, vacío, lo prohíbe todo.
// Lo que se concede es lo mínimo para que un vídeo o un visor funcionen:
//
//   - allow-scripts: sin esto casi ningún embed carga.
//   - allow-same-origin: el marco conserva su propio origen y puede usar su
//     almacenamiento y sus cookies. No le da acceso al origen de la
//     plataforma; sin él, en cambio, muchos reproductores fallan.
//   - allow-popups + allow-popups-to-escape-sandbox: abrir el contenido en
//     una pestaña nueva, que no hereda el sandbox.
//   - allow-presentation: pantalla completa y segunda pantalla.
//   - allow-forms: buscadores y controles internos del embed.
//
// Lo que deliberadamente NO se concede:
//
//   - allow-top-navigation: el marco no puede llevarse la ventana principal a
//     otro sitio. Es la concesión que convierte un embed en un secuestro de
//     navegación.
//   - allow-modals: nada de alert/confirm bloqueando la página del curso.
//   - allow-downloads: una descarga iniciada por el marco parecería venir de
//     la plataforma.
//   - allow-pointer-lock y allow-orientation-lock.
//
// allow-scripts junto a allow-same-origin permite, en teoría, que el marco se
// quite el propio atributo sandbox; por eso la lista blanca es la capa que de
// verdad decide, y el sandbox es contención, no frontera.
var SandboxPorDefecto = []string{
	"allow-scripts",
	"allow-same-origin",
	"allow-popups",
	"allow-popups-to-escape-sandbox",
	"allow-presentation",
	"allow-forms",
}

// Sandbox devuelve el valor del atributo del mismo nombre.
func Sandbox() string { return strings.Join(SandboxPorDefecto, " ") }

// ReferrerPolicy evita filtrar al tercero qué curso y qué recurso está viendo
// el estudiante: la ruta completa iría en el Referer con la política por
// defecto de muchos navegadores.
const ReferrerPolicy = "strict-origin-when-cross-origin"

// Lista es el conjunto de destinos autorizados.
type Lista []Destino

// Autorizar resuelve si una URL puede incrustarse y con qué permisos.
//
// Devuelve el destino que la autoriza para que quien llama pueda registrar
// cuál fue, que es lo que permite auditar por qué entró un contenido.
func (l Lista) Autorizar(crudo string) (*Destino, error) {
	host, err := HostDe(crudo)
	if err != nil {
		return nil, err
	}
	// Se recorre de más específico a menos: un host exacto gana a la regla de
	// subdominios, de modo que se le pueden dar permisos propios.
	candidatos := append(Lista(nil), l...)
	sort.SliceStable(candidatos, func(i, j int) bool {
		if candidatos[i].IncluirSubdominios != candidatos[j].IncluirSubdominios {
			return !candidatos[i].IncluirSubdominios
		}
		return len(candidatos[i].Host) > len(candidatos[j].Host)
	})
	for i := range candidatos {
		if candidatos[i].cubre(host) {
			d := candidatos[i]
			return &d, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrHostNoAutorizado, host)
}

// cubre decide si el destino autoriza ese host.
func (d Destino) cubre(host string) bool {
	autorizado := strings.ToLower(strings.TrimSpace(d.Host))
	if autorizado == "" {
		return false
	}
	if host == autorizado {
		return true
	}
	if !d.IncluirSubdominios {
		return false
	}
	// El punto es imprescindible: sin él "malicioso-example.com" pasaría como
	// subdominio de "example.com".
	return strings.HasSuffix(host, "."+autorizado)
}

// HostDe extrae y normaliza el host de una URL destinada a un iframe.
//
// Exige HTTPS: incrustar HTTP en una página servida por HTTPS lo bloquea el
// navegador como contenido mixto, y si no lo bloqueara sería peor, porque el
// contenido viajaría manipulable.
func HostDe(crudo string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(crudo))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrURLInvalida, err)
	}
	if u.Scheme != "https" {
		return "", ErrEsquemaNoPermitido
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", ErrURLInvalida
	}
	return host, nil
}

// NormalizarHost prepara un host para guardarlo en la lista.
//
// Acepta tanto "example.com" como una URL completa, porque quien administra
// suele tener a mano el enlace y no el dominio suelto.
func NormalizarHost(crudo string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(crudo))
	if s == "" {
		return "", ErrURLInvalida
	}
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil || u.Hostname() == "" {
			return "", ErrURLInvalida
		}
		return strings.ToLower(u.Hostname()), nil
	}
	// Un host suelto no puede traer ruta, puerto ni credenciales.
	if strings.ContainsAny(s, "/?#@: ") {
		return "", ErrURLInvalida
	}
	if !strings.Contains(s, ".") {
		return "", ErrURLInvalida
	}
	return s, nil
}

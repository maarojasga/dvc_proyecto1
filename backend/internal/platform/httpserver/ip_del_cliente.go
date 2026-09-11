package httpserver

import (
	"net"
	"net/http"
	"strings"
)

// Determinar la IP del cliente.
//
// Importa porque de ella dependen el límite de tasa y la bitácora. Si se puede
// elegir, el límite no limita —basta rotar el valor— y la bitácora registra lo
// que diga el atacante.
//
// X-Forwarded-For lo puede poner cualquiera, así que solo se cree cuando la
// petición llega de un proxy que se declaró de confianza. Detrás del proxy
// inverso del despliegue hay que declararlo, o todas las peticiones parecerían
// venir del proxy y un solo abusador agotaría el límite de todos.

// ProxiesDeConfianza son las redes desde las que se acepta X-Forwarded-For.
type ProxiesDeConfianza struct{ redes []*net.IPNet }

// NuevosProxiesDeConfianza interpreta una lista de CIDR separados por comas.
//
// Los valores que no se entienden se descartan en silencio: una entrada mal
// escrita reduce la confianza, que es el lado seguro del error. Lo contrario
// —tratarla como comodín— convertiría un dedazo en un agujero.
func NuevosProxiesDeConfianza(lista string) ProxiesDeConfianza {
	var p ProxiesDeConfianza
	for _, entrada := range strings.Split(lista, ",") {
		entrada = strings.TrimSpace(entrada)
		if entrada == "" {
			continue
		}
		// Una IP suelta vale como red de un solo host.
		if !strings.Contains(entrada, "/") {
			if ip := net.ParseIP(entrada); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				p.redes = append(p.redes, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			}
			continue
		}
		if _, red, err := net.ParseCIDR(entrada); err == nil {
			p.redes = append(p.redes, red)
		}
	}
	return p
}

// contiene dice si la IP pertenece a algún proxy de confianza.
func (p ProxiesDeConfianza) contiene(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, red := range p.redes {
		if red.Contains(ip) {
			return true
		}
	}
	return false
}

// ipDelCliente devuelve la dirección a la que atribuir la petición.
//
// Recorre X-Forwarded-For de derecha a izquierda —del proxy más cercano al más
// lejano— y se queda con la primera que no sea un proxy de confianza. Ese es
// el último salto que no controlamos, y por tanto lo más cercano al cliente
// real que se puede afirmar. Leer el primer elemento, como se hacía, es
// leer justo la parte que el cliente escribe.
func ipDelCliente(r *http.Request, confiables ProxiesDeConfianza) string {
	directa := hostDe(r.RemoteAddr)

	// Sin proxies declarados, o si quien llama no es uno de ellos, la única
	// dirección creíble es la del socket.
	if len(confiables.redes) == 0 || !confiables.contiene(net.ParseIP(directa)) {
		return directa
	}

	reenviadas := r.Header.Values("X-Forwarded-For")
	if len(reenviadas) == 0 {
		return directa
	}
	var cadena []string
	for _, cabecera := range reenviadas {
		for _, parte := range strings.Split(cabecera, ",") {
			if parte = strings.TrimSpace(parte); parte != "" {
				cadena = append(cadena, parte)
			}
		}
	}
	for i := len(cadena) - 1; i >= 0; i-- {
		candidata := net.ParseIP(hostDe(cadena[i]))
		if candidata == nil {
			// Un valor inventado corta la cadena: a partir de ahí nada es
			// creíble, y se atribuye al último salto de confianza.
			return directa
		}
		if !confiables.contiene(candidata) {
			return candidata.String()
		}
	}
	// Toda la cadena son proxies de confianza: la petición es interna.
	return directa
}

// hostDe quita el puerto si lo hay. RemoteAddr siempre lo trae, y una entrada
// de X-Forwarded-For puede traerlo.
func hostDe(dir string) string {
	if host, _, err := net.SplitHostPort(dir); err == nil {
		return host
	}
	return strings.Trim(dir, "[]")
}

package httpserver

import (
	"crypto/ed25519"
	"net/http"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/badge"
)

// EmisorDeCredenciales firma las credenciales Open Badges 3.0.
//
// Puede ser nulo: sin clave configurada la plataforma sigue emitiendo
// insignias verificables por su URL pública, y solo deja de ofrecer la
// credencial portátil.
type EmisorDeCredenciales struct {
	Privada ed25519.PrivateKey
	Publica ed25519.PublicKey
	// KeyID identifica la clave dentro del JWKS, para poder rotarla sin
	// invalidar de golpe todo lo firmado antes.
	KeyID string
	// Nombre es la organización que aparece como emisora.
	Nombre string
	// BaseURL es el origen público de la plataforma.
	BaseURL string
}

func (h *handlers) registerOpenBadges(mux *http.ServeMux) {
	// Las tres rutas son públicas: el sentido de una credencial verificable es
	// que cualquiera pueda comprobarla sin tener cuenta aquí.
	mux.Handle("GET /api/v1/badges/{code}/openbadge", http.HandlerFunc(h.credencialOpenBadge))
	mux.Handle("GET /api/v1/badges/{code}/openbadge.jwt", http.HandlerFunc(h.credencialFirmada))
	mux.Handle("GET /api/v1/.well-known/jwks.json", http.HandlerFunc(h.clavesPublicas))
}

// credencialDe arma la credencial de una insignia vigente.
func (h *handlers) credencialDe(r *http.Request) (badge.Credencial, error) {
	v, err := h.deps.Progreso.Verificar(r.Context(), r.PathValue("code"))
	if err != nil {
		return badge.Credencial{}, err
	}
	// Una insignia revocada no se entrega como credencial: firmar un documento
	// que afirma un logro retirado sería emitir algo que no se puede desdecir.
	if !v.Valid {
		return badge.Credencial{}, ErrInsigniaRevocada
	}

	datos := badge.DatosDeCredencial{
		BaseURL:            h.emisor().BaseURL,
		NombreDelEmisor:    h.emisor().Nombre,
		CodigoVerificacion: v.Code,
		EmitidaEn:          v.IssuedAt,
	}
	// El título del curso es un dato del catálogo, público como la insignia.
	if _, version, err := h.deps.Courses.GetPublishedByCourseID(r.Context(), v.CourseID); err == nil {
		datos.TituloDelCurso = version.Title
		datos.DescripcionCurso = version.Summary
	}
	if v.ImagenClave != "" && h.deps.Entrega != nil {
		if url, err := h.deps.Entrega.PresignedGetURL(r.Context(), v.ImagenClave, vigenciaEntrega, ""); err == nil {
			datos.URLDeLaImagen = url
		}
	}
	return badge.NuevaCredencial(datos), nil
}

func (h *handlers) emisor() EmisorDeCredenciales {
	return h.deps.Credenciales
}

// credencialOpenBadge entrega la credencial sin firmar, legible.
func (h *handlers) credencialOpenBadge(w http.ResponseWriter, r *http.Request) {
	c, err := h.credencialDe(r)
	if err != nil {
		writeError(w, err)
		return
	}
	// El tipo de contenido es el de JSON-LD y no el JSON genérico: es lo que
	// le dice a un verificador que el documento tiene semántica.
	w.Header().Set("Content-Type", "application/ld+json")
	writeJSONSinTipo(w, http.StatusOK, c)
}

// credencialFirmada entrega el VC-JWT, que es la forma portátil: se verifica
// con la clave pública sin llamar a esta plataforma.
func (h *handlers) credencialFirmada(w http.ResponseWriter, r *http.Request) {
	emisor := h.emisor()
	if len(emisor.Privada) == 0 {
		writeError(w, ErrFirmaNoConfigurada)
		return
	}
	c, err := h.credencialDe(r)
	if err != nil {
		writeError(w, err)
		return
	}
	jwt, err := badge.FirmarComoJWT(c, emisor.Privada, emisor.KeyID)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/vc+ld+json+jwt")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(jwt))
}

// clavesPublicas publica el JWKS con el que se verifican las credenciales.
func (h *handlers) clavesPublicas(w http.ResponseWriter, r *http.Request) {
	emisor := h.emisor()
	claves := []map[string]string{}
	if len(emisor.Publica) > 0 {
		claves = append(claves, badge.ClavePublicaJWK(emisor.Publica, emisor.KeyID))
	}
	// Se cachea: la clave cambia muy de vez en cuando y un verificador la pide
	// por cada credencial que comprueba.
	w.Header().Set("Cache-Control", "public, max-age=3600")
	writeJSON(w, http.StatusOK, map[string]any{"keys": claves})
}

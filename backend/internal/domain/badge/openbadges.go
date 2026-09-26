package badge

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// Open Badges 3.0 (alcance opcional 5.2).
//
// La versión 3.0 del estándar deja de ser una imagen con metadatos incrustados
// y pasa a ser una Credencial Verificable del W3C: un documento JSON-LD que
// cualquiera puede comprobar con la clave pública del emisor, sin llamar a la
// plataforma. Eso es lo que la hace portátil —la credencial sigue valiendo
// aunque el emisor desaparezca— y también lo que obliga a firmarla.
//
// Se emite en las dos formas que admite el estándar:
//
//   - El documento JSON-LD sin firmar, que es legible y sirve para inspección.
//   - Un VC-JWT: la credencial como carga de un JWS firmado con Ed25519. Se
//     elige esta prueba y no una Data Integrity porque las pruebas de
//     integridad exigen canonicalizar el JSON-LD (URDNA2015 o JCS), y una
//     canonicalización implementada a medias produce firmas que unos
//     verificadores aceptan y otros no, que es peor que no firmar.
//
// La revocación no viaja dentro de la credencial: una credencial firmada no
// puede cambiar de opinión. Para eso está la URL pública de verificación, que
// es la que dice si sigue vigente.

const (
	contextoVC = "https://www.w3.org/ns/credentials/v2"
	contextoOB = "https://purl.imsglobal.org/spec/ob/v3p0/context-3.0.3.json"

	tipoCredencial = "VerifiableCredential"
	tipoLogro      = "AchievementCredential"
)

// Credencial es una AchievementCredential de Open Badges 3.0.
type Credencial struct {
	Contexto         []string          `json:"@context"`
	ID               string            `json:"id"`
	Tipo             []string          `json:"type"`
	Emisor           Emisor            `json:"issuer"`
	FechaDeValidez   time.Time         `json:"validFrom"`
	Nombre           string            `json:"name"`
	SujetoCredencial SujetoCredencial  `json:"credentialSubject"`
	EstadoCredencial *EstadoCredencial `json:"credentialStatus,omitempty"`
}

// Emisor identifica a la organización que emite.
type Emisor struct {
	ID     string `json:"id"`
	Tipo   string `json:"type"`
	Nombre string `json:"name"`
}

// SujetoCredencial es a quién se le reconoce el logro.
//
// El identificador del sujeto es la propia URL de verificación y no el correo
// ni el identificador del estudiante: la credencial se comparte, y el
// enunciado pide que la URL pública no exponga datos personales. Quien tenga
// que atar la credencial a una persona lo hace por otro canal.
type SujetoCredencial struct {
	Tipo  string `json:"type"`
	ID    string `json:"id"`
	Logro Logro  `json:"achievement"`
}

// Logro describe qué acredita la insignia.
type Logro struct {
	ID          string   `json:"id"`
	Tipo        string   `json:"type"`
	Nombre      string   `json:"name"`
	Descripcion string   `json:"description"`
	Criterio    Criterio `json:"criteria"`
	Imagen      *Imagen  `json:"image,omitempty"`
}

type Criterio struct {
	Narrativa string `json:"narrative"`
}

type Imagen struct {
	ID   string `json:"id"`
	Tipo string `json:"type"`
}

// EstadoCredencial apunta a dónde se comprueba la vigencia.
type EstadoCredencial struct {
	ID   string `json:"id"`
	Tipo string `json:"type"`
}

// DatosDeCredencial es lo que hace falta para emitir.
type DatosDeCredencial struct {
	// BaseURL es el origen público de la plataforma.
	BaseURL string
	// NombreDelEmisor es la organización que opera la plataforma.
	NombreDelEmisor string
	// CodigoVerificacion identifica la insignia públicamente.
	CodigoVerificacion string
	TituloDelCurso     string
	DescripcionCurso   string
	EmitidaEn          time.Time
	// URLDeLaImagen es opcional.
	URLDeLaImagen string
}

// URLDeVerificacion es la dirección pública de una insignia.
func URLDeVerificacion(baseURL, codigo string) string {
	return fmt.Sprintf("%s/insignias/%s", baseURL, codigo)
}

// NuevaCredencial arma la credencial Open Badges 3.0 de una insignia.
func NuevaCredencial(d DatosDeCredencial) Credencial {
	verificacion := URLDeVerificacion(d.BaseURL, d.CodigoVerificacion)
	emisor := d.BaseURL + "/issuer"

	c := Credencial{
		Contexto: []string{contextoVC, contextoOB},
		ID:       verificacion,
		Tipo:     []string{tipoCredencial, tipoLogro},
		Emisor: Emisor{
			ID: emisor, Tipo: "Profile", Nombre: d.NombreDelEmisor,
		},
		FechaDeValidez: d.EmitidaEn.UTC(),
		Nombre:         d.TituloDelCurso,
		SujetoCredencial: SujetoCredencial{
			Tipo: "AchievementSubject",
			ID:   verificacion,
			Logro: Logro{
				ID:          verificacion + "#achievement",
				Tipo:        "Achievement",
				Nombre:      d.TituloDelCurso,
				Descripcion: d.DescripcionCurso,
				Criterio: Criterio{
					Narrativa: "Completar los recursos obligatorios del curso y superar sus evaluaciones.",
				},
			},
		},
		// La vigencia no va dentro de la credencial firmada: una firma no
		// puede cambiar de opinión cuando se revoca.
		EstadoCredencial: &EstadoCredencial{
			ID: verificacion, Tipo: "1EdTechRevocationList",
		},
	}
	if d.URLDeLaImagen != "" {
		c.SujetoCredencial.Logro.Imagen = &Imagen{ID: d.URLDeLaImagen, Tipo: "Image"}
	}
	return c
}

// FirmarComoJWT devuelve la credencial como VC-JWT (RFC 7519 con firma EdDSA).
//
// El identificador de la clave (kid) apunta al documento JWKS de la
// plataforma, que es lo que permite a un verificador encontrar la clave
// pública sin conocer de antemano cómo publica sus claves este emisor.
func FirmarComoJWT(c Credencial, clave ed25519.PrivateKey, kid string) (string, error) {
	if len(clave) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("badge: la clave de firma no es una clave Ed25519 válida")
	}

	cabecera := map[string]string{"alg": "EdDSA", "typ": "vc+jwt", "kid": kid}
	carga := map[string]any{
		"iss": c.Emisor.ID,
		"sub": c.SujetoCredencial.ID,
		"jti": c.ID,
		"iat": c.FechaDeValidez.Unix(),
		"nbf": c.FechaDeValidez.Unix(),
		"vc":  c,
	}

	cabeceraJSON, err := json.Marshal(cabecera)
	if err != nil {
		return "", err
	}
	cargaJSON, err := json.Marshal(carga)
	if err != nil {
		return "", err
	}

	porFirmar := base64url(cabeceraJSON) + "." + base64url(cargaJSON)
	firma := ed25519.Sign(clave, []byte(porFirmar))
	return porFirmar + "." + base64url(firma), nil
}

// ClavePublicaJWK expone la clave pública en el formato que espera un
// verificador (RFC 8037, curva Ed25519).
func ClavePublicaJWK(publica ed25519.PublicKey, kid string) map[string]string {
	return map[string]string{
		"kty": "OKP",
		"crv": "Ed25519",
		"x":   base64url(publica),
		"use": "sig",
		"alg": "EdDSA",
		"kid": kid,
	}
}

// base64url codifica sin relleno, que es como lo exige JWS.
func base64url(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

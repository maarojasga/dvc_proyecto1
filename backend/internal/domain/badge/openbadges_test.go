package badge_test

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/badge"
)

func datosDePrueba() badge.DatosDeCredencial {
	return badge.DatosDeCredencial{
		BaseURL:            "https://mooc.example.edu",
		NombreDelEmisor:    "Universidad de Ejemplo",
		CodigoVerificacion: "abc123",
		TituloDelCurso:     "Fundamentos de la nube",
		DescripcionCurso:   "Arquitecturas distribuidas y almacenamiento de objetos.",
		EmitidaEn:          time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC),
		URLDeLaImagen:      "https://cdn.example.edu/badges/abc123.svg",
	}
}

func TestLaCredencialDeclaraLosContextosYTiposDelEstandar(t *testing.T) {
	c := badge.NuevaCredencial(datosDePrueba())

	if len(c.Contexto) < 2 || c.Contexto[0] != "https://www.w3.org/ns/credentials/v2" {
		t.Errorf("el primer contexto debe ser el de credenciales verificables: %v", c.Contexto)
	}
	if !strings.Contains(c.Contexto[1], "ob/v3p0") {
		t.Errorf("falta el contexto de Open Badges 3.0: %v", c.Contexto)
	}

	tipos := strings.Join(c.Tipo, ",")
	if !strings.Contains(tipos, "VerifiableCredential") || !strings.Contains(tipos, "AchievementCredential") {
		t.Errorf("tipos inesperados: %v", c.Tipo)
	}
}

// La credencial se comparte: no puede llevar nada de la persona, igual que la
// URL pública de verificación.
func TestLaCredencialNoLlevaDatosDelEstudiante(t *testing.T) {
	c := badge.NuevaCredencial(datosDePrueba())
	crudo, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	texto := strings.ToLower(string(crudo))
	for _, prohibido := range []string{"@example.com", "student_id", "estudiante"} {
		if strings.Contains(texto, prohibido) {
			t.Errorf("la credencial contiene %q: %s", prohibido, crudo)
		}
	}
	// El sujeto se identifica por la URL de verificación.
	if c.SujetoCredencial.ID != "https://mooc.example.edu/insignias/abc123" {
		t.Errorf("identificador del sujeto inesperado: %q", c.SujetoCredencial.ID)
	}
}

// La vigencia no puede vivir dentro de una credencial firmada: una firma no
// cambia de opinión cuando alguien revoca.
func TestLaCredencialApuntaAlEstadoEnLugarDeAfirmarlo(t *testing.T) {
	c := badge.NuevaCredencial(datosDePrueba())
	if c.EstadoCredencial == nil || c.EstadoCredencial.ID == "" {
		t.Fatal("la credencial no dice dónde comprobar su vigencia")
	}
	crudo, _ := json.Marshal(c)
	if strings.Contains(string(crudo), "revoked") || strings.Contains(string(crudo), "valid\":true") {
		t.Errorf("la credencial afirma un estado que podría quedar obsoleto: %s", crudo)
	}
}

func TestElJWTSeVerificaConLaClavePublica(t *testing.T) {
	publica, privada, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	c := badge.NuevaCredencial(datosDePrueba())

	jwt, err := badge.FirmarComoJWT(c, privada, "clave-1")
	if err != nil {
		t.Fatalf("firmar: %v", err)
	}

	partes := strings.Split(jwt, ".")
	if len(partes) != 3 {
		t.Fatalf("un JWS tiene tres partes, llegaron %d", len(partes))
	}

	// La firma cubre cabecera y carga, tal como viajan.
	porFirmar := partes[0] + "." + partes[1]
	firma, err := base64.RawURLEncoding.DecodeString(partes[2])
	if err != nil {
		t.Fatalf("la firma no está en base64url sin relleno: %v", err)
	}
	if !ed25519.Verify(publica, []byte(porFirmar), firma) {
		t.Error("la firma no se verifica con la clave pública correspondiente")
	}
}

// Un verificador tiene que poder saber qué algoritmo y qué clave se usaron.
func TestLaCabeceraDelJWTDeclaraAlgoritmoYClave(t *testing.T) {
	_, privada, _ := ed25519.GenerateKey(nil)
	jwt, _ := badge.FirmarComoJWT(badge.NuevaCredencial(datosDePrueba()), privada, "clave-1")

	crudo, err := base64.RawURLEncoding.DecodeString(strings.Split(jwt, ".")[0])
	if err != nil {
		t.Fatalf("cabecera ilegible: %v", err)
	}
	var cabecera map[string]string
	if err := json.Unmarshal(crudo, &cabecera); err != nil {
		t.Fatal(err)
	}
	if cabecera["alg"] != "EdDSA" || cabecera["kid"] != "clave-1" {
		t.Errorf("cabecera inesperada: %v", cabecera)
	}
	if cabecera["typ"] != "vc+jwt" {
		t.Errorf("el tipo debería declarar que la carga es una credencial: %v", cabecera)
	}
}

// Alterar la credencial después de firmar tiene que invalidar la firma: es
// justamente lo que hace verificable una credencial verificable.
func TestManipularLaCargaInvalidaLaFirma(t *testing.T) {
	publica, privada, _ := ed25519.GenerateKey(nil)
	jwt, _ := badge.FirmarComoJWT(badge.NuevaCredencial(datosDePrueba()), privada, "clave-1")
	partes := strings.Split(jwt, ".")

	var carga map[string]any
	crudo, _ := base64.RawURLEncoding.DecodeString(partes[1])
	_ = json.Unmarshal(crudo, &carga)
	vc, _ := carga["vc"].(map[string]any)
	vc["name"] = "Un curso que nunca cursó"
	alterada, _ := json.Marshal(carga)

	porFirmar := partes[0] + "." + base64.RawURLEncoding.EncodeToString(alterada)
	firma, _ := base64.RawURLEncoding.DecodeString(partes[2])
	if ed25519.Verify(publica, []byte(porFirmar), firma) {
		t.Error("una credencial alterada no debería verificarse")
	}
}

func TestUnaClaveInvalidaNoProduceUnJWTSilenciosamenteRoto(t *testing.T) {
	_, err := badge.FirmarComoJWT(badge.NuevaCredencial(datosDePrueba()), ed25519.PrivateKey("corta"), "k")
	if err == nil {
		t.Error("firmar con una clave inválida debería fallar, no producir un JWT que nadie puede verificar")
	}
}

func TestElJWKPublicaLaClaveEnElFormatoQueEsperaUnVerificador(t *testing.T) {
	publica, _, _ := ed25519.GenerateKey(nil)
	jwk := badge.ClavePublicaJWK(publica, "clave-1")

	if jwk["kty"] != "OKP" || jwk["crv"] != "Ed25519" || jwk["alg"] != "EdDSA" {
		t.Errorf("el JWK no describe una clave Ed25519: %v", jwk)
	}
	x, err := base64.RawURLEncoding.DecodeString(jwk["x"])
	if err != nil {
		t.Fatalf("la clave no está en base64url sin relleno: %v", err)
	}
	if string(x) != string(publica) {
		t.Error("el JWK no publica la clave pública que se le pasó")
	}
}

package user

import (
	"encoding/base64"
	"testing"
)

// La entropia es la unica propiedad del token que no se puede verificar por
// inspeccion: un token corto sigue funcionando y pasando el resto de las
// pruebas, pero se vuelve adivinable. Por eso se comprueba explicitamente.
func TestNuevoTokenTieneLaEntropiaDeclarada(t *testing.T) {
	const minimoBytes = 32

	if LongitudTokenBytes < minimoBytes {
		t.Fatalf("LongitudTokenBytes = %d, se esperaban al menos %d bytes (256 bits)",
			LongitudTokenBytes, minimoBytes)
	}

	claro, _, err := NuevoToken()
	if err != nil {
		t.Fatalf("NuevoToken: %v", err)
	}

	crudo, err := base64.RawURLEncoding.DecodeString(claro)
	if err != nil {
		t.Fatalf("el token deberia decodificarse como base64 sin relleno: %v", err)
	}
	if len(crudo) != LongitudTokenBytes {
		t.Errorf("el token decodificado mide %d bytes, se esperaban %d",
			len(crudo), LongitudTokenBytes)
	}
}

func TestHashTokenEsDeterministaYDeLongitudFija(t *testing.T) {
	const bytesSHA256 = 32

	if h := HashToken("mismo-valor"); len(h) != bytesSHA256 {
		t.Errorf("el hash mide %d bytes, se esperaban %d", len(h), bytesSHA256)
	}
	if !TokensIguales(HashToken("mismo-valor"), HashToken("mismo-valor")) {
		t.Error("hashear dos veces el mismo token deberia dar el mismo resultado")
	}
}

// TokensIguales se apoya en subtle.ConstantTimeCompare, que devuelve 0 cuando
// las longitudes difieren. Se fija ese comportamiento para que un hash truncado
// nunca se acepte como valido.
func TestTokensIgualesRechazaLongitudesDistintas(t *testing.T) {
	completo := HashToken("token")
	if TokensIguales(completo, completo[:16]) {
		t.Error("un hash truncado no deberia compararse igual al completo")
	}
	if TokensIguales(completo, nil) {
		t.Error("un hash vacio no deberia compararse igual al completo")
	}
}

func TestNuevoTokenEsAleatorioYCoincideConSuHash(t *testing.T) {
	claro, hash, err := NuevoToken()
	if err != nil {
		t.Fatalf("NuevoToken: %v", err)
	}
	if claro == "" {
		t.Fatal("el token en claro no deberia venir vacio")
	}
	if !TokensIguales(hash, HashToken(claro)) {
		t.Error("el hash devuelto deberia coincidir con el del token en claro")
	}

	otro, _, err := NuevoToken()
	if err != nil {
		t.Fatalf("NuevoToken: %v", err)
	}
	if claro == otro {
		t.Error("dos tokens consecutivos no deberian coincidir")
	}
}

func TestTokensIgualesDistingueValores(t *testing.T) {
	if TokensIguales(HashToken("uno"), HashToken("dos")) {
		t.Error("hashes de tokens distintos no deberian compararse iguales")
	}
}

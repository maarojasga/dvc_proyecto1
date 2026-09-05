package user

import "testing"

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

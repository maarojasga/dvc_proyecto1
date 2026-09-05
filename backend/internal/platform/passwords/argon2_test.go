package passwords

import "testing"

func TestHashYVerificar(t *testing.T) {
	a := Nuevo(ParametrosDePrueba())
	hash, err := a.Hash("clave-de-prueba-larga")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	ok, err := a.Verificar("clave-de-prueba-larga", hash)
	if err != nil {
		t.Fatalf("Verificar: %v", err)
	}
	if !ok {
		t.Error("la clave correcta deberia verificar")
	}

	ok, err = a.Verificar("otra-clave-distinta", hash)
	if err != nil {
		t.Fatalf("Verificar: %v", err)
	}
	if ok {
		t.Error("una clave incorrecta no deberia verificar")
	}
}

func TestHashUsaSalDistintaCadaVez(t *testing.T) {
	a := Nuevo(ParametrosDePrueba())
	uno, err := a.Hash("misma-clave-siempre")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	dos, err := a.Hash("misma-clave-siempre")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if uno == dos {
		t.Error("dos hashes de la misma clave deberian diferir por la sal")
	}
}

func TestVerificarConParametrosDelPropioHash(t *testing.T) {
	// Un hash derivado con parametros antiguos debe seguir validando aunque
	// el servicio ya use otros: los parametros viajan dentro del hash.
	antiguo := Nuevo(ParametrosDePrueba())
	hash, err := antiguo.Hash("clave-de-prueba-larga")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	recalibrado := Nuevo(Parametros{Memoria: 2048, Iteraciones: 2, Paralelismo: 1, LongitudSal: 16, LongitudRes: 32})
	ok, err := recalibrado.Verificar("clave-de-prueba-larga", hash)
	if err != nil {
		t.Fatalf("Verificar: %v", err)
	}
	if !ok {
		t.Error("el hash antiguo deberia seguir verificando tras recalibrar")
	}
}

func TestVerificarRechazaHashesIlegibles(t *testing.T) {
	a := Nuevo(ParametrosDePrueba())
	for _, hash := range []string{"", "texto-plano", "$argon2i$v=19$m=1024,t=1,p=1$c2Fs$aGFzaA"} {
		if _, err := a.Verificar("cualquiera", hash); err == nil {
			t.Errorf("Verificar(%q) deberia devolver error", hash)
		}
	}
}

package storage

import "testing"

// Con llave explícita el token de sesión tiene que viajar con ella: las
// credenciales de AWS Academy son temporales y S3 rechaza una firma que
// lleve la llave de una sesión STS sin su token.
func TestLlaveExplicitaConservaElTokenDeSesion(t *testing.T) {
	v, err := credencialesDe(Config{AccessKey: "AKIA-PRUEBA", SecretKey: "secreto", SessionToken: "token"}).Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v.AccessKeyID != "AKIA-PRUEBA" || v.SessionToken != "token" {
		t.Fatalf("credenciales inesperadas: %+v", v)
	}
}

// Sin llave explícita se usa la cadena de AWS, que empieza por las variables
// AWS_*. Se prueba solo ese eslabón: el siguiente (perfil de instancia) exige
// el servicio de metadatos de EC2 y no tiene sentido fuera de una máquina.
func TestSinLlaveSeUsanLasVariablesAWS(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "ASIA-TEMPORAL")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secreto-temporal")
	t.Setenv("AWS_SESSION_TOKEN", "token-temporal")

	v, err := credencialesDe(Config{}).Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v.AccessKeyID != "ASIA-TEMPORAL" || v.SessionToken != "token-temporal" {
		t.Fatalf("la cadena no tomó las variables AWS_*: %+v", v)
	}
}

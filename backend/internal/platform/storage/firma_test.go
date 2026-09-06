package storage

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
)

// clienteQueFirmaEn construye el cliente de firma tal y como lo hace New(),
// pero sin pasar por New: New verifica el bucket contra un servidor vivo, y lo
// que se comprueba aquí es justo lo contrario, que firmar no toca la red.
func clienteQueFirmaEn(t *testing.T, hostPublico string) *Client {
	t.Helper()
	cfg := Config{
		// Un endpoint interno que nunca coincide con el público, para que
		// clienteDeFirma construya siempre un cliente propio y no devuelva
		// el interno (nil aquí, porque estas pruebas no lo necesitan).
		Endpoint:       "almacen.interno.de.prueba:9000",
		PublicEndpoint: hostPublico,
		AccessKey:      "minioadmin",
		SecretKey:      "minioadmin",
		Bucket:         "mooc",
		Region:         "us-east-1",
	}
	f, err := clienteDeFirma(cfg, nil)
	if err != nil {
		t.Fatalf("clienteDeFirma(%q): %v", hostPublico, err)
	}
	return &Client{firmante: f, bucket: cfg.Bucket}
}

func hostDe(t *testing.T, crudo string) string {
	t.Helper()
	u, err := url.Parse(crudo)
	if err != nil {
		t.Fatalf("URL no parseable %q: %v", crudo, err)
	}
	return u.Host
}

func firma(t *testing.T, crudo string) string {
	t.Helper()
	u, err := url.Parse(crudo)
	if err != nil {
		t.Fatalf("URL no parseable: %v", err)
	}
	return u.Query().Get("X-Amz-Signature")
}

// El fallo que motiva esto: una URL firmada contra "minio:9000" llega al
// navegador y muere con ERR_NAME_NOT_RESOLVED, porque ese nombre solo existe
// dentro de la red del compose.
func TestURLsPrefirmadasApuntanAlHostDelNavegador(t *testing.T) {
	c := clienteQueFirmaEn(t, "localhost:9100")
	ctx := context.Background()

	put, err := c.PresignedPutURL(ctx, "resources/abc/original", time.Hour)
	if err != nil {
		t.Fatalf("PresignedPutURL: %v", err)
	}
	get, err := c.PresignedGetURL(ctx, "resources/abc/original", time.Hour, "clase.pdf")
	if err != nil {
		t.Fatalf("PresignedGetURL: %v", err)
	}
	parte, err := c.PresignedUploadPartURL(ctx, "resources/abc/original", "upload-1", 2, time.Hour)
	if err != nil {
		t.Fatalf("PresignedUploadPartURL: %v", err)
	}

	for nombre, u := range map[string]string{"PUT": put, "GET": get, "parte": parte} {
		if host := hostDe(t, u); host != "localhost:9100" {
			t.Errorf("%s: host %q, esperaba localhost:9100 (URL: %s)", nombre, host, u)
		}
		if !strings.Contains(u, "X-Amz-Signature=") {
			t.Errorf("%s: la URL no viene firmada: %s", nombre, u)
		}
	}
}

// La firma SigV4 cubre el Host, así que no basta con reescribir la URL después
// de firmarla: hay que firmar contra el host público desde el principio. Si
// alguien "arregla" esto con un strings.Replace sobre la URL, aquí se nota.
func TestLaFirmaDependeDelHost(t *testing.T) {
	ctx := context.Background()
	interno := clienteQueFirmaEn(t, "minio:9000")
	publico := clienteQueFirmaEn(t, "localhost:9100")

	a, err := interno.PresignedPutURL(ctx, "k", time.Hour)
	if err != nil {
		t.Fatalf("interno: %v", err)
	}
	b, err := publico.PresignedPutURL(ctx, "k", time.Hour)
	if err != nil {
		t.Fatalf("público: %v", err)
	}
	if firma(t, a) == firma(t, b) {
		t.Fatal("la firma no cambió al cambiar de host: reescribir la URL a posteriori parecería válido, y no lo es")
	}
}

// Sin endpoint público configurado se reutiliza el interno: es lo correcto
// cuando la API y el navegador ven el almacén por el mismo host, y evita
// obligar a configurar algo que en ese caso no aporta nada.
func TestSinEndpointPublicoSeReutilizaElInterno(t *testing.T) {
	interno, err := minio.New("localhost:9000", &minio.Options{Region: "us-east-1"})
	if err != nil {
		t.Fatalf("minio.New: %v", err)
	}
	for _, publico := range []string{"", "localhost:9000"} {
		cfg := Config{Endpoint: "localhost:9000", PublicEndpoint: publico, Bucket: "mooc"}
		f, err := clienteDeFirma(cfg, interno)
		if err != nil {
			t.Fatalf("clienteDeFirma(%q): %v", publico, err)
		}
		if f != interno {
			t.Errorf("PublicEndpoint=%q: esperaba reutilizar el cliente interno", publico)
		}
	}
}

// Firmar no puede requerir red: el cliente de firma apunta al host del
// navegador, que desde el contenedor de la API puede no resolver siquiera. Si
// minio-go intentara descubrir la región del bucket, esto fallaría — que es
// exactamente lo que pasa si se olvida fijar Region.
func TestFirmarNoRequiereRed(t *testing.T) {
	c := clienteQueFirmaEn(t, "host.que.no.existe.invalid:9100")

	hecho := make(chan error, 1)
	go func() {
		_, err := c.PresignedPutURL(context.Background(), "k", time.Hour)
		hecho <- err
	}()
	select {
	case err := <-hecho:
		if err != nil {
			t.Fatalf("firmar contra un host irresoluble falló, señal de que se intentó una petición: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("firmar tardó más de 5 s: está haciendo red")
	}
}

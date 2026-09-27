package httpserver_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"
)

// Estas pruebas cubren el punto 5.1.5 del alcance mínimo: carga multipart
// directa, reanudable, con verificación de integridad, MIME real y escaneo
// antimalware. El almacén es el falso en memoria (almacen_falso_test.go): lo
// que se verifica aquí es la lógica de la API, no la firma SigV4, que tiene
// sus propias pruebas contra el cliente real.

// recursoParaCarga deja listo un recurso del tipo pedido y devuelve su id.
//
// Se usa "pdf" en casi todas: es un tipo que no exige procesamiento asíncrono,
// así que la carga termina en la propia respuesta y la prueba no necesita la
// cola levantada.
func recursoParaCarga(t *testing.T, c *cliente, versionID, tipo, titulo string) string {
	t.Helper()
	moduloID := moduloEnVersion(t, c, versionID, "Módulo de carga")
	res := c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/modules/"+moduloID+"/units",
		map[string]any{"title": "Unidad de carga", "position": 1})
	unidadID, _ := res.campo(t, "ID").(string)

	res = c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/units/"+unidadID+"/resources",
		map[string]any{"type": tipo, "title": titulo, "position": 1, "visible": true})
	if res.Estado != http.StatusCreated {
		t.Fatalf("crear recurso: %d %s", res.Estado, res.Crudo)
	}
	id, _ := res.campo(t, "ID").(string)
	if id == "" {
		t.Fatalf("no se pudo leer el id del recurso: %s", res.Crudo)
	}
	return id
}

func pdfDePrueba(relleno int) []byte {
	cuerpo := append([]byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\n"), bytes.Repeat([]byte("x"), relleno)...)
	return append(cuerpo, []byte("\n%%EOF\n")...)
}

func sha256Hex(datos []byte) string {
	suma := sha256.Sum256(datos)
	return hex.EncodeToString(suma[:])
}

// El ciclo completo: iniciar, subir partes, consultar lo ya recibido tras una
// interrupción, reanudar solo lo que falta y completar.
func TestCargaMultipartSeReanudaDesdeLasPartesYaRecibidas(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof-multi@example.com", "curso-multi")
	recursoID := recursoParaCarga(t, c, versionID, "pdf", "Guía en PDF")
	base := "/courses/versions/" + versionID + "/resources/" + recursoID

	res := c.hacer(http.MethodPost, base+"/multipart/initiate", map[string]any{"content_type": "application/pdf"})
	if res.Estado != http.StatusOK {
		t.Fatalf("iniciar multipart: %d %s", res.Estado, res.Crudo)
	}
	uploadID, _ := res.campo(t, "upload_id").(string)
	if uploadID == "" {
		t.Fatalf("no se recibió upload_id: %s", res.Crudo)
	}

	completo := pdfDePrueba(2048)
	corte := len(completo) / 2
	trozos := [][]byte{completo[:corte], completo[corte:]}

	// Solo llega la primera parte: aquí se cae la red.
	res = c.hacer(http.MethodPost, base+"/multipart/part-url",
		map[string]any{"upload_id": uploadID, "part_number": 1})
	if res.Estado != http.StatusOK {
		t.Fatalf("pedir url de parte: %d %s", res.Estado, res.Crudo)
	}
	if u, _ := res.campo(t, "upload_url").(string); u == "" {
		t.Fatalf("no se recibió upload_url de la parte: %s", res.Crudo)
	}
	etag1 := env.almacen.ponerParte(uploadID, 1, trozos[0])

	// Al volver, el cliente pregunta qué partes ya están para no reenviarlas.
	res = c.hacer(http.MethodGet, base+"/multipart/parts?upload_id="+uploadID, nil)
	if res.Estado != http.StatusOK {
		t.Fatalf("listar partes para reanudar: %d %s", res.Estado, res.Crudo)
	}
	partes, _ := res.campo(t, "parts").([]any)
	if len(partes) != 1 {
		t.Fatalf("se esperaba 1 parte ya recibida, llegaron %d: %s", len(partes), res.Crudo)
	}
	if n, _ := partes[0].(map[string]any)["part_number"].(float64); int(n) != 1 {
		t.Errorf("la parte listada no es la 1: %s", res.Crudo)
	}

	// Reanudación: solo se sube lo que faltaba.
	res = c.hacer(http.MethodPost, base+"/multipart/part-url",
		map[string]any{"upload_id": uploadID, "part_number": 2})
	if res.Estado != http.StatusOK {
		t.Fatalf("pedir url de la parte 2: %d %s", res.Estado, res.Crudo)
	}
	etag2 := env.almacen.ponerParte(uploadID, 2, trozos[1])

	res = c.hacer(http.MethodPost, base+"/multipart/complete", map[string]any{
		"upload_id": uploadID,
		"parts": []map[string]any{
			{"part_number": 1, "etag": etag1},
			{"part_number": 2, "etag": etag2},
		},
		"checksum_sha256": sha256Hex(completo),
	})
	if res.Estado != http.StatusOK {
		t.Fatalf("completar multipart: %d %s", res.Estado, res.Crudo)
	}
	if estado, _ := res.campo(t, "status").(string); estado != "ready" {
		t.Errorf("el recurso debería quedar listo, llegó %q: %s", estado, res.Crudo)
	}
	if suma, _ := res.campo(t, "checksum_sha256").(string); suma != sha256Hex(completo) {
		t.Errorf("el checksum devuelto no es el del objeto ensamblado: %s", res.Crudo)
	}
	if mime, _ := res.campo(t, "mime_type").(string); mime != "application/pdf" {
		t.Errorf("el MIME detectado debería ser application/pdf, llegó %q", mime)
	}
}

// Una parte corrupta cambia el hash del objeto ensamblado, y eso es
// exactamente lo que el checksum declarado por el cliente debe delatar.
func TestUnChecksumQueNoCuadraRechazaYBorraElObjeto(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof-sha@example.com", "curso-sha")
	recursoID := recursoParaCarga(t, c, versionID, "pdf", "PDF alterado")
	base := "/courses/versions/" + versionID + "/resources/" + recursoID

	res := c.hacer(http.MethodPost, base+"/multipart/initiate", map[string]any{"content_type": "application/pdf"})
	uploadID, _ := res.campo(t, "upload_id").(string)

	original := pdfDePrueba(512)
	alterado := append(append([]byte{}, original...), []byte("bytes de más")...)
	etag := env.almacen.ponerParte(uploadID, 1, alterado)

	res = c.hacer(http.MethodPost, base+"/multipart/complete", map[string]any{
		"upload_id":       uploadID,
		"parts":           []map[string]any{{"part_number": 1, "etag": etag}},
		"checksum_sha256": sha256Hex(original),
	})
	if res.Estado != http.StatusUnprocessableEntity {
		t.Fatalf("se esperaba 422 por integridad, llegó %d: %s", res.Estado, res.Crudo)
	}
	if codigo, _ := res.campo(t, "error", "code").(string); codigo != "checksum_mismatch" {
		t.Errorf("código de error inesperado: %s", res.Crudo)
	}
	if env.almacen.existe("resources/" + recursoID + "/original") {
		t.Error("el objeto rechazado debería haberse borrado del almacén")
	}
}

// El escaneo antimalware corre sobre el objeto ya ensamblado, no sobre lo que
// el cliente declare.
func TestElEscaneoAntimalwareRechazaLaCarga(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof-virus@example.com", "curso-virus")
	recursoID := recursoParaCarga(t, c, versionID, "file", "Material adjunto")
	base := "/courses/versions/" + versionID + "/resources/" + recursoID

	res := c.hacer(http.MethodPost, base+"/multipart/initiate", map[string]any{"content_type": "application/octet-stream"})
	uploadID, _ := res.campo(t, "upload_id").(string)

	// Vector de prueba estándar, compuesto en tiempo de ejecución para no
	// dejarlo escrito en el repositorio.
	vector := []byte(`X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*`)
	etag := env.almacen.ponerParte(uploadID, 1, vector)

	res = c.hacer(http.MethodPost, base+"/multipart/complete", map[string]any{
		"upload_id": uploadID,
		"parts":     []map[string]any{{"part_number": 1, "etag": etag}},
	})
	if res.Estado != http.StatusUnprocessableEntity {
		t.Fatalf("se esperaba 422 por antimalware, llegó %d: %s", res.Estado, res.Crudo)
	}
	if codigo, _ := res.campo(t, "error", "code").(string); codigo != "malware_detected" {
		t.Errorf("código de error inesperado: %s", res.Crudo)
	}
	if env.almacen.existe("resources/" + recursoID + "/original") {
		t.Error("el objeto infectado debería haberse borrado del almacén")
	}
}

// El tipo real se deduce de los bytes: declarar "application/pdf" sobre un
// HTML no cuela.
func TestElMimeRealPrevaleceSobreElDeclarado(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof-mime@example.com", "curso-mime")
	recursoID := recursoParaCarga(t, c, versionID, "pdf", "PDF falso")
	base := "/courses/versions/" + versionID + "/resources/" + recursoID

	res := c.hacer(http.MethodPost, base+"/multipart/initiate", map[string]any{"content_type": "application/pdf"})
	uploadID, _ := res.campo(t, "upload_id").(string)

	html := []byte("<!DOCTYPE html><html><body><script>fetch('/api/v1/auth/me')</script></body></html>")
	etag := env.almacen.ponerParte(uploadID, 1, html)

	res = c.hacer(http.MethodPost, base+"/multipart/complete", map[string]any{
		"upload_id": uploadID,
		"parts":     []map[string]any{{"part_number": 1, "etag": etag}},
	})
	if res.Estado != http.StatusUnprocessableEntity {
		t.Fatalf("se esperaba 422 por MIME, llegó %d: %s", res.Estado, res.Crudo)
	}
	if codigo, _ := res.campo(t, "error", "code").(string); codigo != "mime_mismatch" {
		t.Errorf("código de error inesperado: %s", res.Crudo)
	}
}

// La subida simple pasa por los mismos controles que la multipart. Antes no:
// un archivo por debajo del umbral del cliente entraba sin verificar nada, que
// es justo lo que buscaría quien quiera colar algo.
func TestLaSubidaSimpleAplicaLosMismosControles(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof-simple@example.com", "curso-simple")
	recursoID := recursoParaCarga(t, c, versionID, "file", "Adjunto pequeño")
	base := "/courses/versions/" + versionID + "/resources/" + recursoID
	clave := "resources/" + recursoID + "/original"

	res := c.hacer(http.MethodPost, base+"/upload-url", map[string]any{"mime_type": "application/octet-stream"})
	if res.Estado != http.StatusOK {
		t.Fatalf("pedir url de subida: %d %s", res.Estado, res.Crudo)
	}

	// Un ejecutable pequeño, del tamaño que antes evitaba la multipart.
	env.almacen.ponerObjeto(clave, append([]byte{'M', 'Z', 0x90, 0x00}, bytes.Repeat([]byte{0x41}, 256)...), "application/octet-stream")

	res = c.hacer(http.MethodPost, base+"/confirm-upload", map[string]any{})
	if res.Estado != http.StatusUnprocessableEntity {
		t.Fatalf("se esperaba 422 en la subida simple, llegó %d: %s", res.Estado, res.Crudo)
	}
	if codigo, _ := res.campo(t, "error", "code").(string); codigo != "malware_detected" {
		t.Errorf("código de error inesperado: %s", res.Crudo)
	}
	if env.almacen.existe(clave) {
		t.Error("el ejecutable rechazado debería haberse borrado del almacén")
	}
}

// Y una subida simple legítima sí termina, con su checksum verificado.
func TestLaSubidaSimpleLegitimaQuedaLista(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof-ok@example.com", "curso-ok")
	recursoID := recursoParaCarga(t, c, versionID, "pdf", "Lectura")
	base := "/courses/versions/" + versionID + "/resources/" + recursoID
	clave := "resources/" + recursoID + "/original"

	if res := c.hacer(http.MethodPost, base+"/upload-url", map[string]any{"mime_type": "application/pdf"}); res.Estado != http.StatusOK {
		t.Fatalf("pedir url de subida: %d %s", res.Estado, res.Crudo)
	}
	contenido := pdfDePrueba(128)
	env.almacen.ponerObjeto(clave, contenido, "application/pdf")

	res := c.hacer(http.MethodPost, base+"/confirm-upload",
		map[string]any{"checksum_sha256": sha256Hex(contenido)})
	if res.Estado != http.StatusOK {
		t.Fatalf("confirmar subida: %d %s", res.Estado, res.Crudo)
	}
	if estado, _ := res.campo(t, "status").(string); estado != "ready" {
		t.Errorf("el recurso debería quedar listo, llegó %q: %s", estado, res.Crudo)
	}
}

// La URL de carga vale 24 horas, que es el plazo de reanudación que fija el
// alcance mínimo.
func TestLasURLDeCargaDuranLasVeinticuatroHorasQueExigeElAlcance(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID := env.profesorConCurso("prof-vigencia@example.com", "curso-vigencia")
	recursoID := recursoParaCarga(t, c, versionID, "pdf", "Documento")
	base := "/courses/versions/" + versionID + "/resources/" + recursoID

	res := c.hacer(http.MethodPost, base+"/upload-url", map[string]any{"mime_type": "application/pdf"})
	url, _ := res.campo(t, "upload_url").(string)
	if !bytes.Contains([]byte(url), []byte("expira=86400")) {
		t.Errorf("la URL de subida debería durar 24 h: %s", url)
	}

	res = c.hacer(http.MethodPost, base+"/multipart/initiate", map[string]any{"content_type": "application/pdf"})
	uploadID, _ := res.campo(t, "upload_id").(string)
	res = c.hacer(http.MethodPost, base+"/multipart/part-url",
		map[string]any{"upload_id": uploadID, "part_number": 1})
	urlParte, _ := res.campo(t, "upload_url").(string)
	if !bytes.Contains([]byte(urlParte), []byte("expira=86400")) {
		t.Errorf("la URL de una parte debería durar 24 h: %s", urlParte)
	}
}

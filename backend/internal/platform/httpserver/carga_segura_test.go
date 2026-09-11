package httpserver_test

import (
	"net/http"
	"testing"
)

// Estas pruebas cubren lo que el enunciado pide de la carga directa:
// verificación de integridad, MIME real y escaneo antimalware. Todas van por
// la ruta multipart, que es la que usa el frontend, pero las comprobaciones
// son compartidas con el PUT simple.

// recursoBinario deja un profesor con un recurso listo para recibir archivo y
// devuelve el cliente, la versión y el recurso.
func recursoBinario(t *testing.T, env *entorno, correo, slug, tipo string) (*cliente, string, string) {
	t.Helper()
	c, versionID := env.profesorConCurso(correo, slug)
	moduloID := moduloEnVersion(t, c, versionID, "Módulo")

	res := c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/modules/"+moduloID+"/units",
		map[string]any{"title": "Unidad", "position": 1})
	unidadID, _ := res.campo(t, "ID").(string)

	res = c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/units/"+unidadID+"/resources",
		map[string]any{"type": tipo, "title": "Material", "position": 1, "visible": true})
	recursoID, _ := res.campo(t, "ID").(string)
	return c, versionID, recursoID
}

// subirPor hace el ciclo initiate → part-url → complete y devuelve la
// respuesta del complete. El almacén de pruebas da la parte por subida al
// firmarla, así que no hace falta un PUT real.
func subirPor(t *testing.T, c *cliente, versionID, recursoID string, cuerpoComplete map[string]any) respuesta {
	t.Helper()
	base := "/courses/versions/" + versionID + "/resources/" + recursoID
	res := c.hacer(http.MethodPost, base+"/multipart/initiate", map[string]any{"content_type": "application/pdf"})
	if res.Estado != http.StatusOK {
		t.Fatalf("initiate: %d %s", res.Estado, res.Crudo)
	}
	uploadID, _ := res.campo(t, "upload_id").(string)

	res = c.hacer(http.MethodPost, base+"/multipart/part-url",
		map[string]any{"upload_id": uploadID, "part_number": 1})
	if res.Estado != http.StatusOK {
		t.Fatalf("part-url: %d %s", res.Estado, res.Crudo)
	}

	cuerpo := map[string]any{
		"upload_id": uploadID,
		"parts":     []map[string]any{{"part_number": 1, "etag": "etag-1"}},
	}
	for k, v := range cuerpoComplete {
		cuerpo[k] = v
	}
	return c.hacer(http.MethodPost, base+"/multipart/complete", cuerpo)
}

// Un checksum opcional no verifica integridad: solo la documenta cuando ya
// salió bien. Se exige.
func TestCerrarLaCargaSinChecksumSeRechaza(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID, recursoID := recursoBinario(t, env, "prof-sin-sha@example.com", "curso-sin-sha", "pdf")

	res := subirPor(t, c, versionID, recursoID, nil)
	if res.Estado != http.StatusBadRequest {
		t.Fatalf("esperaba 400 sin checksum, obtuve %d: %s", res.Estado, res.Crudo)
	}
	if codigo := res.codigoDeError(t); codigo != "checksum_requerido" {
		t.Errorf("código %q, esperaba checksum_requerido", codigo)
	}
}

func TestUnChecksumQueNoCuadraRechazaYBorraElObjeto(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID, recursoID := recursoBinario(t, env, "prof-sha-malo@example.com", "curso-sha-malo", "pdf")

	res := subirPor(t, c, versionID, recursoID, map[string]any{
		"checksum_sha256": "0000000000000000000000000000000000000000000000000000000000000000",
	})
	if res.Estado != http.StatusUnprocessableEntity {
		t.Fatalf("esperaba 422, obtuve %d: %s", res.Estado, res.Crudo)
	}
	if codigo := res.codigoDeError(t); codigo != "integridad_fallida" {
		t.Errorf("código %q, esperaba integridad_fallida", codigo)
	}
	// Un objeto que se decidió rechazar no se queda en el almacén.
	if !env.almacen.huboBorrado() {
		t.Error("el objeto rechazado por integridad no se borró")
	}
}

// EICAR es el archivo de prueba estándar: inocuo, y todo motor lo reporta.
func TestUnArchivoInfectadoSeRechazaYNoSeQueda(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID, recursoID := recursoBinario(t, env, "prof-virus@example.com", "curso-virus", "file")

	sha := env.almacen.pon([]byte(`X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*`))
	res := subirPor(t, c, versionID, recursoID, map[string]any{"checksum_sha256": sha})

	if res.Estado != http.StatusUnprocessableEntity {
		t.Fatalf("esperaba 422, obtuve %d: %s", res.Estado, res.Crudo)
	}
	if codigo := res.codigoDeError(t); codigo != "archivo_infectado" {
		t.Errorf("código %q, esperaba archivo_infectado", codigo)
	}
	if !env.almacen.huboBorrado() {
		t.Error("el archivo infectado se quedó en el almacén")
	}
}

// El MIME lo decide el contenido, no lo que declaró el cliente. HTML es el
// caso que más importa: servido en línea desde el dominio del almacén,
// ejecutaría scripts con ese origen.
func TestElMIMERealManda(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID, recursoID := recursoBinario(t, env, "prof-html@example.com", "curso-html", "pdf")

	sha := env.almacen.pon([]byte("<html><body><script>alert(1)</script></body></html>"))
	res := subirPor(t, c, versionID, recursoID, map[string]any{"checksum_sha256": sha})

	if res.Estado != http.StatusUnprocessableEntity {
		t.Fatalf("esperaba 422, obtuve %d: %s", res.Estado, res.Crudo)
	}
	if codigo := res.codigoDeError(t); codigo != "tipo_no_admitido" {
		t.Errorf("código %q, esperaba tipo_no_admitido", codigo)
	}
}

// El camino bueno: un PDF de verdad, con su checksum, queda listo y la API
// devuelve lo que verificó.
func TestUnaCargaCorrectaQuedaListaYDevuelveLoVerificado(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID, recursoID := recursoBinario(t, env, "prof-ok@example.com", "curso-ok", "pdf")

	sha := env.almacen.pon([]byte("%PDF-1.7\nmaterial del curso"))
	res := subirPor(t, c, versionID, recursoID, map[string]any{"checksum_sha256": sha})

	if res.Estado != http.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d: %s", res.Estado, res.Crudo)
	}
	if estado, _ := res.campo(t, "status").(string); estado != "ready" {
		t.Errorf("status %q, esperaba ready", estado)
	}
	if devuelto, _ := res.campo(t, "checksum_sha256").(string); devuelto != sha {
		t.Errorf("checksum devuelto %q, esperaba %q", devuelto, sha)
	}
	if env.almacen.huboBorrado() {
		t.Error("una carga correcta borró el objeto")
	}
}

// Lo que el rechazo tiene que impedir de verdad: que el recurso se publique.
func TestUnRecursoConCargaRechazadaNoSePublica(t *testing.T) {
	env := nuevoEntorno(t)
	c, versionID, recursoID := recursoBinario(t, env, "prof-no-pub@example.com", "curso-no-pub", "pdf")

	sha := env.almacen.pon([]byte(`X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*`))
	if res := subirPor(t, c, versionID, recursoID, map[string]any{"checksum_sha256": sha}); res.Estado == http.StatusOK {
		t.Fatalf("el archivo infectado se aceptó: %s", res.Crudo)
	}
	_ = recursoID

	res := c.hacer(http.MethodPost, "/courses/versions/"+versionID+"/publish", nil)
	if res.Estado == http.StatusOK {
		t.Fatal("la versión se publicó con un recurso cuya carga fue rechazada")
	}
	if res.Estado != http.StatusUnprocessableEntity {
		t.Fatalf("esperaba 422 con la lista de motivos, obtuve %d: %s", res.Estado, res.Crudo)
	}
}

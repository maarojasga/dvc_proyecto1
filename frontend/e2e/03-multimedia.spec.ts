import { test, expect } from "@playwright/test";
import { API, conToken, credencialesDeAdmin, sufijo, tokenDeAdmin,
  tokenDeSesion } from "./ayudas";
import { createHash } from "node:crypto";

/**
 * Segmento 3 — Carga multimedia.
 *
 * Criterio: multimedia y distribución. El enunciado pide URLs prefirmadas,
 * reanudación, checksum, validación MIME, escaneo antimalware, encolamiento y
 * conservación del original.
 *
 * Este segmento se conduce por la API y no por el navegador a propósito: lo
 * que hay que demostrar es que el binario **no pasa por la API** sino que va
 * directo al almacén con una URL prefirmada. Manejarlo desde Playwright con
 * un <input type=file> escondería justamente eso. La parte de interfaz —el
 * botón que reanuda una subida a medias— se cubre en la demostración grabada.
 */

// PDF mínimo válido: empieza por %PDF, que es lo que olfatea el servidor.
const PDF = Buffer.from(
  "%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n" +
    "2 0 obj<</Type/Pages/Kids[]/Count 0>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF\n",
);

// Cadena de prueba estándar del antivirus. No es malware: es el patrón que
// todo escáner debe reconocer, y sirve para probar que el escaneo ocurre.
const EICAR =
  "X5O!P%@AP[4\\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*";

async function recursoBinario(
  request: Parameters<typeof tokenDeSesion>[0],
  token: string,
  tipo: string,
) {
  const marca = sufijo();
  const creacion = await request.post(`${API}/api/v1/courses`, {
    headers: conToken(token),
    data: { slug: `media-${marca}`, title: `Media ${marca}` },
  });
  const { version_id: versionId } = await creacion.json();

  const modulo = await request.post(`${API}/api/v1/courses/versions/${versionId}/modules`, {
    headers: conToken(token),
    data: { title: "M", position: 1 },
  });
  const moduloId = (await modulo.json()).ID;
  const unidad = await request.post(
    `${API}/api/v1/courses/versions/${versionId}/modules/${moduloId}/units`,
    { headers: conToken(token), data: { title: "U", position: 1 } },
  );
  const unidadId = (await unidad.json()).ID;

  const recurso = await request.post(
    `${API}/api/v1/courses/versions/${versionId}/units/${unidadId}/resources`,
    {
      headers: conToken(token),
      data: {
        Type: tipo, title: `Recurso ${tipo}`, position: 1,
        visible: true, required: false, downloadable: true,
      },
    },
  );
  return { versionId, resourceId: (await recurso.json()).ID };
}

test.describe("3. Carga multimedia", () => {
  test("el binario viaja directo al almacén con URL prefirmada y se verifica su checksum", async ({
    request,
  }) => {
    const token = await tokenDeAdmin(request);
    const { versionId, resourceId } = await recursoBinario(request, token, "pdf");

    const firma = await request.post(
      `${API}/api/v1/courses/versions/${versionId}/resources/${resourceId}/upload-url`,
      { headers: conToken(token), data: { mime_type: "application/pdf" } },
    );
    expect(firma.ok(), "la API firma la subida").toBeTruthy();
    const { upload_url: url } = await firma.json();

    // La URL apunta al almacén, no a la API: el binario no la atraviesa.
    expect(new URL(url).port, "la subida no va contra el puerto de la API").not.toBe(
      new URL(API).port,
    );

    const subida = await request.fetch(url, {
      method: "PUT",
      data: PDF,
      headers: { "Content-Type": "application/pdf" },
    });
    expect(subida.ok(), `el almacén acepta el PUT prefirmado: ${subida.status()}`).toBeTruthy();

    const sha = createHash("sha256").update(PDF).digest("hex");
    const confirmacion = await request.post(
      `${API}/api/v1/courses/versions/${versionId}/resources/${resourceId}/confirm-upload`,
      { headers: conToken(token), data: { checksum_sha256: sha } },
    );
    expect(confirmacion.ok(), `la verificación acepta el checksum correcto`).toBeTruthy();
  });

  test("un checksum que no coincide se rechaza", async ({ request }) => {
    const token = await tokenDeAdmin(request);
    const { versionId, resourceId } = await recursoBinario(request, token, "pdf");

    const { upload_url: url } = await (
      await request.post(
        `${API}/api/v1/courses/versions/${versionId}/resources/${resourceId}/upload-url`,
        { headers: conToken(token), data: { mime_type: "application/pdf" } },
      )
    ).json();
    await request.fetch(url, { method: "PUT", data: PDF, headers: { "Content-Type": "application/pdf" } });

    const confirmacion = await request.post(
      `${API}/api/v1/courses/versions/${versionId}/resources/${resourceId}/confirm-upload`,
      { headers: conToken(token), data: { checksum_sha256: "0".repeat(64) } },
    );
    expect(confirmacion.status(), "el fallo de integridad es del cliente, no del servidor").toBe(422);
  });

  test("el contenido real manda sobre el tipo declarado", async ({ request }) => {
    const token = await tokenDeAdmin(request);
    const { versionId, resourceId } = await recursoBinario(request, token, "pdf");

    const { upload_url: url } = await (
      await request.post(
        `${API}/api/v1/courses/versions/${versionId}/resources/${resourceId}/upload-url`,
        { headers: conToken(token), data: { mime_type: "application/pdf" } },
      )
    ).json();

    // Se declara PDF y se sube HTML. Lo declarado no acredita nada.
    await request.fetch(url, {
      method: "PUT",
      data: Buffer.from("<!doctype html><html><body>no soy un pdf</body></html>"),
      headers: { "Content-Type": "application/pdf" },
    });

    const confirmacion = await request.post(
      `${API}/api/v1/courses/versions/${versionId}/resources/${resourceId}/confirm-upload`,
      { headers: conToken(token), data: {} },
    );
    expect(confirmacion.status(), "el MIME real no corresponde al tipo del recurso").toBe(422);
  });

  test("el escaneo antimalware rechaza el vector de prueba estándar", async ({ request }) => {
    const token = await tokenDeAdmin(request);
    const { versionId, resourceId } = await recursoBinario(request, token, "file");

    const { upload_url: url } = await (
      await request.post(
        `${API}/api/v1/courses/versions/${versionId}/resources/${resourceId}/upload-url`,
        { headers: conToken(token), data: { mime_type: "application/octet-stream" } },
      )
    ).json();
    await request.fetch(url, { method: "PUT", data: Buffer.from(EICAR) });

    const confirmacion = await request.post(
      `${API}/api/v1/courses/versions/${versionId}/resources/${resourceId}/confirm-upload`,
      { headers: conToken(token), data: {} },
    );
    expect(confirmacion.status(), "el archivo infectado no entra").toBe(422);
  });

  test("una carga multipart interrumpida se reanuda sin reenviar lo que ya llegó", async ({
    request,
  }) => {
    const token = await tokenDeAdmin(request);
    const { versionId, resourceId } = await recursoBinario(request, token, "file");
    const base = `${API}/api/v1/courses/versions/${versionId}/resources/${resourceId}/multipart`;

    const inicio = await request.post(`${base}/initiate`, {
      headers: conToken(token),
      data: { content_type: "application/octet-stream" },
    });
    expect(inicio.ok()).toBeTruthy();
    const { upload_id: uploadId } = await inicio.json();

    // Las partes de una multipart tienen mínimo 5 MiB salvo la última.
    const parte = Buffer.alloc(5 * 1024 * 1024, "a");
    const ultima = Buffer.from("final");

    const urlParte1 = (
      await (
        await request.post(`${base}/part-url`, {
          headers: conToken(token),
          data: { upload_id: uploadId, part_number: 1 },
        })
      ).json()
    ).upload_url;
    const put1 = await request.fetch(urlParte1, { method: "PUT", data: parte });
    expect(put1.ok()).toBeTruthy();

    // Aquí se "corta la red": el cliente muere sin subir la parte 2. Al
    // volver, pregunta al almacén qué llegó en vez de fiarse de su registro.
    const listado = await request.get(`${base}/parts?upload_id=${uploadId}`, {
      headers: conToken(token),
    });
    expect(listado.ok()).toBeTruthy();
    const partes = (await listado.json()).parts ?? [];
    expect(partes.length, "el almacén confirma que la parte 1 ya está").toBe(1);
    expect(partes[0].part_number).toBe(1);

    const urlParte2 = (
      await (
        await request.post(`${base}/part-url`, {
          headers: conToken(token),
          data: { upload_id: uploadId, part_number: 2 },
        })
      ).json()
    ).upload_url;
    const put2 = await request.fetch(urlParte2, { method: "PUT", data: ultima });
    const etag2 = put2.headers()["etag"];

    const cierre = await request.post(`${base}/complete`, {
      headers: conToken(token),
      data: {
        upload_id: uploadId,
        parts: [
          { part_number: 1, etag: put1.headers()["etag"] },
          { part_number: 2, etag: etag2 },
        ],
        checksum_sha256: createHash("sha256").update(Buffer.concat([parte, ultima])).digest("hex"),
      },
    });
    expect(cierre.ok(), `la multipart se cierra tras reanudar: ${await cierre.text()}`).toBeTruthy();
  });
});

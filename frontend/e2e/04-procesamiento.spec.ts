import { test, expect } from "@playwright/test";
import { API, conToken, credencialesDeAdmin, sufijo, tokenDeAdmin,
  tokenDeSesion } from "./ayudas";
import { createHash } from "node:crypto";

/**
 * Segmento 4 — Procesamiento y fallos.
 *
 * Criterio: arquitectura, multimedia, calidad operativa. El enunciado pide
 * estados del recurso, doble entrega idempotente, backoff, DLQ, alerta y
 * reencolado con la misma clave.
 *
 * Lo que se puede acreditar desde fuera del proceso es esto: que confirmar
 * una carga deja el recurso encolado y no bloquea la API, que el original se
 * conserva, y que repetir la confirmación no encola dos veces. El backoff, la
 * DLQ y el reencolado viven dentro del worker y están cubiertos por las
 * pruebas de integración de `internal/platform/queue`, que sí pueden inyectar
 * fallos; reproducirlos desde el navegador exigiría esperar tres reintentos
 * reales y no añadiría evidencia.
 */

function wavSilencio(muestras = 1024): Buffer {
  const dataSize = muestras * 2;
  const buf = Buffer.alloc(44 + dataSize);
  buf.write("RIFF", 0);
  buf.writeUInt32LE(36 + dataSize, 4);
  buf.write("WAVE", 8);
  buf.write("fmt ", 12);
  buf.writeUInt32LE(16, 16);
  buf.writeUInt16LE(1, 20);
  buf.writeUInt16LE(1, 22);
  buf.writeUInt32LE(8000, 24);
  buf.writeUInt32LE(16000, 28);
  buf.writeUInt16LE(2, 32);
  buf.writeUInt16LE(16, 34);
  buf.write("data", 36);
  buf.writeUInt32LE(dataSize, 40);
  return buf;
}

test.describe("4. Procesamiento y fallos", () => {
  test("confirmar la carga encola sin bloquear la API y conserva el original", async ({
    request,
  }) => {
    const token = await tokenDeAdmin(request);
    const marca = sufijo();

    const creacion = await request.post(`${API}/api/v1/courses`, {
      headers: conToken(token),
      data: { slug: `proc-${marca}`, title: `Proceso ${marca}` },
    });
    const { version_id: versionId } = await creacion.json();
    const moduloId = (
      await (
        await request.post(`${API}/api/v1/courses/versions/${versionId}/modules`, {
          headers: conToken(token),
          data: { title: "M", position: 1 },
        })
      ).json()
    ).ID;
    const unidadId = (
      await (
        await request.post(
          `${API}/api/v1/courses/versions/${versionId}/modules/${moduloId}/units`,
          { headers: conToken(token), data: { title: "U", position: 1 } },
        )
      ).json()
    ).ID;
    const resourceId = (
      await (
        await request.post(
          `${API}/api/v1/courses/versions/${versionId}/units/${unidadId}/resources`,
          {
            headers: conToken(token),
            data: {
              type: "audio", title: "Audio de prueba", position: 1,
              visible: true, required: false, downloadable: true,
            },
          },
        )
      ).json()
    ).ID;

    const binario = wavSilencio();
    const { upload_url: url } = await (
      await request.post(
        `${API}/api/v1/courses/versions/${versionId}/resources/${resourceId}/upload-url`,
        { headers: conToken(token), data: { mime_type: "audio/wav" } },
      )
    ).json();
    await request.fetch(url, { method: "PUT", data: binario });

    const sha = createHash("sha256").update(binario).digest("hex");
    const antes = Date.now();
    const confirmacion = await request.post(
      `${API}/api/v1/courses/versions/${versionId}/resources/${resourceId}/confirm-upload`,
      { headers: conToken(token), data: { checksum_sha256: sha } },
    );
    const tardanza = Date.now() - antes;
    expect(confirmacion.ok(), `confirmar la carga: ${await confirmacion.text()}`).toBeTruthy();

    const resultado = await confirmacion.json();
    expect(resultado.status, "el recurso queda encolado, no transcodificado en línea").toBe(
      "queued",
    );
    // La API no espera a FFmpeg: si esperara, este número sería de minutos.
    expect(tardanza, "confirmar no bloquea a la espera del worker").toBeLessThan(15_000);

    const version = await (
      await request.get(`${API}/api/v1/courses/versions/${versionId}`, { headers: conToken(token) })
    ).json();
    const recurso = version.Modules.flatMap((m: any) => m.Units)
      .flatMap((u: any) => u.Resources)
      .find((r: any) => r.ID === resourceId);
    expect(recurso.ProcessingStatus, "y su estado lo refleja").toMatch(/pending|processing|ready/);
    expect(recurso.ObjectKey, "el original se conserva bajo su clave").toBeTruthy();

    // Doble entrega: repetir la confirmación reutiliza el activo y no encola
    // una segunda transcodificación.
    const repetida = await request.post(
      `${API}/api/v1/courses/versions/${versionId}/resources/${resourceId}/confirm-upload`,
      { headers: conToken(token), data: { checksum_sha256: sha } },
    );
    expect(repetida.ok(), "confirmar dos veces es inofensivo").toBeTruthy();
    const repetido = await repetida.json();
    expect(repetido.media_asset_id, "la segunda confirmación reutiliza el activo").toBe(
      resultado.media_asset_id,
    );
  });

  test("un audio que no es audio se rechaza al confirmar y no se encola", async ({
    request,
  }) => {
    const token = await tokenDeAdmin(request);
    const marca = sufijo();

    const { version_id: versionId } = await (
      await request.post(`${API}/api/v1/courses`, {
        headers: conToken(token),
        data: { slug: `basura-${marca}`, title: `Basura ${marca}` },
      })
    ).json();
    const moduloId = (
      await (
        await request.post(`${API}/api/v1/courses/versions/${versionId}/modules`, {
          headers: conToken(token), data: { title: "M", position: 1 },
        })
      ).json()
    ).ID;
    const unidadId = (
      await (
        await request.post(
          `${API}/api/v1/courses/versions/${versionId}/modules/${moduloId}/units`,
          { headers: conToken(token), data: { title: "U", position: 1 } },
        )
      ).json()
    ).ID;
    const resourceId = (
      await (
        await request.post(
          `${API}/api/v1/courses/versions/${versionId}/units/${unidadId}/resources`,
          {
            headers: conToken(token),
            data: {
              type: "audio", title: "Audio falso", position: 1,
              visible: true, required: false, downloadable: true,
            },
          },
        )
      ).json()
    ).ID;

    const binario = Buffer.alloc(2048, 7);
    const { upload_url: url } = await (
      await request.post(
        `${API}/api/v1/courses/versions/${versionId}/resources/${resourceId}/upload-url`,
        { headers: conToken(token), data: { mime_type: "application/octet-stream" } },
      )
    ).json();
    await request.fetch(url, { method: "PUT", data: binario });

    const confirmacion = await request.post(
      `${API}/api/v1/courses/versions/${versionId}/resources/${resourceId}/confirm-upload`,
      { headers: conToken(token), data: { checksum_sha256: createHash("sha256").update(binario).digest("hex") } },
    );
    expect(confirmacion.status(), "la basura no entra a la cola").toBe(422);
    const cuerpo = await confirmacion.json();
    expect(cuerpo.error?.code).toBe("media_container_unrecognized");
  });

  test("un recurso que aún se procesa no se entrega como si estuviera listo", async ({
    request,
  }) => {
    const token = await tokenDeAdmin(request);
    const marca = sufijo();

    const { version_id: versionId } = await (
      await request.post(`${API}/api/v1/courses`, {
        headers: conToken(token),
        data: { slug: `nolisto-${marca}`, title: `No listo ${marca}` },
      })
    ).json();
    const moduloId = (
      await (
        await request.post(`${API}/api/v1/courses/versions/${versionId}/modules`, {
          headers: conToken(token), data: { title: "M", position: 1 },
        })
      ).json()
    ).ID;
    const unidadId = (
      await (
        await request.post(
          `${API}/api/v1/courses/versions/${versionId}/modules/${moduloId}/units`,
          { headers: conToken(token), data: { title: "U", position: 1 } },
        )
      ).json()
    ).ID;
    await request.post(
      `${API}/api/v1/courses/versions/${versionId}/units/${unidadId}/resources`,
      {
        headers: conToken(token),
        data: {
          type: "video", title: "Vídeo sin procesar", position: 1,
          visible: true, required: true, downloadable: false,
        },
      },
    );

    // Publicar con un recurso visible que no está listo debe fallar: es la
    // condición de "publicación válida" de la sección 6.
    const publicacion = await request.post(
      `${API}/api/v1/courses/versions/${versionId}/publish`,
      { headers: conToken(token) },
    );
    expect(publicacion.status(), "no se publica con multimedia sin procesar").toBe(422);
    const motivos = JSON.stringify(await publicacion.json());
    expect(motivos.toLowerCase()).toMatch(/proces|disponible|listo/);
  });
});

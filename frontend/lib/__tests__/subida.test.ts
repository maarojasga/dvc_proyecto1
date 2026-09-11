import { describe, expect, it } from "vitest";
import { createHash, randomBytes } from "node:crypto";
import { sha256Hex, TAMANO_PARTE } from "@/lib/subida";

/**
 * El SHA-256 del navegador se compara contra el de Node, que es la referencia.
 *
 * Importa porque este hash es el contrato con el servidor: si difiere en un
 * bit, toda subida se rechaza por integridad y el fallo parece del servidor.
 * Se implementó a mano porque SubtleCrypto solo digiere de una vez, y un vídeo
 * de curso no cabe en memoria.
 */
function referencia(b: Uint8Array): string {
  return createHash("sha256").update(b).digest("hex");
}

/** archivoDe finge un File a partir de bytes; solo se usan slice y size. */
function archivoDe(bytes: Uint8Array): File {
  // Se copia a un ArrayBuffer propio: el tipo de File no acepta una vista
  // sobre un SharedArrayBuffer, que es lo que Uint8Array admite en general.
  const buf = new ArrayBuffer(bytes.length);
  new Uint8Array(buf).set(bytes);
  return new File([buf], "material.bin", { lastModified: 0 });
}

describe("sha256Hex", () => {
  it("coincide con Node para el archivo vacío", async () => {
    const vacio = new Uint8Array(0);
    expect(await sha256Hex(archivoDe(vacio))).toBe(referencia(vacio));
  });

  it("coincide con Node para contenido corto", async () => {
    const b = new TextEncoder().encode("abc");
    expect(await sha256Hex(archivoDe(b))).toBe(referencia(b));
  });

  // Los tamaños alrededor del bloque de 64 bytes y del relleno de 56 son
  // donde una implementación de SHA-256 se rompe.
  it.each([1, 55, 56, 57, 63, 64, 65, 119, 120, 121, 127, 128, 129])(
    "coincide con Node para %i bytes",
    async (n) => {
      const b = new Uint8Array(n);
      for (let i = 0; i < n; i++) b[i] = i % 256;
      expect(await sha256Hex(archivoDe(b))).toBe(referencia(b));
    },
  );

  // Más de un trozo de lectura: es el caso que ejercita el acumulado entre
  // llamadas a update, que es lo propio de esta implementación.
  it("coincide con Node cruzando varios trozos de lectura", async () => {
    const b = randomBytes(4 * 1024 * 1024 + 12345);
    const bytes = new Uint8Array(b);
    expect(await sha256Hex(archivoDe(bytes))).toBe(referencia(bytes));
  }, 30_000);

  it("coincide con Node para un tamaño no alineado a la parte multipart", async () => {
    const bytes = new Uint8Array(randomBytes(TAMANO_PARTE + 1));
    expect(await sha256Hex(archivoDe(bytes))).toBe(referencia(bytes));
  }, 30_000);

  it("informa del avance hasta el total", async () => {
    const bytes = new Uint8Array(randomBytes(5 * 1024 * 1024));
    const avances: number[] = [];
    await sha256Hex(archivoDe(bytes), (leido) => avances.push(leido));
    expect(avances.length).toBeGreaterThan(1);
    expect(avances.at(-1)).toBe(bytes.length);
    // monótono creciente
    expect([...avances].sort((a, b) => a - b)).toEqual(avances);
  }, 30_000);
});

import { api, ApiError } from "@/lib/api";

/**
 * Subida de material de curso: directa al almacén de objetos, en partes, y
 * reanudable durante 24 horas.
 *
 * Las partes no pasan por la API —la API solo firma— así que el progreso que
 * se muestra es el real de la transferencia. El SHA-256 se calcula aquí, en el
 * navegador, y el servidor lo verifica contra lo que ensambló: es la única
 * forma de que "verificación de integridad" signifique algo, porque un
 * checksum que calcula el mismo que guarda el archivo no comprueba nada.
 */

/** TAMANO_PARTE es el mínimo que S3 admite salvo en la última parte. */
export const TAMANO_PARTE = 5 * 1024 * 1024;

/** Debajo de este tamaño no merece la pena el ciclo multipart. */
const UMBRAL_MULTIPART = TAMANO_PARTE;

export interface ProgresoSubida {
  subido: number;
  total: number;
  /** parte que se está transfiriendo, 1-indexada; 0 mientras se prepara. */
  parte: number;
  partes: number;
}

export interface OpcionesSubida {
  versionId: string;
  resourceId: string;
  file: File;
  onProgreso?: (p: ProgresoSubida) => void;
  /** señal para cancelar; la reanudación posterior reaprovecha lo subido. */
  signal?: AbortSignal;
}

/**
 * sha256Hex calcula el hash del archivo leyéndolo por trozos.
 *
 * No se usa crypto.subtle.digest sobre el archivo entero a propósito: eso
 * exige cargarlo en memoria, y un vídeo de curso puede no caber. Se acumula
 * con una implementación incremental porque SubtleCrypto no ofrece digest por
 * trozos.
 */
export async function sha256Hex(file: File, onAvance?: (leido: number) => void): Promise<string> {
  const hasher = new Sha256Incremental();
  const TROZO = 4 * 1024 * 1024;
  for (let offset = 0; offset < file.size; offset += TROZO) {
    const trozo = await file.slice(offset, Math.min(offset + TROZO, file.size)).arrayBuffer();
    hasher.update(new Uint8Array(trozo));
    onAvance?.(Math.min(offset + TROZO, file.size));
  }
  return hasher.hex();
}

/** subirMaterial sube el archivo y deja el recurso confirmado en la API. */
export async function subirMaterial(opts: OpcionesSubida): Promise<void> {
  const { versionId, resourceId, file, onProgreso, signal } = opts;

  // El hash va primero: si el archivo cambia bajo los pies (el usuario lo
  // edita mientras se sube) se detecta al confirmar, no a mitad.
  const checksum = await sha256Hex(file);

  if (file.size < UMBRAL_MULTIPART) {
    onProgreso?.({ subido: 0, total: file.size, parte: 1, partes: 1 });
    const { upload_url } = await api.requestUploadUrl(versionId, resourceId, file.type || "application/octet-stream");
    await ponerEnAlmacen(upload_url, file, signal);
    onProgreso?.({ subido: file.size, total: file.size, parte: 1, partes: 1 });
    await api.confirmUpload(versionId, resourceId, checksum);
    return;
  }

  const partes = Math.ceil(file.size / TAMANO_PARTE);
  const { upload_id } = await api.initiateMultipart(versionId, resourceId, file.type || "application/octet-stream");
  guardarCargaEnCurso(resourceId, upload_id, file);

  // Lo que el almacén ya tenga no se vuelve a subir. Con una carga nueva la
  // lista viene vacía; con una reanudada, no.
  const yaSubidas = await partesYaSubidas(versionId, resourceId, upload_id);
  const confirmadas: { part_number: number; etag: string }[] = [];
  let subido = 0;

  for (let n = 1; n <= partes; n++) {
    const previa = yaSubidas.get(n);
    if (previa) {
      confirmadas.push({ part_number: n, etag: previa.etag });
      subido += previa.size_bytes;
      onProgreso?.({ subido, total: file.size, parte: n, partes });
      continue;
    }
    onProgreso?.({ subido, total: file.size, parte: n, partes });
    const trozo = file.slice((n - 1) * TAMANO_PARTE, Math.min(n * TAMANO_PARTE, file.size));
    const { upload_url } = await api.getMultipartPartUrl(versionId, resourceId, upload_id, n);
    const etag = await ponerEnAlmacen(upload_url, trozo, signal);
    confirmadas.push({ part_number: n, etag });
    subido += trozo.size;
    onProgreso?.({ subido, total: file.size, parte: n, partes });
  }

  await api.completeMultipart(versionId, resourceId, upload_id, confirmadas, checksum);
  olvidarCargaEnCurso(resourceId);
}

async function partesYaSubidas(versionId: string, resourceId: string, uploadId: string) {
  const mapa = new Map<number, { size_bytes: number; etag: string }>();
  try {
    const { parts } = await api.listMultipartParts(versionId, resourceId, uploadId);
    for (const p of parts ?? []) {
      mapa.set(p.part_number, { size_bytes: p.size_bytes, etag: p.etag });
    }
  } catch (e) {
    // Que no se pueda consultar no debe impedir subir: se sube todo.
    if (!(e instanceof ApiError)) throw e;
  }
  return mapa;
}

/**
 * ponerEnAlmacen hace el PUT a la URL prefirmada y devuelve el ETag, que el
 * cierre de la carga necesita para cada parte.
 *
 * El ETag viaja en una cabecera de respuesta, así que el almacén tiene que
 * exponerla por CORS (Access-Control-Expose-Headers: ETag). Sin eso la lectura
 * devuelve null y el cierre falla, que es un fallo difícil de diagnosticar: el
 * PUT sale con 200.
 */
async function ponerEnAlmacen(url: string, cuerpo: Blob, signal?: AbortSignal): Promise<string> {
  let res: Response;
  try {
    res = await fetch(url, { method: "PUT", body: cuerpo, signal });
  } catch (e) {
    if (e instanceof DOMException && e.name === "AbortError") throw e;
    const host = (() => {
      try {
        return new URL(url).host;
      } catch {
        return url;
      }
    })();
    throw new Error(
      `No se pudo contactar con el almacenamiento en ${host}. Revisa que ese host sea alcanzable desde el navegador (S3_PUBLIC_ENDPOINT) y que permita peticiones desde este origen.`,
    );
  }
  if (!res.ok) {
    throw new Error(`El almacenamiento rechazó la subida (HTTP ${res.status}).`);
  }
  const etag = res.headers.get("ETag") ?? res.headers.get("etag");
  if (!etag) {
    throw new Error(
      "El almacenamiento no expuso la cabecera ETag. Añade ETag a Access-Control-Expose-Headers en el bucket o el CDN.",
    );
  }
  return etag.replaceAll('"', "");
}

// ---------------------------------------------------------------------------
// Recuerdo de cargas a medias
// ---------------------------------------------------------------------------

/**
 * El upload_id se guarda en el navegador para poder reanudar tras recargar la
 * página. Se guarda también nombre, tamaño y fecha del archivo: reanudar con
 * un archivo distinto produciría un objeto mezcla de dos, y el checksum lo
 * detectaría al final, pero es mejor no llegar ahí.
 */
const CLAVE = "mooc_cargas_en_curso";
const VIGENCIA_MS = 24 * 60 * 60 * 1000;

interface CargaEnCurso {
  uploadId: string;
  nombre: string;
  tamano: number;
  modificado: number;
  iniciada: number;
}

function leerCargas(): Record<string, CargaEnCurso> {
  try {
    return JSON.parse(localStorage.getItem(CLAVE) ?? "{}");
  } catch {
    return {};
  }
}

function escribirCargas(v: Record<string, CargaEnCurso>) {
  try {
    localStorage.setItem(CLAVE, JSON.stringify(v));
  } catch {
    // Sin almacenamiento local se pierde la reanudación, no la subida.
  }
}

function guardarCargaEnCurso(resourceId: string, uploadId: string, file: File) {
  const cargas = leerCargas();
  cargas[resourceId] = {
    uploadId,
    nombre: file.name,
    tamano: file.size,
    modificado: file.lastModified,
    iniciada: Date.now(),
  };
  escribirCargas(cargas);
}

function olvidarCargaEnCurso(resourceId: string) {
  const cargas = leerCargas();
  delete cargas[resourceId];
  escribirCargas(cargas);
}

/**
 * cargaReanudable devuelve el upload_id de una carga a medias de este mismo
 * archivo, si sigue dentro de las 24 horas que el almacén la conserva.
 */
export function cargaReanudable(resourceId: string, file: File): string | null {
  const c = leerCargas()[resourceId];
  if (!c) return null;
  if (Date.now() - c.iniciada > VIGENCIA_MS) {
    olvidarCargaEnCurso(resourceId);
    return null;
  }
  const mismoArchivo = c.nombre === file.name && c.tamano === file.size && c.modificado === file.lastModified;
  return mismoArchivo ? c.uploadId : null;
}

// ---------------------------------------------------------------------------
// SHA-256 incremental
// ---------------------------------------------------------------------------

/**
 * SubtleCrypto solo digiere de una vez, y cargar el archivo entero en memoria
 * no es opción para vídeo. Esta es la implementación del estándar FIPS 180-4,
 * que permite alimentarla por trozos.
 */
class Sha256Incremental {
  private static readonly K = new Uint32Array([
    0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5, 0xd807aa98,
    0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174, 0xe49b69c1, 0xefbe4786,
    0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da, 0x983e5152, 0xa831c66d, 0xb00327c8,
    0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967, 0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13,
    0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85, 0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819,
    0xd6990624, 0xf40e3585, 0x106aa070, 0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a,
    0x5b9cca4f, 0x682e6ff3, 0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7,
    0xc67178f2,
  ]);

  private h = new Uint32Array([
    0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19,
  ]);
  private resto = new Uint8Array(64);
  private restoLen = 0;
  private largoTotal = 0;
  private w = new Uint32Array(64);

  update(datos: Uint8Array) {
    this.largoTotal += datos.length;
    let i = 0;
    if (this.restoLen > 0) {
      const falta = 64 - this.restoLen;
      const toma = Math.min(falta, datos.length);
      this.resto.set(datos.subarray(0, toma), this.restoLen);
      this.restoLen += toma;
      i = toma;
      if (this.restoLen === 64) {
        this.bloque(this.resto, 0);
        this.restoLen = 0;
      }
    }
    for (; i + 64 <= datos.length; i += 64) {
      this.bloque(datos, i);
    }
    if (i < datos.length) {
      this.resto.set(datos.subarray(i), 0);
      this.restoLen = datos.length - i;
    }
  }

  hex(): string {
    // Relleno: un 0x80, ceros, y el largo en bits como entero de 64 bits.
    const bits = this.largoTotal * 8;
    const relleno = new Uint8Array(this.restoLen < 56 ? 64 : 128);
    relleno.set(this.resto.subarray(0, this.restoLen), 0);
    relleno[this.restoLen] = 0x80;
    const vista = new DataView(relleno.buffer);
    // El largo en bits de un archivo de navegador cabe de sobra en 53 bits, así
    // que se escribe como dos mitades de 32 sin perder precisión.
    vista.setUint32(relleno.length - 8, Math.floor(bits / 0x100000000));
    vista.setUint32(relleno.length - 4, bits >>> 0);
    for (let i = 0; i < relleno.length; i += 64) {
      this.bloque(relleno, i);
    }
    return Array.from(this.h, (x) => x.toString(16).padStart(8, "0")).join("");
  }

  private bloque(b: Uint8Array, off: number) {
    const { w, h } = this;
    const K = Sha256Incremental.K;
    for (let t = 0; t < 16; t++) {
      const i = off + t * 4;
      w[t] = ((b[i] << 24) | (b[i + 1] << 16) | (b[i + 2] << 8) | b[i + 3]) >>> 0;
    }
    for (let t = 16; t < 64; t++) {
      const s0 = (rotr(w[t - 15], 7) ^ rotr(w[t - 15], 18) ^ (w[t - 15] >>> 3)) >>> 0;
      const s1 = (rotr(w[t - 2], 17) ^ rotr(w[t - 2], 19) ^ (w[t - 2] >>> 10)) >>> 0;
      w[t] = (w[t - 16] + s0 + w[t - 7] + s1) >>> 0;
    }
    let [a, bb, c, d, e, f, g, hh] = h;
    for (let t = 0; t < 64; t++) {
      const S1 = (rotr(e, 6) ^ rotr(e, 11) ^ rotr(e, 25)) >>> 0;
      const ch = ((e & f) ^ (~e & g)) >>> 0;
      const t1 = (hh + S1 + ch + K[t] + w[t]) >>> 0;
      const S0 = (rotr(a, 2) ^ rotr(a, 13) ^ rotr(a, 22)) >>> 0;
      const maj = ((a & bb) ^ (a & c) ^ (bb & c)) >>> 0;
      const t2 = (S0 + maj) >>> 0;
      hh = g;
      g = f;
      f = e;
      e = (d + t1) >>> 0;
      d = c;
      c = bb;
      bb = a;
      a = (t1 + t2) >>> 0;
    }
    h[0] = (h[0] + a) >>> 0;
    h[1] = (h[1] + bb) >>> 0;
    h[2] = (h[2] + c) >>> 0;
    h[3] = (h[3] + d) >>> 0;
    h[4] = (h[4] + e) >>> 0;
    h[5] = (h[5] + f) >>> 0;
    h[6] = (h[6] + g) >>> 0;
    h[7] = (h[7] + hh) >>> 0;
  }
}

function rotr(x: number, n: number): number {
  return ((x >>> n) | (x << (32 - n))) >>> 0;
}

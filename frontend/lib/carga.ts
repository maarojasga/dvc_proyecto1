"use client";

import { api, type ParteCargada, type CargaVerificada } from "./api";

// Carga de archivos al almacén de objetos, reanudable.
//
// El alcance mínimo pide una carga multipart directa que se pueda reanudar
// durante 24 horas. "Reanudable" no lo da el endpoint por sí solo: si el
// navegador se cierra a mitad de la subida, el identificador de la carga se
// pierde y las partes que ya llegaron quedan huérfanas. Por eso el progreso
// vive en localStorage y, al volver, se le pregunta al servidor qué partes
// tiene antes de enviar nada.
//
// Lo que no se conserva es el archivo: ningún navegador permite releer un
// File de una sesión anterior. Al reanudar hay que volver a elegirlo, y de ahí
// la comprobación de tamaño y nombre antes de dar la carga por continuable.

const TAMANO_DE_PARTE = 8 * 1024 * 1024;

// El almacén de objetos exige que toda parte menos la última tenga al menos
// 5 MiB, así que por debajo de eso una carga multipart no es válida y se sube
// de una vez.
const MINIMO_MULTIPART = 5 * 1024 * 1024;

const PREFIJO = "mooc.carga.";

interface CargaEnCurso {
  uploadId: string;
  nombre: string;
  tamano: number;
  tamanoDeParte: number;
  iniciadaEn: number;
}

// Las URL prefirmadas caducan a las 24 horas; pasado ese plazo la carga ya no
// se puede continuar y el registro local solo estorba.
const VIGENCIA_MS = 24 * 60 * 60 * 1000;

function clave(resourceId: string) {
  return PREFIJO + resourceId;
}

function leerCarga(resourceId: string, archivo: File): CargaEnCurso | null {
  try {
    const crudo = localStorage.getItem(clave(resourceId));
    if (!crudo) return null;
    const carga = JSON.parse(crudo) as CargaEnCurso;
    const vigente = Date.now() - carga.iniciadaEn < VIGENCIA_MS;
    // Es el mismo archivo solo si coinciden nombre y tamaño: reanudar sobre
    // otro archivo produciría un objeto mezclado, peor que volver a empezar.
    const mismoArchivo = carga.nombre === archivo.name && carga.tamano === archivo.size;
    if (!vigente || !mismoArchivo) {
      olvidarCarga(resourceId);
      return null;
    }
    return carga;
  } catch {
    return null;
  }
}

function recordarCarga(resourceId: string, carga: CargaEnCurso) {
  try {
    localStorage.setItem(clave(resourceId), JSON.stringify(carga));
  } catch {
    // Sin localStorage se pierde la reanudación, no la subida.
  }
}

export function olvidarCarga(resourceId: string) {
  try {
    localStorage.removeItem(clave(resourceId));
  } catch {
    /* nada que limpiar */
  }
}

/** hayCargaPendiente informa si este recurso dejó una subida a medias. */
export function hayCargaPendiente(resourceId: string): boolean {
  try {
    const crudo = localStorage.getItem(clave(resourceId));
    if (!crudo) return false;
    const carga = JSON.parse(crudo) as CargaEnCurso;
    return Date.now() - carga.iniciadaEn < VIGENCIA_MS;
  } catch {
    return false;
  }
}

// checksum calcula el SHA-256 del archivo en el navegador.
//
// Es lo que el servidor contrasta contra el objeto ya almacenado: si una parte
// llegó truncada o alterada, los hashes no cuadran y la carga se rechaza en
// lugar de publicarse un archivo corrupto.
//
// crypto.subtle solo existe en contextos seguros (HTTPS o localhost). Donde no
// esté, se devuelve vacío y el servidor calcula su propio checksum igualmente:
// se pierde la comparación extremo a extremo, no la carga.
export async function checksum(archivo: File): Promise<string> {
  if (typeof crypto === "undefined" || !crypto.subtle) return "";
  try {
    const buffer = await archivo.arrayBuffer();
    const resumen = await crypto.subtle.digest("SHA-256", buffer);
    return Array.from(new Uint8Array(resumen))
      .map((b) => b.toString(16).padStart(2, "0"))
      .join("");
  } catch {
    return "";
  }
}

/**
 * FaseDeCarga es en qué punto está la subida.
 *
 * Se emite la fase y no el texto porque este módulo no es un componente y no
 * ve el idioma elegido: quien pinta la barra traduce la fase con el catálogo,
 * y así el progreso de una subida se lee en el mismo idioma que el resto de
 * la pantalla.
 */
export type FaseDeCarga =
  | "huella"
  | "preparando"
  | "subiendo"
  | "verificando"
  | "encolado"
  | "listo"
  | "retomando"
  | "iniciando"
  | "parteSubida"
  | "parteEnCurso";

export interface AvanceDeCarga {
  porcentaje: number;
  fase: FaseDeCarga;
  /** Número de parte y total, solo en las fases multipart. */
  numero?: number;
  total?: number;
}

/** ClaveDeErrorDeCarga identifica el fallo, por el mismo motivo que la fase. */
export type ClaveDeErrorDeCarga = "subida" | "parte" | "etag";

/**
 * ErrorDeCarga es un fallo de la subida con los datos para explicarlo.
 *
 * El mensaje que hereda de Error queda en la clave, que no se enseña: el texto
 * legible lo produce el catálogo a partir de `clave` y `datos`.
 */
export class ErrorDeCarga extends Error {
  constructor(
    readonly clave: ClaveDeErrorDeCarga,
    readonly datos: Record<string, string | number> = {},
  ) {
    super(clave);
    this.name = "ErrorDeCarga";
  }
}

export interface OpcionesDeCarga {
  versionId: string;
  resourceId: string;
  archivo: File;
  alAvanzar?: (avance: AvanceDeCarga) => void;
  senal?: AbortSignal;
}

/**
 * subirArchivo lleva el archivo al almacén y devuelve el resultado de la
 * verificación del servidor.
 *
 * Elige entre subida simple y multipart por tamaño, y en multipart reanuda
 * cualquier carga previa de este mismo archivo.
 */
export async function subirArchivo(opciones: OpcionesDeCarga): Promise<CargaVerificada> {
  const { versionId, resourceId, archivo, alAvanzar, senal } = opciones;
  const avisar: Avisar = (porcentaje, fase, partes) =>
    alAvanzar?.({ porcentaje, fase, ...partes });

  avisar(0, "huella");
  const huella = await checksum(archivo);

  if (archivo.size <= MINIMO_MULTIPART) {
    return subidaSimple({ versionId, resourceId, archivo, huella, avisar, senal });
  }
  return subidaMultipart({ versionId, resourceId, archivo, huella, avisar, senal });
}

interface Contexto {
  versionId: string;
  resourceId: string;
  archivo: File;
  huella: string;
  avisar: Avisar;
  senal?: AbortSignal;
}

type Avisar = (
  porcentaje: number,
  fase: FaseDeCarga,
  partes?: { numero: number; total: number },
) => void;

async function subidaSimple(ctx: Contexto): Promise<CargaVerificada> {
  const { versionId, resourceId, archivo, huella, avisar, senal } = ctx;
  avisar(10, "preparando");
  const { upload_url } = await api.requestUploadUrl(versionId, resourceId, archivo.type || "application/octet-stream");

  avisar(30, "subiendo");
  const res = await fetch(upload_url, { method: "PUT", body: archivo, signal: senal });
  if (!res.ok) throw new ErrorDeCarga("subida", { status: res.status });

  avisar(85, "verificando");
  const resultado = await api.confirmUpload(versionId, resourceId, huella);
  avisar(100, resultado.status === "queued" ? "encolado" : "listo");
  return resultado;
}

async function subidaMultipart(ctx: Contexto): Promise<CargaVerificada> {
  const { versionId, resourceId, archivo, huella, avisar, senal } = ctx;

  const previa = leerCarga(resourceId, archivo);
  let uploadId = previa?.uploadId ?? "";
  const tamanoDeParte = previa?.tamanoDeParte ?? TAMANO_DE_PARTE;
  const total = Math.ceil(archivo.size / tamanoDeParte);

  // Las partes que el almacén ya tiene. Al reanudar se le preguntan al
  // servidor en vez de fiarse del registro local, que puede estar desfasado.
  const recibidas = new Map<number, ParteCargada>();
  if (uploadId) {
    avisar(0, "retomando");
    try {
      const { parts } = await api.listMultipartParts(versionId, resourceId, uploadId);
      for (const p of parts) recibidas.set(p.part_number, p);
    } catch {
      // La carga caducó o ya no existe: se empieza de cero.
      uploadId = "";
      olvidarCarga(resourceId);
    }
  }

  if (!uploadId) {
    avisar(0, "iniciando");
    const inicio = await api.initiateMultipart(versionId, resourceId, archivo.type || "application/octet-stream");
    uploadId = inicio.upload_id;
    recordarCarga(resourceId, {
      uploadId,
      nombre: archivo.name,
      tamano: archivo.size,
      tamanoDeParte,
      iniciadaEn: Date.now(),
    });
  }

  for (let i = 0; i < total; i++) {
    const numero = i + 1;
    if (senal?.aborted) throw new DOMException("Subida cancelada", "AbortError");

    const porcentaje = Math.round((i / total) * 90);
    if (recibidas.has(numero)) {
      avisar(porcentaje, "parteSubida", { numero, total });
      continue;
    }

    avisar(porcentaje, "parteEnCurso", { numero, total });
    const trozo = archivo.slice(i * tamanoDeParte, Math.min((i + 1) * tamanoDeParte, archivo.size));
    const { upload_url } = await api.getMultipartPartUrl(versionId, resourceId, uploadId, numero);
    const res = await fetch(upload_url, { method: "PUT", body: trozo, signal: senal });
    if (!res.ok) throw new ErrorDeCarga("parte", { numero, status: res.status });

    // Sin el ETag real el almacén no puede ensamblar, y un valor inventado
    // haría fallar el cierre con un error mucho menos claro. En origen cruzado
    // el navegador solo lo expone si el almacén publica Access-Control-Expose-Headers.
    const etag = res.headers.get("ETag");
    if (!etag) {
      throw new ErrorDeCarga("etag");
    }
    recibidas.set(numero, { part_number: numero, etag });
  }

  avisar(95, "verificando");
  const partes = Array.from(recibidas.values()).sort((a, b) => a.part_number - b.part_number);
  const resultado = await api.completeMultipart(versionId, resourceId, uploadId, partes, huella);

  olvidarCarga(resourceId);
  avisar(100, resultado.status === "queued" ? "encolado" : "listo");
  return resultado;
}

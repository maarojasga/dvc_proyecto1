/**
 * Contratos de transporte de la API REST (/api/v1, OpenAPI 3.1).
 *
 * La API usa errores uniformes, paginacion por cursor, ETag e Idempotency-Key.
 */

/** Error uniforme estilo RFC 9457 (problem details). */
export interface ProblemDetails {
  type: string;
  title: string;
  status: number;
  detail?: string;
  instance?: string;
  /** Errores por campo en validaciones (por ejemplo, al publicar un curso). */
  errors?: Record<string, string[]>;
}

/** Coleccion paginada por cursor. */
export interface PaginaCursor<T> {
  items: T[];
  siguienteCursor: string | null;
}

export interface ParametrosPaginacion {
  cursor?: string;
  limite?: number;
}

export interface OpcionesPeticion extends RequestInit {
  /** Requerido en operaciones no idempotentes por naturaleza (envio de quiz, etc.). */
  idempotencyKey?: string;
  /** Para peticiones condicionales sobre recursos versionados. */
  etag?: string;
  query?: Record<string, string | number | boolean | undefined>;
}

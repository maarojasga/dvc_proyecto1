/**
 * Configuracion leida del entorno. Ver .env.example.
 *
 * En contenedor, las variables NEXT_PUBLIC_* se resuelven en tiempo de build,
 * por lo que se declaran aqui con valores por defecto de desarrollo local.
 */

function leer(nombre: string, porDefecto: string): string {
  const valor = process.env[nombre];
  return valor && valor.length > 0 ? valor : porDefecto;
}

export const env = {
  /** Base de la API en Go. El navegador la usa via el rewrite de next.config.ts. */
  apiBaseUrl: leer("NEXT_PUBLIC_API_BASE_URL", "/api/v1"),
  /** Base de la API vista desde el contenedor del frontend (SSR / route handlers). */
  apiInternalUrl: leer("API_INTERNAL_URL", "http://localhost:8080/api/v1"),
  /** Origen publico usado para construir URLs de verificacion de insignias. */
  siteUrl: leer("NEXT_PUBLIC_SITE_URL", "http://localhost:3000"),
} as const;

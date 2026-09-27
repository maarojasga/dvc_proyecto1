import { defineConfig } from "vitest/config";

// Las pruebas unitarias corren con vitest; las de extremo a extremo, con
// Playwright. Sin esta configuración vitest recoge también `e2e/*.spec.ts`,
// que es su patrón por defecto, e intenta ejecutarlas: fallan al arrancar
// porque importan `@playwright/test`, que no es un ejecutor compatible.
//
// El síntoma engaña: vitest informaba "29 passed" —las unitarias sí pasaban—
// junto a diez archivos en rojo, así que mirando solo el resumen de pruebas
// parecía correcto.
export default defineConfig({
  test: {
    include: ["**/*.{test,spec}.{ts,tsx}"],
    exclude: [
      "node_modules/**",
      ".next/**",
      // Territorio de Playwright: `npm run e2e`.
      "e2e/**",
      "playwright-report/**",
      "test-results/**",
    ],
  },
});

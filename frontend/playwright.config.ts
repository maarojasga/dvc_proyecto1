import { defineConfig, devices } from "@playwright/test";

/**
 * Pruebas E2E de los nueve flujos críticos (sección 10.2 del enunciado).
 *
 * La condición de aceptación dice que los nueve flujos "deben superar pruebas
 * E2E". La cobertura que ya existía —259 pruebas de backend y 29 de
 * frontend— llega hasta la API y hasta la conversión de Markdown, pero no
 * hasta el navegador: no acredita que un estudiante pueda inscribirse
 * pulsando un botón, solo que el endpoint responde. Esto cierra esa brecha.
 *
 * Las pruebas corren contra el sistema levantado, no contra dobles. Eso las
 * hace lentas y sensibles al entorno, y es a propósito: una prueba E2E que
 * simula el backend no demuestra nada que no demuestre ya una de integración.
 */

// El host importa y no es intercambiable: "localhost" y "127.0.0.1" son
// orígenes distintos para el navegador, así que una cookie emitida por uno no
// viaja al otro. Interfaz y API tienen que compartir host o el inicio de
// sesión falla de una forma que parece un error del producto.
const BASE_URL = process.env.E2E_BASE_URL ?? "http://localhost:3000";
const API_URL = process.env.E2E_API_URL ?? "http://localhost:8080";

export default defineConfig({
  testDir: "./e2e",
  // Los flujos tocan estado compartido (el catálogo, el curso sembrado), así
  // que se ejecutan en serie. Paralelizarlos daría carreras que se leerían
  // como fallos del producto y no del arnés.
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  // Un flujo completo incluye esperar a que el servidor califique un intento y
  // recalcule el progreso. 60 s es holgado en local y ajustado en CI.
  timeout: 60_000,
  expect: { timeout: 10_000 },

  reporter: process.env.CI
    ? [["list"], ["html", { open: "never", outputFolder: "playwright-report" }], ["json", { outputFile: "playwright-report/resultados.json" }]]
    : [["list"]],

  use: {
    baseURL: BASE_URL,
    // La evidencia que pide la sección 10.1 —"comportamiento de la interfaz"—
    // es justamente esto: traza y vídeo del primer reintento fallido.
    trace: "retain-on-failure",
    video: "retain-on-failure",
    screenshot: "only-on-failure",
    actionTimeout: 15_000,
    // La interfaz arranca en el idioma del navegador si la persona no eligió
    // ninguno. Se fija el español, que es el de origen, para que las pruebas
    // no dependan de cómo venga configurado el navegador del que ejecuta; el
    // cambio a inglés se prueba explícitamente en el segmento 5.
    locale: "es-ES",
  },

  projects: [
    {
      name: "chromium",
      use: {
        ...devices["Desktop Chrome"],
        // PLAYWRIGHT_CHROMIUM_PATH deja apuntar a un Chromium ya instalado.
        // Hace falta cuando la versión de @playwright/test no coincide con la
        // del navegador que trae la imagen: Playwright busca por número de
        // build y falla aunque haya un Chromium perfectamente usable al lado.
        launchOptions: process.env.PLAYWRIGHT_CHROMIUM_PATH
          ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_PATH }
          : {},
      },
    },
  ],

  metadata: { apiURL: API_URL },
});

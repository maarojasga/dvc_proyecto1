import { defineConfig, devices } from "@playwright/test";

/**
 * Pruebas de extremo a extremo de los flujos críticos y auditoría de
 * accesibilidad.
 *
 * La condición de aceptación del enunciado es que los nueve flujos críticos
 * pasen E2E y que la auditoría automática de accesibilidad no presente
 * incumplimientos críticos. Esto es eso, contra la pila desplegada.
 */
export default defineConfig({
  testDir: "./e2e",
  // Un solo trabajador: las pruebas comparten una base de datos, y en paralelo
  // se pisan los datos sembrados. Es más lento y es lo correcto.
  workers: 1,
  fullyParallel: false,
  // Sin reintentos en local; en CI uno, para no confundir una carrera del
  // arranque con un fallo del producto. Dos ya sería tapar problemas.
  retries: process.env.CI ? 1 : 0,
  timeout: 60_000,
  expect: { timeout: 10_000 },
  reporter: process.env.CI ? [["html", { open: "never" }], ["list"]] : [["list"]],
  use: {
    baseURL: process.env.E2E_BASE_URL ?? "http://localhost:3000",
    // La evidencia se guarda solo del primer reintento fallido: las trazas
    // pesan y lo que hace falta es la del fallo, no la de todo.
    // La traza ya incluye capturas paso a paso, que es la evidencia que pide
    // el enunciado. El vídeo se deja fuera porque exige el ffmpeg que trae
    // Playwright, que no está en todos los entornos, y aporta poco sobre la
    // traza.
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    // El navegador viene preinstalado en el entorno; en CI lo instala el
    // pipeline.
    launchOptions: process.env.PLAYWRIGHT_CHROMIUM
      ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM }
      : {},
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});

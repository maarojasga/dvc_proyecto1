import { defineConfig, devices } from "@playwright/test";
import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";

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

/**
 * Vuelca el .env de la raíz en process.env sin pisar lo que ya venga puesto.
 *
 * Playwright no lee .env por su cuenta. En CI las variables se inyectan a mano
 * (ver el paso "Los nueve flujos críticos" de .github/workflows/ci.yml), pero
 * en local nadie las exporta, y el mismo archivo que configura el compose ya
 * tiene las credenciales del administrador que la API siembra al arrancar.
 * Sin esto la suite entera falla con "faltan E2E_ADMIN_*" teniendo la
 * plataforma perfectamente levantada, que es un fallo del arnés leído como
 * fallo del producto.
 */
function cargarEnvDeLaRaiz() {
  // __dirname y no import.meta: Playwright transpila este archivo a CommonJS
  // antes de cargarlo, así que import.meta.url revienta al arrancar.
  const ruta = resolve(__dirname, "..", ".env");
  if (!existsSync(ruta)) return;
  for (const linea of readFileSync(ruta, "utf8").split("\n")) {
    const limpia = linea.trim();
    if (!limpia || limpia.startsWith("#")) continue;
    const corte = limpia.indexOf("=");
    if (corte <= 0) continue;
    const clave = limpia.slice(0, corte).trim();
    // Lo que ya esté en el entorno manda: exportar una variable en la línea
    // de órdenes tiene que seguir sirviendo para una ejecución puntual.
    if (process.env[clave] !== undefined) continue;
    // Las comillas pertenecen al formato del archivo, no al valor.
    process.env[clave] = limpia
      .slice(corte + 1)
      .trim()
      .replace(/^(['"])(.*)\1$/, "$2");
  }
}
cargarEnvDeLaRaiz();

// El administrador de las pruebas es el que la API siembra con ADMIN_EMAIL y
// ADMIN_PASSWORD. Se mantienen los nombres E2E_* porque CI usa credenciales
// propias y allí no hay .env que leer.
process.env.E2E_ADMIN_EMAIL ??= process.env.ADMIN_EMAIL;
process.env.E2E_ADMIN_PASSWORD ??= process.env.ADMIN_PASSWORD;
// El límite que la suite da por bueno tiene que ser el que aplica la API. Si
// se desincronizan, 99-limite-de-tasa da por agotado un presupuesto que sigue
// abierto —o al revés— y el fallo no apunta a ningún sitio.
process.env.E2E_AUTH_RATE_LIMIT ??= process.env.AUTH_RATE_LIMIT_PER_MINUTE;

// El host importa y no es intercambiable: "localhost" y "127.0.0.1" son
// orígenes distintos para el navegador, así que una cookie emitida por uno no
// viaja al otro. Interfaz y API tienen que compartir host o el inicio de
// sesión falla de una forma que parece un error del producto.
const BASE_URL = process.env.E2E_BASE_URL ?? "http://localhost:3000";
const API_URL = process.env.E2E_API_URL ?? "http://localhost:8080";

export default defineConfig({
  testDir: "./e2e",
  // Los flujos tocan estado compartido (el catálogo, el curso sembrado, y el
  // presupuesto del limitador de tasa, que se cuenta por IP), así que se
  // ejecutan en serie y en el orden del nombre del archivo. Paralelizarlos
  // daría carreras que se leerían como fallos del producto y no del arnés.
  //
  // El orden importa: 99-limite-de-tasa agota a propósito el presupuesto de
  // autenticación, así que va numerado para caer el último. Por eso los
  // archivos llevan prefijo, incluido 10-accesibilidad.
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

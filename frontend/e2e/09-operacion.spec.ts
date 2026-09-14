import { test, expect } from "@playwright/test";
import { API, conToken, ipDePrueba, registrarEstudiante, sufijo, tokenDeSesion } from "./ayudas";

/**
 * Segmento 9 — Operación.
 *
 * Criterio: arquitectura y calidad operativa. La evidencia principal de este
 * segmento es la prueba de carga (`load/etapa1.js`), que mide p95 y
 * escalamiento y no cabe en Playwright.
 *
 * Lo que sí se acredita aquí es la premisa sin la cual escalar no significa
 * nada: que la API es **sin estado**. Si una sesión solo valiera contra la
 * instancia que la creó, `--scale api=3` repartiría usuarios y la mitad se
 * encontraría deslogueada.
 */
test.describe("9. Operación", () => {
  test("la salud del servicio se puede sondear sin sesión", async ({ request }) => {
    const salud = await request.get(`${API}/api/v1/health`);
    expect(salud.ok(), "el sondeo de salud no exige credenciales").toBeTruthy();
    expect((await salud.json()).status).toBe("ok");
  });

  test("la sesión no vive en la memoria del proceso: cualquier instancia la resuelve", async ({
    request,
  }) => {
    const cuenta = await registrarEstudiante(request, sufijo());
    const token = await tokenDeSesion(request, cuenta.email, cuenta.password);

    // Detrás del proxy, peticiones sucesivas pueden caer en instancias
    // distintas. Todas deben resolver la misma sesión, porque vive en
    // Postgres y no en el proceso.
    for (let i = 0; i < 8; i++) {
      const yo = await request.get(`${API}/api/v1/auth/me`, { headers: conToken(token) });
      expect(yo.ok(), `petición ${i + 1}: la sesión se resuelve en cualquier instancia`).toBeTruthy();
      expect((await yo.json()).email).toBe(cuenta.email);
    }
  });

  test("cada respuesta trae un identificador para correlacionar los registros", async ({
    request,
  }) => {
    // Es la evidencia que pide la sección 10.1: "trazas o logs correlacionados".
    const res = await request.get(`${API}/api/v1/catalog`);
    const id = res.headers()["x-request-id"];
    expect(id, "la respuesta trae X-Request-Id").toBeTruthy();

    const otra = await request.get(`${API}/api/v1/catalog`);
    expect(
      otra.headers()["x-request-id"],
      "y es distinto en cada petición, que es lo que lo hace útil",
    ).not.toBe(id);
  });

  test("el límite de tasa protege la autenticación por IP", async ({ request }) => {
    // Se ataca el restablecimiento de contraseña y no el login, aunque cuenten
    // en el mismo cubo ("auth"): verificar una contraseña cuesta un Argon2id
    // deliberadamente lento, y agotar el presupuesto a golpe de login mediría
    // el coste del hash en vez del limitador.
    //
    // Con una IP propia, esta prueba corre contra el límite real de producción
    // y no contra uno inflado para que la suite quepa.
    const email = `inexistente-${sufijo()}@e2e.local`;
    const ip = ipDePrueba();
    const limite = Number(process.env.E2E_AUTH_RATE_LIMIT ?? 10);

    let limitado = false;
    for (let i = 0; i < limite + 5 && !limitado; i++) {
      const res = await request.post(`${API}/api/v1/auth/password/reset-request`, {
        headers: { "X-Forwarded-For": ip },
        data: { email },
      });
      limitado = res.status() === 429;
    }

    expect(limitado, `insistir por encima de ${limite}/min desde la misma IP acaba en 429`).toBeTruthy();

    // Y el límite es por IP, no global: otra dirección sigue pudiendo entrar.
    const otra = await request.post(`${API}/api/v1/auth/password/reset-request`, {
      headers: { "X-Forwarded-For": ipDePrueba() },
      data: { email },
    });
    expect(otra.status(), "otra IP no arrastra el castigo de la primera").not.toBe(429);
  });
});

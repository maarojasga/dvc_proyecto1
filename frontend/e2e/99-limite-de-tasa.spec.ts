import { test, expect } from "@playwright/test";
import { API, sufijo } from "./ayudas";

/**
 * El límite de tasa de autenticación (segmento 9, y condición de la sección 7).
 *
 * Va en su propio archivo y numerado 99 por una razón de funcionamiento, no de
 * orden estético: esta prueba **agota a propósito** el presupuesto del
 * limitador, que se cuenta por IP y por minuto. Toda la suite sale de la misma
 * IP, así que cualquier prueba que corriera después se encontraría con un 429
 * que no tiene nada que ver con lo que estaba probando. Playwright ordena los
 * archivos por nombre, y este va el último.
 *
 * Hubo una versión que inyectaba un X-Forwarded-For distinto por prueba para
 * evitarlo. Funcionaba contra la API directamente y dejó de funcionar en
 * cuanto las pruebas pasaron por el proxy, porque nginx sobrescribe esa
 * cabecera con la dirección real del cliente. Es el control que impide saltarse
 * el límite rotando una cabecera: que estorbe a las pruebas es la señal de que
 * funciona.
 */
test.describe("Límite de tasa de autenticación", () => {
  test("insistir desde la misma IP acaba en 429", async ({ request }) => {
    // Se ataca el restablecimiento de contraseña y no el inicio de sesión,
    // aunque cuenten en el mismo cubo ("auth"): verificar una contraseña
    // cuesta un Argon2id deliberadamente lento, y agotar el presupuesto a
    // golpe de login mediría el coste del hash en vez del limitador.
    const email = `inexistente-${sufijo()}@e2e.local`;
    const limite = Number(process.env.E2E_AUTH_RATE_LIMIT ?? 10);

    // En tandas: en serie, agotar un presupuesto alto tardaría más que el
    // propio minuto de la ventana y la prueba se mordería la cola.
    const tanda = 25;
    let limitado = false;
    let enviadas = 0;
    for (; enviadas < limite + tanda * 2 && !limitado; enviadas += tanda) {
      const respuestas = await Promise.all(
        Array.from({ length: tanda }, () =>
          request.post(`${API}/api/v1/auth/password/reset-request`, { data: { email } }),
        ),
      );
      limitado = respuestas.some((r) => r.status() === 429);
    }

    expect(
      limitado,
      `insistir por encima de ${limite}/min desde la misma IP acaba en 429 ` +
        `(se enviaron ${enviadas} peticiones)`,
    ).toBeTruthy();

    // Que el límite sea POR IP y no global no se puede comprobar desde aquí:
    // el proxy sustituye X-Forwarded-For por la dirección real, así que todas
    // las peticiones de esta suite son la misma IP hagamos lo que hagamos. Esa
    // parte la cubren las pruebas de integración del backend, que hablan con
    // el manejador sin proxy delante.
  });
});

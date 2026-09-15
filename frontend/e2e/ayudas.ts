import { expect, request as clienteHTTP, type APIRequestContext, type Page } from "@playwright/test";

export const API = process.env.E2E_API_URL ?? "http://localhost:8080";
export const MAILPIT = process.env.E2E_MAILPIT_URL ?? "http://localhost:8025";

/**
 * Cada ejecución usa un sufijo propio. Sin esto, dos corridas seguidas
 * chocarían con el slug del curso o con el correo del estudiante, y el
 * segundo fallo se leería como un error del producto.
 */
export function sufijo(): string {
  return `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
}

/**
 * Nota sobre el límite de tasa y por qué esta suite lo sube.
 *
 * El limitador de autenticación cuenta por IP, y toda la suite sale de una
 * sola. Hubo una versión de estos ayudantes que inyectaba un X-Forwarded-For
 * distinto en cada llamada para no compartir presupuesto; funcionaba contra la
 * API directamente y dejó de funcionar en cuanto las pruebas pasaron por el
 * proxy, porque nginx sobrescribe esa cabecera con la dirección real del
 * cliente. Eso no es un estorbo: es el control que impide que cualquiera se
 * salte el límite rotando una cabecera, y quitarlo para que la suite quepa
 * sería desactivar lo que se quiere probar.
 *
 * Así que la suite corre con AUTH_RATE_LIMIT_PER_MINUTE elevado y lo dice.
 * Cien inicios de sesión por minuto desde una misma máquina es un artefacto
 * de las pruebas, no un patrón de uso real. El limitador se ejercita aparte,
 * en 99-limite-de-tasa.spec.ts, que corre el último porque agota el
 * presupuesto a propósito.
 */

export interface Cuenta {
  email: string;
  password: string;
  fullName: string;
}

export const CLAVE_VALIDA = "e2e-clave-larga-2026";

/**
 * verificarPorCorreo saca el enlace de verificación del correo que la
 * plataforma acaba de enviar.
 *
 * Se lee de Mailpit y no se genera el token por dentro a propósito: el
 * segmento 1 pide demostrar el registro con su correo de confirmación, y un
 * atajo que inserte el usuario ya verificado no probaría que ese correo se
 * envía ni que el enlace funciona.
 */
export async function enlaceDeVerificacion(
  request: APIRequestContext,
  email: string,
): Promise<string> {
  // Mailpit indexa con un pequeño retardo tras aceptar el SMTP.
  for (let intento = 0; intento < 20; intento++) {
    const res = await request.get(`${MAILPIT}/api/v1/search?query=${encodeURIComponent(`to:${email}`)}`);
    if (res.ok()) {
      const datos = await res.json();
      const mensajes = datos.messages ?? [];
      if (mensajes.length > 0) {
        const detalle = await request.get(`${MAILPIT}/api/v1/message/${mensajes[0].ID}`);
        const cuerpo = await detalle.json();
        const texto = `${cuerpo.Text ?? ""}${cuerpo.HTML ?? ""}`;
        const url = texto.match(/https?:\/\/[^\s"'<>]*token=[A-Za-z0-9._-]+/);
        if (url) return url[0];
      }
    }
    await new Promise((r) => setTimeout(r, 500));
  }
  throw new Error(`no llegó el correo de verificación para ${email}`);
}

/** registrarEstudiante hace el alta por la API y confirma el correo. */
export async function registrarEstudiante(
  request: APIRequestContext,
  marca: string,
): Promise<Cuenta> {
  const cuenta: Cuenta = {
    email: `estudiante-${marca}@e2e.local`,
    password: CLAVE_VALIDA,
    fullName: `Estudiante ${marca}`,
  };
  const alta = await request.post(`${API}/api/v1/auth/register`, {
    data: { email: cuenta.email, password: cuenta.password, full_name: cuenta.fullName },
  });
  expect(alta.status(), "el registro público debe aceptarse").toBe(202);

  const enlace = await enlaceDeVerificacion(request, cuenta.email);
  const token = new URL(enlace).searchParams.get("token");
  const verificacion = await request.post(`${API}/api/v1/auth/verify-email`, {
    data: { token },
  });
  expect(verificacion.ok(), "el enlace del correo debe verificar la cuenta").toBeTruthy();
  return cuenta;
}

/**
 * tokenDeSesion abre una sesión y devuelve su token.
 *
 * Cada llamada usa un contexto HTTP recién creado, y eso no es una manía: el
 * contexto compartido de Playwright arrastra las cookies, así que un segundo
 * login llegaría con la cookie de sesión del primero y sin la cabecera
 * X-CSRF-Token. El middleware CSRF lo rechazaría con un 403 —correctamente,
 * porque eso es exactamente lo que debe rechazar— y el fallo se leería como
 * un error del producto. Un contexto por sesión es además lo fiel: representa
 * un dispositivo distinto.
 *
 * Las peticiones posteriores usan la cabecera Authorization (ver conToken),
 * que no está expuesta a CSRF porque el navegador no la añade sola. El inicio
 * de sesión por la interfaz, con cookies y todo, se prueba en el segmento 1.
 */
export async function tokenDeSesion(
  _compartido: APIRequestContext,
  email: string,
  password: string,
): Promise<string> {
  const ctx = await clienteHTTP.newContext();
  try {
    const res = await ctx.post(`${API}/api/v1/auth/login`, {
        data: { email, password },
    });
    expect(res.ok(), `login de ${email}: ${res.status()} ${await res.text()}`).toBeTruthy();
    const sesion = res
      .headersArray()
      .filter((h) => h.name.toLowerCase() === "set-cookie")
      .map((c) => c.value)
      .find((v) => v.startsWith("mooc_session="));
    if (!sesion) throw new Error("el login no devolvió la cookie de sesión");
    return sesion.split(";")[0].split("=")[1];
  } finally {
    await ctx.dispose();
  }
}

export function conToken(token: string) {
  return { Authorization: `Bearer ${token}`, "Content-Type": "application/json" };
}

/** entrarPorLaInterfaz inicia sesión pulsando, que es lo que hace un usuario. */
export async function entrarPorLaInterfaz(page: Page, cuenta: Cuenta) {
  // Aquí NO se inyecta X-Forwarded-For, al contrario que en los ayudantes de
  // API. Una cabecera propia en una petición de origen cruzado obliga al
  // navegador a hacer preflight, y la API tendría que declararla como
  // permitida en Access-Control-Allow-Headers: eso dejaría que cualquier
  // página del mundo dijera desde qué IP viene, que es justo lo que el
  // limitador no debe creerse. Por eso el inicio de sesión por la interfaz sí
  // consume el presupuesto real, y la suite levanta el límite lo justo (ver
  // AUTH_RATE_LIMIT_PER_MINUTE en el arranque de la API para las pruebas).
  await page.goto("/login");
  await page.getByLabel(/correo electrónico|email address/i).fill(cuenta.email);
  await page.getByLabel(/contraseña|password/i).fill(cuenta.password);
  await page.getByRole("button", { name: /ingresar|sign in/i }).click();
  await expect(page.getByRole("button", { name: /salir|sign out/i })).toBeVisible();
}

/**
 * tokenDeAdmin devuelve la sesión del administrador, abriéndola una sola vez.
 *
 * Casi todas las pruebas necesitan al administrador para preparar su
 * escenario. Autenticarlo en cada una gastaba veintitantos inicios de sesión
 * del presupuesto del limitador sin probar nada: el inicio de sesión ya tiene
 * sus propias pruebas en el segmento 1. Una sesión vale para toda la suite
 * porque es revocable, no de un solo uso.
 */
let sesionDeAdmin: Promise<string> | null = null;

export function tokenDeAdmin(request: APIRequestContext): Promise<string> {
  if (!sesionDeAdmin) {
    const admin = credencialesDeAdmin();
    sesionDeAdmin = tokenDeSesion(request, admin.email, admin.password).catch((e) => {
      // Una promesa rechazada en la caché envenenaría el resto de la suite:
      // toda prueba posterior heredaría este fallo sin volver a intentarlo, y
      // el reintento de Playwright tampoco serviría de nada. Se limpia para
      // que el siguiente uso vuelva a autenticar.
      sesionDeAdmin = null;
      throw e;
    });
  }
  return sesionDeAdmin;
}

/**
 * credencialesDeAdmin son las del administrador sembrado por ADMIN_EMAIL /
 * ADMIN_PASSWORD. Sin ellas no se puede probar la administración, así que la
 * prueba falla en vez de saltarse el segmento en silencio.
 */
export function credencialesDeAdmin(): Cuenta {
  const email = process.env.E2E_ADMIN_EMAIL;
  const password = process.env.E2E_ADMIN_PASSWORD;
  if (!email || !password) {
    throw new Error(
      "faltan E2E_ADMIN_EMAIL y E2E_ADMIN_PASSWORD: el administrador se siembra con ADMIN_EMAIL/ADMIN_PASSWORD al arrancar la API",
    );
  }
  return { email, password, fullName: "Administrador" };
}

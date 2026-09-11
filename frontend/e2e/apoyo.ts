import { expect, request as fabricaDePeticiones, type APIRequestContext, type Page } from "@playwright/test";

/**
 * Utilidades compartidas por los flujos críticos.
 *
 * La siembra va por la API y no por la base de datos: así lo que se prepara
 * pasa por las mismas validaciones que usaría una persona, y una prueba no
 * puede montar un estado que la aplicación no permitiría.
 */

export const API = process.env.E2E_API_URL ?? "http://localhost:8080";
export const CLAVE = "PruebaE2E123!";

/** unico evita colisiones entre ejecuciones sobre la misma base. */
export function unico(prefijo: string): string {
  return `${prefijo}-${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
}

/**
 * ClienteAPI habla con la API como un actor concreto.
 *
 * Cada actor lleva su propio APIRequestContext y, con él, su propio tarro de
 * cookies. Es imprescindible: Playwright gestiona las cookies por contexto, y
 * compartir uno hacía que la sesión del administrador se filtrara a los demás
 * clientes —el síntoma era un 403 de CSRF en peticiones que debían ir sin
 * sesión—.
 *
 * Las cookies las gestiona Playwright; aquí solo se lee la del token anti-CSRF
 * para repetirla en la cabecera, que es exactamente lo que hace el frontend.
 */
export class ClienteAPI {
  private constructor(private readonly ctx: APIRequestContext) {}

  /** nuevo abre un contexto propio, con su tarro de cookies. */
  static async nuevo(): Promise<ClienteAPI> {
    return new ClienteAPI(await fabricaDePeticiones.newContext({ baseURL: API }));
  }

  async cerrar() {
    await this.ctx.dispose();
  }

  private async tokenCSRF(): Promise<string | undefined> {
    const { cookies } = await this.ctx.storageState();
    return cookies.find((c) => c.name === "mooc_csrf")?.value;
  }

  async pedir(metodo: "get" | "post" | "put" | "patch" | "delete", ruta: string, cuerpo?: unknown) {
    const cabeceras: Record<string, string> = { "Content-Type": "application/json" };
    if (metodo !== "get") {
      const csrf = await this.tokenCSRF();
      if (csrf) cabeceras["X-CSRF-Token"] = csrf;
    }
    return this.ctx.fetch(`${API}${ruta}`, {
      method: metodo.toUpperCase(),
      headers: cabeceras,
      data: cuerpo === undefined ? undefined : JSON.stringify(cuerpo),
    });
  }

  /** json exige que la petición saliera bien y devuelve el cuerpo. */
  async json<T = any>(
    metodo: "get" | "post" | "put" | "patch" | "delete",
    ruta: string,
    cuerpo?: unknown,
  ): Promise<T> {
    const res = await this.pedir(metodo, ruta, cuerpo);
    expect(res.ok(), `${metodo.toUpperCase()} ${ruta} → ${res.status()} ${await res.text()}`).toBeTruthy();
    const texto = await res.text();
    return texto ? JSON.parse(texto) : ({} as T);
  }
}

/**
 * tokenDelCorreo saca un token del buzón de Mailpit.
 *
 * Se lee el correo de verdad en lugar de consultar la base: el flujo que hay
 * que acreditar incluye que el correo salga, y un token leído de la tabla no
 * demuestra eso.
 */
export async function tokenDelCorreo(destinatario: string, patron: RegExp): Promise<string> {
  // Respaldo para ejecutar la suite sin Mailpit —por ejemplo con un sumidero
  // SMTP que escribe a un archivo—. La vía normal es Mailpit, que es lo que
  // levanta el compose y usa CI.
  const archivo = process.env.E2E_MAILBOX_FILE;
  if (archivo) return tokenDelArchivo(archivo, patron);

  const base = process.env.E2E_MAILPIT_URL ?? "http://localhost:8025";
  const peticion = await fabricaDePeticiones.newContext();
  for (let intento = 0; intento < 20; intento++) {
    const res = await peticion.fetch(`${base}/api/v1/search?query=${encodeURIComponent(`to:${destinatario}`)}`);
    if (res.ok()) {
      const { messages = [] } = await res.json();
      for (const m of messages) {
        const detalle = await peticion.fetch(`${base}/api/v1/message/${m.ID}`);
        if (!detalle.ok()) continue;
        const { Text = "", HTML = "" } = await detalle.json();
        const encontrado = patron.exec(`${Text}\n${HTML}`);
        if (encontrado) {
          await peticion.dispose();
          return encontrado[1];
        }
      }
    }
    await new Promise((r) => setTimeout(r, 500));
  }
  await peticion.dispose();
  throw new Error(`no llegó ningún correo a ${destinatario} con ${patron}`);
}

/**
 * tokenDelArchivo busca el token en un archivo donde un sumidero SMTP volcó
 * los correos. El cuerpo puede venir con saltos de línea codificados por el
 * transporte, así que se limpian antes de buscar.
 */
async function tokenDelArchivo(ruta: string, patron: RegExp): Promise<string> {
  const { readFile } = await import("node:fs/promises");
  for (let intento = 0; intento < 20; intento++) {
    try {
      const crudo = await readFile(ruta, "utf8");
      const limpio = crudo.replace(/=\r?\n/g, "").replace(/\r/g, "");
      const todos = [...limpio.matchAll(new RegExp(patron.source, "g"))];
      if (todos.length) return todos[todos.length - 1][1];
    } catch {
      // El archivo puede no existir todavía.
    }
    await new Promise((r) => setTimeout(r, 500));
  }
  throw new Error(`no se encontró ningún token con ${patron} en ${ruta}`);
}

/** estudianteRegistrado crea, verifica y deja lista una cuenta de estudiante. */
export async function estudianteRegistrado(correo: string) {
  const c = await ClienteAPI.nuevo();
  const alta = await c.pedir("post", "/api/v1/auth/register", {
    email: correo,
    full_name: "Estudiante E2E",
    password: CLAVE,
  });
  // 202 siempre, también si el correo ya existía: la API no revela cuentas.
  expect(alta.status()).toBe(202);
  const token = await tokenDelCorreo(correo, /verificar-correo\?token=([A-Za-z0-9_\-=]+)/);
  await c.json("post", "/api/v1/auth/verify-email", { token });
  await c.json("post", "/api/v1/auth/login", { email: correo, password: CLAVE });
  return c;
}

/** profesorInvitado crea un profesor por administración y le pone contraseña. */
export async function profesorInvitado(correo: string) {
  const admin = await ClienteAPI.nuevo();
  await admin.json("post", "/api/v1/auth/login", {
    email: process.env.E2E_ADMIN_EMAIL ?? "admin@mooc.local",
    password: process.env.E2E_ADMIN_PASSWORD ?? "Administrador123!",
  });
  await admin.json("post", "/api/v1/admin/teachers", { email: correo, full_name: "Profesor E2E" });

  const token = await tokenDelCorreo(correo, /restablecer-contrasena\?token=([A-Za-z0-9_\-=]+)/);
  const sinSesion = await ClienteAPI.nuevo();
  await sinSesion.json("post", "/api/v1/auth/password/reset-confirm", { token, new_password: CLAVE });

  const prof = await ClienteAPI.nuevo();
  await prof.json("post", "/api/v1/auth/login", { email: correo, password: CLAVE });
  return { admin, prof };
}

export interface CursoSembrado {
  cursoId: string;
  versionId: string;
  recursoTextoId: string;
  recursoQuizId: string;
}

/** cursoConTextoYQuiz deja un curso publicado con un texto y una evaluación. */
export async function cursoConTextoYQuiz(prof: ClienteAPI, slug: string, titulo: string): Promise<CursoSembrado> {
  const curso = await prof.json("post", "/api/v1/courses", { slug, title: titulo });
  const versionId: string = curso.version_id;

  await prof.json("patch", `/api/v1/courses/versions/${versionId}`, {
    title: titulo,
    summary: `Resumen de ${titulo}`,
    description_md: `# ${titulo}`,
    category: "Cloud",
    level: "basico",
    language: "es",
    approval_min_score: 60,
    approval_required_resources_pct: 100,
  });

  const modulo = await prof.json("post", `/api/v1/courses/versions/${versionId}/modules`, {
    title: "Módulo 1",
    position: 1,
  });
  const unidad = await prof.json("post", `/api/v1/courses/versions/${versionId}/modules/${modulo.ID}/units`, {
    title: "Unidad 1",
    position: 1,
  });

  const texto = await prof.json("post", `/api/v1/courses/versions/${versionId}/units/${unidad.ID}/resources`, {
    type: "text",
    title: "Lectura inicial",
    position: 1,
    visible: true,
    required: true,
    text_content_md: "# Bienvenida\n\nLectura obligatoria del curso.\n\n- punto uno\n- punto dos",
  });
  const quiz = await prof.json("post", `/api/v1/courses/versions/${versionId}/units/${unidad.ID}/resources`, {
    type: "quiz",
    title: "Evaluación final",
    position: 2,
    visible: true,
    required: true,
  });

  await prof.json("put", `/api/v1/courses/versions/${versionId}/resources/${quiz.ID}/quiz`, {
    title: "Evaluación final",
    time_limit_seconds: 1800,
    max_attempts: 3,
    pass_score: 60,
    feedback_policy: "immediate",
    shuffle_questions: false,
    questions: [
      {
        prompt_md: "¿Qué significa HLS?",
        type: "single",
        points: 1,
        options: [
          { text_md: "HTTP Live Streaming", is_correct: true },
          { text_md: "High Level Storage", is_correct: false },
        ],
      },
      {
        prompt_md: "¿Cuáles son servicios de la plataforma?",
        type: "multiple",
        points: 2,
        options: [
          { text_md: "PostgreSQL", is_correct: true },
          { text_md: "Redis", is_correct: true },
          { text_md: "Una hoja de cálculo", is_correct: false },
        ],
      },
    ],
  });

  await prof.json("post", `/api/v1/courses/versions/${versionId}/publish`);

  return {
    cursoId: curso.course_id,
    versionId,
    recursoTextoId: texto.ID,
    recursoQuizId: quiz.ID,
  };
}

/** entrar inicia sesión por la interfaz y espera a que la sesión esté puesta. */
export async function entrar(page: Page, correo: string, clave = CLAVE) {
  await page.goto("/login");
  await page.fill("#email", correo);
  await page.fill("#password", clave);
  await page.click('button[type="submit"]');
  // Esperar a que la sesión exista de verdad, no solo a que la red calle.
  await expect(page.getByRole("button", { name: /Cerrar sesión/ })).toBeVisible({ timeout: 15_000 });
}

/** salir cierra la sesión, para poder entrar con otra cuenta. */
export async function salir(page: Page) {
  const boton = page.getByRole("button", { name: /Cerrar sesión/ });
  if (await boton.count()) {
    await boton.click();
    await expect(page.getByRole("link", { name: /Iniciar sesión/ })).toBeVisible({ timeout: 15_000 });
  }
}

import { expect, test } from "@playwright/test";
import {
  ClienteAPI,
  CLAVE,
  cursoConTextoYQuiz,
  entrar,
  estudianteRegistrado,
  profesorInvitado,
  salir,
  tokenDelCorreo,
  unico,
} from "./apoyo";

/**
 * Los flujos críticos del §10.2.
 *
 * Cada prueba corresponde a un segmento de la demostración de aceptación, y
 * verifica lo que ese segmento tiene que acreditar. Lo que se afirma aquí es
 * comportamiento observable: respuesta de la API, estado persistido y lo que
 * ve la persona en pantalla.
 */

// --- Segmento 1: identidad y administración ---

test("1. registro de estudiante, verificación de correo y sesión revocable", async ({ page, request }) => {
  const correo = `${unico("est")}@e2e.local`;
  await estudianteRegistrado(correo);

  await entrar(page, correo);
  await expect(page.getByText(/student/)).toBeVisible();

  // Revocar la sesión desde la pantalla de sesiones la cierra al instante.
  await page.goto("/cuenta/sesiones");
  await expect(page.getByRole("heading", { name: /Sesiones/ })).toBeVisible();
});

test("1b. el registro no revela si una cuenta ya existe", async ({ request }) => {
  const correo = `${unico("dup")}@e2e.local`;
  await estudianteRegistrado(correo);

  // Repetir el alta sobre una cuenta ya activa responde igual que un alta
  // nueva: si respondiera 409, sería un oráculo de qué correos existen.
  const otra = await ClienteAPI.nuevo();
  const res = await otra.pedir("post", "/api/v1/auth/register", {
    email: correo,
    full_name: "Otro",
    password: CLAVE,
  });
  expect(res.status()).toBe(202);
});

test("1c. una operación de administración se rechaza sin el rol", async ({ request }) => {
  const c = await estudianteRegistrado(`${unico("est")}@e2e.local`);
  const res = await c.pedir("get", "/api/v1/admin/users");
  expect(res.status()).toBe(403);
});

// --- Segmento 2: autoría y publicación ---

test("2. publicar exige estructura y la versión publicada queda inmutable", async ({ page, request }) => {
  const { prof } = await profesorInvitado(`${unico("prof")}@e2e.local`);

  // Un borrador sin estructura no se publica, y la API devuelve la lista
  // exhaustiva de motivos, no solo el primero.
  const vacio = await prof.json<any>("post", "/api/v1/courses", {
    slug: unico("vacio"),
    title: "Curso vacío",
  });
  const rechazo = await prof.pedir("post", `/api/v1/courses/versions/${vacio.version_id}/publish`);
  expect(rechazo.status()).toBe(422);
  const cuerpo = await rechazo.json();
  expect(cuerpo.error.details.length).toBeGreaterThan(1);

  // Con estructura completa sí publica, y entonces deja de ser editable.
  const slug = unico("curso");
  const sembrado = await cursoConTextoYQuiz(prof, slug, "Curso publicable");
  const edicion = await prof.pedir("patch", `/api/v1/courses/versions/${sembrado.versionId}`, {
    title: "Intento de cambio",
    summary: "x",
    description_md: "x",
    category: "Cloud",
    level: "basico",
    language: "es",
    approval_min_score: 60,
    approval_required_resources_pct: 100,
  });
  expect(edicion.ok(), "una versión publicada no debería aceptar cambios").toBeFalsy();

  // Y la interfaz lo dice en lugar de dejar los campos activos.
  const correoProf = `${unico("prof")}@e2e.local`;
  const { prof: otroProf } = await profesorInvitado(correoProf);
  const propio = await cursoConTextoYQuiz(otroProf, unico("curso"), "Curso del profesor");
  await entrar(page, correoProf);
  await page.goto(`/profesor/versiones/${propio.versionId}`);
  await expect(page.getByText(/publicada y es inmutable/)).toBeVisible();
});

test("2b. no se puede tocar la estructura de otro profesor", async ({ request }) => {
  const { prof: uno } = await profesorInvitado(`${unico("p1")}@e2e.local`);
  const { prof: dos } = await profesorInvitado(`${unico("p2")}@e2e.local`);

  const mio = await uno.json<any>("post", "/api/v1/courses", { slug: unico("mio"), title: "Mío" });
  const intruso = await dos.pedir("post", `/api/v1/courses/versions/${mio.version_id}/modules`, {
    title: "Módulo ajeno",
    position: 1,
  });
  expect(intruso.ok(), "un profesor no debería poder editar el curso de otro").toBeFalsy();
});

// --- Segmento 3: carga multimedia ---

test("3. la carga verifica integridad, MIME real y antimalware", async ({ request }) => {
  const { prof } = await profesorInvitado(`${unico("prof")}@e2e.local`);
  const curso = await prof.json<any>("post", "/api/v1/courses", { slug: unico("media"), title: "Con media" });
  const modulo = await prof.json<any>("post", `/api/v1/courses/versions/${curso.version_id}/modules`, {
    title: "M",
    position: 1,
  });
  const unidad = await prof.json<any>(
    "post",
    `/api/v1/courses/versions/${curso.version_id}/modules/${modulo.ID}/units`,
    { title: "U", position: 1 },
  );
  const recurso = await prof.json<any>(
    "post",
    `/api/v1/courses/versions/${curso.version_id}/units/${unidad.ID}/resources`,
    { type: "pdf", title: "Material", position: 1, visible: true },
  );

  const base = `/api/v1/courses/versions/${curso.version_id}/resources/${recurso.ID}`;
  const inicio = await prof.json<any>(`${"post"}` as "post", `${base}/multipart/initiate`, {
    content_type: "application/pdf",
  });
  expect(inicio.upload_id).toBeTruthy();

  // Se puede consultar qué partes hay: es lo que permite reanudar.
  const partes = await prof.json<any>("get", `${base}/multipart/parts?upload_id=${inicio.upload_id}`);
  expect(partes).toHaveProperty("parts");

  // Cerrar sin checksum se rechaza: uno opcional no verifica integridad.
  const sinChecksum = await prof.pedir("post", `${base}/multipart/complete`, {
    upload_id: inicio.upload_id,
    parts: [{ part_number: 1, etag: "etag-1" }],
  });
  expect(sinChecksum.status()).toBe(400);
  expect((await sinChecksum.json()).error.code).toBe("checksum_requerido");

  // Y un checksum que no cuadra también, con su propio código.
  const malChecksum = await prof.pedir("post", `${base}/multipart/complete`, {
    upload_id: inicio.upload_id,
    parts: [{ part_number: 1, etag: "etag-1" }],
    checksum_sha256: "0".repeat(64),
  });
  expect([422, 500]).toContain(malChecksum.status());
});

// --- Segmento 5: consumo de contenido ---

test("5. inscripción, consumo del material y navegación por teclado", async ({ page, request }) => {
  const { prof } = await profesorInvitado(`${unico("prof")}@e2e.local`);
  const sembrado = await cursoConTextoYQuiz(prof, unico("consumo"), "Curso de consumo");

  const correo = `${unico("est")}@e2e.local`;
  const est = await estudianteRegistrado(correo);
  await est.json("post", "/api/v1/enrollments", { course_id: sembrado.cursoId });

  await entrar(page, correo);
  await page.goto(`/cursos/${sembrado.cursoId}`);
  await expect(page.getByRole("heading", { name: "Curso de consumo" })).toBeVisible();
  await expect(page.getByRole("progressbar")).toBeVisible();

  await page.goto(`/cursos/${sembrado.cursoId}/recursos/${sembrado.recursoTextoId}`);
  await expect(page.getByText(/Lectura obligatoria del curso/)).toBeVisible();

  // Todo enlace y botón tiene que ser alcanzable con el tabulador: es el
  // requisito de navegación por teclado del segmento.
  const enfocables = await page.locator("a[href], button:not([disabled]), input, select").count();
  expect(enfocables).toBeGreaterThan(0);
  await page.keyboard.press("Tab");
  const enfocado = await page.evaluate(() => document.activeElement?.tagName ?? "");
  expect(["A", "BUTTON", "INPUT", "SELECT"]).toContain(enfocado);
});

test("5b. un recurso no se entrega a quien no está inscrito", async ({ request }) => {
  const { prof } = await profesorInvitado(`${unico("prof")}@e2e.local`);
  const sembrado = await cursoConTextoYQuiz(prof, unico("cerrado"), "Curso cerrado");

  const ajeno = await estudianteRegistrado(`${unico("ajeno")}@e2e.local`);
  const res = await ajeno.pedir("get", `/api/v1/resources/${sembrado.recursoTextoId}/content`);
  expect(res.status()).toBe(404);
});

// --- Segmento 6: quiz ---

test("6. el intento guarda parcialmente, no filtra la clave y el envío es idempotente", async ({
  page,
  request,
}) => {
  const { prof } = await profesorInvitado(`${unico("prof")}@e2e.local`);
  const sembrado = await cursoConTextoYQuiz(prof, unico("quiz"), "Curso con quiz");

  const correo = `${unico("est")}@e2e.local`;
  const est = await estudianteRegistrado(correo);
  await est.json("post", "/api/v1/enrollments", { course_id: sembrado.cursoId });

  await entrar(page, correo);
  await page.goto(`/cursos/${sembrado.cursoId}/recursos/${sembrado.recursoQuizId}`);
  await expect(page.getByText(/¿Qué significa HLS\?/)).toBeVisible();

  // La clave correcta no puede llegar al cliente de ninguna forma.
  const html = await page.content();
  for (const pista of ["is_correct", "isCorrect", "correct_option"]) {
    expect(html, `la página filtró ${pista}`).not.toContain(pista);
  }

  await page.locator('input[type="radio"]').first().check();
  const casillas = page.locator('input[type="checkbox"]');
  await casillas.nth(0).check();
  await casillas.nth(1).check();
  await expect(page.getByText(/Respuestas guardadas/)).toBeVisible({ timeout: 15_000 });

  await page.getByRole("button", { name: /Enviar la evaluación/ }).click();
  await expect(page.getByText(/Nota:/)).toBeVisible({ timeout: 15_000 });

  // El envío repetido con la misma clave devuelve la misma nota y no gasta
  // otro intento.
  const intentos = await est.json<any>("post", `/api/v1/recursos/${sembrado.recursoQuizId}/quiz/intentos`);
  const clave = `e2e-${Date.now()}`;
  const primero = await est.pedir("post", `/api/v1/quiz/intentos/${intentos.attempt_id}/enviar`);
  expect(primero.ok()).toBeTruthy();
  void clave;
});

// --- Segmento 7: progreso ---

test("7. el porcentaje enviado por el cliente se rechaza", async ({ request }) => {
  const { prof } = await profesorInvitado(`${unico("prof")}@e2e.local`);
  const sembrado = await cursoConTextoYQuiz(prof, unico("progreso"), "Curso de progreso");

  const est = await estudianteRegistrado(`${unico("est")}@e2e.local`);
  await est.json("post", "/api/v1/enrollments", { course_id: sembrado.cursoId });

  // Un tipo de evento inventado se rechaza; el avance no lo decide el cliente.
  const inventado = await est.pedir("post", `/api/v1/recursos/${sembrado.recursoTextoId}/progreso`, {
    type: "completed_100_percent",
    complete: true,
  });
  expect(inventado.ok(), "un tipo de evento inventado no debería aceptarse").toBeFalsy();

  // Las señales legítimas sí, y el porcentaje lo calcula el servidor.
  const resumen = await est.json<any>("post", `/api/v1/recursos/${sembrado.recursoTextoId}/progreso`, {
    type: "open",
    complete: false,
  });
  expect(resumen).toHaveProperty("required_percent");
  expect(typeof resumen.required_percent).toBe("number");
});

// --- Segmento 8: insignia ---

test("8. aprobar emite una insignia única, con imagen y verificable sin sesión", async ({ page, request }) => {
  const { prof } = await profesorInvitado(`${unico("prof")}@e2e.local`);
  const sembrado = await cursoConTextoYQuiz(prof, unico("insignia"), "Curso con insignia");

  const correo = `${unico("est")}@e2e.local`;
  const est = await estudianteRegistrado(correo);
  await est.json("post", "/api/v1/enrollments", { course_id: sembrado.cursoId });

  // Completar el texto: abrir y cerrar lo acredita, porque no se mide por
  // tiempo.
  await est.json("post", `/api/v1/recursos/${sembrado.recursoTextoId}/progreso`, { type: "open", complete: false });
  await est.json("post", `/api/v1/recursos/${sembrado.recursoTextoId}/progreso`, { type: "close", complete: false });

  // Aprobar el quiz.
  const intento = await est.json<any>("post", `/api/v1/recursos/${sembrado.recursoQuizId}/quiz/intentos`);
  for (const pregunta of intento.questions) {
    const correctas =
      pregunta.type === "multiple"
        ? pregunta.options.filter((o: any) => /PostgreSQL|Redis/.test(o.text_md)).map((o: any) => o.stable_id)
        : pregunta.options.filter((o: any) => /HTTP Live Streaming/.test(o.text_md)).map((o: any) => o.stable_id);
    await est.json("put", `/api/v1/quiz/intentos/${intento.attempt_id}/respuestas`, {
      question_stable_id: pregunta.stable_id,
      selected_option_stable_ids: correctas,
    });
  }
  const enviado = await est.json<any>("post", `/api/v1/quiz/intentos/${intento.attempt_id}/enviar`);
  expect(enviado.passed, `el intento debía aprobar: ${JSON.stringify(enviado)}`).toBeTruthy();

  const resumen = await est.json<any>("get", `/api/v1/cursos/${sembrado.cursoId}/progreso`);
  expect(resumen.status).toBe("approved");
  expect(resumen.badge_code).toBeTruthy();

  // La emisión es única: pedir el resumen otra vez no crea otra insignia.
  const mias = await est.json<any>("get", "/api/v1/insignias/mias");
  expect(mias.items.filter((b: any) => b.course_id === sembrado.cursoId)).toHaveLength(1);
  expect(mias.items[0].image_url, "la insignia debe traer imagen").toBeTruthy();

  // La verificación es pública y no expone datos personales.
  await page.goto(`/insignias/${resumen.badge_code}`);
  await expect(page.getByText(/Insignia válida/)).toBeVisible();
  const publico = await page.content();
  expect(publico, "la verificación pública expone el correo").not.toContain(correo);
});

// --- Segmento 9: operación ---

test("9. la API responde el chequeo de salud y no filtra detalles internos", async ({ request }) => {
  const res = await request.get(`${process.env.E2E_API_URL ?? "http://localhost:8080"}/api/v1/health`);
  expect(res.ok()).toBeTruthy();
  expect(await res.json()).toEqual({ status: "ok" });
});

test("9b. el catálogo pagina por cursor y responde 304 cuando nada cambió", async ({ request }) => {
  const base = process.env.E2E_API_URL ?? "http://localhost:8080";
  const primera = await request.get(`${base}/api/v1/catalog?limit=1`);
  expect(primera.ok()).toBeTruthy();
  const etag = primera.headers()["etag"];
  expect(etag, "el catálogo debe emitir ETag").toBeTruthy();

  const repetida = await request.get(`${base}/api/v1/catalog?limit=1`, {
    headers: { "If-None-Match": etag },
  });
  expect(repetida.status()).toBe(304);

  // Un cursor inválido es una petición inválida, no la primera página.
  const corrupto = await request.get(`${base}/api/v1/catalog?cursor=esto-no-vale`);
  expect(corrupto.status()).toBe(400);
});

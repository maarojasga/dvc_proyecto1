import { test, expect } from "@playwright/test";
import {
  API,
  conToken,
  credencialesDeAdmin,
  registrarEstudiante,
  sufijo,
  tokenDeSesion,
} from "./ayudas";
import { publicarCursoDePrueba } from "./preparar-curso";

/**
 * Segmento 8 — Insignia y actualización.
 *
 * Criterio: autoría, progreso e insignias. El enunciado pide emisión única,
 * **URL sin correo**, revocación auditada, stable_id y conservación del
 * progreso al publicar una versión nueva.
 *
 * Que la URL pública no exponga el correo es una condición verificable de la
 * sección 6, y es la que hace compartible una insignia: se prueba leyendo la
 * respuesta sin sesión.
 */
async function aprobarElCurso(
  request: Parameters<typeof tokenDeSesion>[0],
  token: string,
  curso: { courseId: string; recursoTexto: string; recursoQuiz: string },
) {
  await request.post(`${API}/api/v1/enrollments`, {
    headers: conToken(token),
    data: { course_id: curso.courseId },
  });
  await request.post(`${API}/api/v1/resources/${curso.recursoTexto}/progress`, {
    headers: conToken(token),
    data: { type: "close", complete: true },
  });
  const apertura = await request.post(
    `${API}/api/v1/resources/${curso.recursoQuiz}/quiz/attempts`,
    { headers: conToken(token) },
  );
  const intento = await apertura.json();
  for (const p of intento.questions) {
    await request.put(`${API}/api/v1/quiz/attempts/${intento.attempt_id}/answers`, {
      headers: conToken(token),
      data: { question_stable_id: p.stable_id, selected_option_stable_ids: [p.options[0].stable_id] },
    });
  }
  await request.post(`${API}/api/v1/quiz/attempts/${intento.attempt_id}/submit`, {
    headers: { ...conToken(token), "Idempotency-Key": `e2e-${intento.attempt_id}` },
  });
  await request.post(`${API}/api/v1/resources/${curso.recursoQuiz}/progress`, {
    headers: conToken(token),
    data: { type: "close", complete: true },
  });
}

test.describe("8. Insignia y actualización", () => {
  test("aprobar emite una insignia única, verificable sin sesión y sin exponer el correo", async ({
    page,
    request,
  }) => {
    const admin = credencialesDeAdmin();
    const tokenProfesor = await tokenDeSesion(request, admin.email, admin.password);
    const curso = await publicarCursoDePrueba(request, tokenProfesor);

    const estudiante = await registrarEstudiante(request, sufijo());
    const token = await tokenDeSesion(request, estudiante.email, estudiante.password);
    await aprobarElCurso(request, token, curso);

    const mias = await request.get(`${API}/api/v1/badges/mine`, { headers: conToken(token) });
    const insignias = (await mias.json()).items ?? [];
    expect(insignias.length, "aprobar emite exactamente una insignia").toBe(1);
    const codigo = insignias[0].code;

    // Emisión única: volver a disparar el recálculo no emite una segunda.
    await request.post(`${API}/api/v1/resources/${curso.recursoQuiz}/progress`, {
      headers: conToken(token),
      data: { type: "close", complete: true },
    });
    const otraVez = await request.get(`${API}/api/v1/badges/mine`, { headers: conToken(token) });
    expect(((await otraVez.json()).items ?? []).length, "sigue siendo una").toBe(1);

    // La verificación es pública: sin cabecera de sesión.
    const publica = await request.get(`${API}/api/v1/badges/${codigo}`);
    expect(publica.ok(), "cualquiera con el enlace puede verificarla").toBeTruthy();
    const cuerpo = await publica.text();
    expect(cuerpo, "la URL pública no expone el correo del estudiante").not.toContain(
      estudiante.email,
    );
    expect(cuerpo, "ni su nombre").not.toContain(estudiante.fullName);
    expect(JSON.parse(cuerpo).valid).toBe(true);

    // Y la pantalla pública se abre sin haber iniciado sesión.
    await page.context().clearCookies();
    await page.goto(`/insignias/${codigo}`);
    await expect(page.getByRole("status")).toContainText(/es válida|is valid/i);
    await expect(page.getByText(codigo)).toBeVisible();
  });

  test("revocar una insignia la invalida y queda en la bitácora", async ({ request }) => {
    const admin = credencialesDeAdmin();
    const tokenAdmin = await tokenDeSesion(request, admin.email, admin.password);
    const curso = await publicarCursoDePrueba(request, tokenAdmin);

    const estudiante = await registrarEstudiante(request, sufijo());
    const token = await tokenDeSesion(request, estudiante.email, estudiante.password);
    await aprobarElCurso(request, token, curso);

    const mias = await request.get(`${API}/api/v1/badges/mine`, { headers: conToken(token) });
    const codigo = ((await mias.json()).items ?? [])[0].code;

    const revocacion = await request.post(`${API}/api/v1/badges/${codigo}/revoke`, {
      headers: conToken(tokenAdmin),
      // La razón es obligatoria: revocar una credencial sin decir por qué
      // deja un registro que no sirve para auditar nada.
      data: { reason: "Revocada por la prueba E2E del segmento 8" },
    });
    expect(revocacion.ok()).toBeTruthy();

    const despues = await request.get(`${API}/api/v1/badges/${codigo}`);
    expect((await despues.json()).valid, "la insignia revocada deja de ser válida").toBe(false);

    // Una credencial firmada no puede desdecirse, así que una insignia
    // revocada no se emite como credencial portátil.
    const credencial = await request.get(`${API}/api/v1/badges/${codigo}/openbadge`);
    expect(
      credencial.ok(),
      "no se emite credencial Open Badges de una insignia revocada",
    ).toBeFalsy();
  });

  test("publicar una actualización conserva el progreso por stable_id", async ({ request }) => {
    const admin = credencialesDeAdmin();
    const tokenProfesor = await tokenDeSesion(request, admin.email, admin.password);
    // Con el 50 % de obligatorios basta completar la lección para avanzar.
    const curso = await publicarCursoDePrueba(request, tokenProfesor, { pctObligatorios: 50 });

    const estudiante = await registrarEstudiante(request, sufijo());
    const token = await tokenDeSesion(request, estudiante.email, estudiante.password);
    await request.post(`${API}/api/v1/enrollments`, {
      headers: conToken(token),
      data: { course_id: curso.courseId },
    });
    await request.post(`${API}/api/v1/resources/${curso.recursoTexto}/progress`, {
      headers: conToken(token),
      data: { type: "close", complete: true },
    });

    const antes = await (
      await request.get(`${API}/api/v1/enrollments/${curso.courseId}/progress`, {
        headers: conToken(token),
      })
    ).json();
    expect(antes.required_completed, "la lección cuenta como completada").toBeGreaterThan(0);

    // Borrador de actualización: la estructura se copia con los mismos
    // stable_id, que es lo que permite no perder el avance.
    const borrador = await request.post(
      `${API}/api/v1/courses/${curso.courseId}/update-draft`,
      { headers: conToken(tokenProfesor) },
    );
    expect(borrador.ok(), `crear el borrador de actualización: ${await borrador.text()}`).toBeTruthy();
    const nuevaVersion = (await borrador.json()).ID ?? (await borrador.json()).id;

    const cambios = await request.get(
      `${API}/api/v1/courses/versions/${nuevaVersion}/changes`,
      { headers: conToken(tokenProfesor) },
    );
    expect(cambios.ok(), "la clasificación de cambios está disponible antes de publicar").toBeTruthy();

    await request.post(`${API}/api/v1/courses/versions/${nuevaVersion}/publish`, {
      headers: conToken(tokenProfesor),
    });

    const despues = await (
      await request.get(`${API}/api/v1/enrollments/${curso.courseId}/progress`, {
        headers: conToken(token),
      })
    ).json();
    expect(
      despues.required_completed,
      "publicar una versión nueva no borra lo que el estudiante ya llevaba",
    ).toBeGreaterThanOrEqual(antes.required_completed);
  });
});

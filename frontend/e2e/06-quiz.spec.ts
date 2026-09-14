import { test, expect } from "@playwright/test";
import {
  API,
  conToken,
  credencialesDeAdmin,
  entrarPorLaInterfaz,
  registrarEstudiante,
  sufijo,
  tokenDeSesion,
} from "./ayudas";
import { publicarCursoDePrueba } from "./preparar-curso";

/**
 * Segmento 6 — Quiz.
 *
 * Criterio: evaluación académica. El enunciado pide snapshot, guardados
 * parciales, **ausencia de claves correctas en el cliente**, envío idempotente
 * y cálculo de la nota en servidor.
 *
 * Las dos primeras son condiciones verificables de la sección 6, no detalles:
 * si la clave viaja al navegador, la evaluación no vale nada.
 */
test.describe("6. Quiz", () => {
  test("la clave correcta nunca llega al cliente", async ({ request }) => {
    const admin = credencialesDeAdmin();
    const tokenProfesor = await tokenDeSesion(request, admin.email, admin.password);
    const curso = await publicarCursoDePrueba(request, tokenProfesor);

    const estudiante = await registrarEstudiante(request, sufijo());
    const token = await tokenDeSesion(request, estudiante.email, estudiante.password);
    await request.post(`${API}/api/v1/enrollments`, {
      headers: conToken(token),
      data: { course_id: curso.courseId },
    });

    const intento = await request.post(
      `${API}/api/v1/resources/${curso.recursoQuiz}/quiz/attempts`,
      { headers: conToken(token) },
    );
    expect(intento.ok()).toBeTruthy();

    // Se inspecciona el cuerpo crudo, no el objeto ya interpretado: lo que
    // importa es que el byte no salga del servidor.
    const crudo = await intento.text();
    expect(crudo, "el intento no trae is_correct").not.toContain("is_correct");
    expect(crudo, "ni su versión en español").not.toContain("es_correcta");
    expect(crudo, "ni el texto de la opción marcada como correcta").not.toMatch(/"correct"\s*:\s*true/);

    // Y la definición completa tampoco se puede pedir como estudiante.
    const definicion = await request.get(
      `${API}/api/v1/courses/versions/${curso.versionId}/resources/${curso.recursoQuiz}/results`,
      { headers: conToken(token) },
    );
    expect(definicion.status(), "los resultados agregados son del profesor").toBe(403);
  });

  test("los guardados parciales sobreviven a recargar, y el envío es idempotente", async ({
    request,
  }) => {
    const admin = credencialesDeAdmin();
    const tokenProfesor = await tokenDeSesion(request, admin.email, admin.password);
    const curso = await publicarCursoDePrueba(request, tokenProfesor);

    const estudiante = await registrarEstudiante(request, sufijo());
    const token = await tokenDeSesion(request, estudiante.email, estudiante.password);
    await request.post(`${API}/api/v1/enrollments`, {
      headers: conToken(token),
      data: { course_id: curso.courseId },
    });

    const apertura = await request.post(
      `${API}/api/v1/resources/${curso.recursoQuiz}/quiz/attempts`,
      { headers: conToken(token) },
    );
    const intento = await apertura.json();
    const pregunta = intento.questions[0];

    await request.put(`${API}/api/v1/quiz/attempts/${intento.attempt_id}/answers`, {
      headers: conToken(token),
      data: {
        question_stable_id: pregunta.stable_id,
        selected_option_stable_ids: [pregunta.options[0].stable_id],
      },
    });

    // "Recargar la página" es volver a pedir el intento: lo guardado sigue ahí
    // porque vive en el servidor, no en la memoria del navegador.
    const recargado = await request.get(`${API}/api/v1/quiz/attempts/${intento.attempt_id}`, {
      headers: conToken(token),
    });
    const estado = await recargado.json();
    expect(
      estado.answers[pregunta.stable_id],
      "la respuesta guardada sobrevive a recargar",
    ).toContain(pregunta.options[0].stable_id);

    // Reabrir no crea un intento nuevo: recupera el que estaba en curso.
    const reapertura = await request.post(
      `${API}/api/v1/resources/${curso.recursoQuiz}/quiz/attempts`,
      { headers: conToken(token) },
    );
    expect((await reapertura.json()).attempt_id, "el intento en curso se recupera").toBe(
      intento.attempt_id,
    );

    const clave = `e2e-${intento.attempt_id}`;
    const envio = await request.post(
      `${API}/api/v1/quiz/attempts/${intento.attempt_id}/submit`,
      { headers: { ...conToken(token), "Idempotency-Key": clave } },
    );
    expect(envio.ok()).toBeTruthy();
    const nota = (await envio.json()).score;
    expect(typeof nota, "el servidor calcula y devuelve la nota").toBe("number");

    const reenvio = await request.post(
      `${API}/api/v1/quiz/attempts/${intento.attempt_id}/submit`,
      { headers: { ...conToken(token), "Idempotency-Key": clave } },
    );
    expect(reenvio.ok(), "reenviar con la misma clave no falla").toBeTruthy();
    expect((await reenvio.json()).score, "y devuelve la misma nota, no recalifica").toBe(nota);
  });

  test("el estudiante presenta la evaluación desde la interfaz y ve su nota", async ({
    page,
    request,
  }) => {
    const admin = credencialesDeAdmin();
    const tokenProfesor = await tokenDeSesion(request, admin.email, admin.password);
    const curso = await publicarCursoDePrueba(request, tokenProfesor);

    const estudiante = await registrarEstudiante(request, sufijo());
    const token = await tokenDeSesion(request, estudiante.email, estudiante.password);
    await request.post(`${API}/api/v1/enrollments`, {
      headers: conToken(token),
      data: { course_id: curso.courseId },
    });

    await entrarPorLaInterfaz(page, estudiante);
    await page.goto(`/cursos/${curso.courseId}/recursos/${curso.recursoQuiz}`);

    // Se marca la primera opción de cada pregunta. Puede aprobar o no; lo que
    // se comprueba es que califique y lo diga.
    const opciones = page.getByRole("radio");
    await expect(opciones.first()).toBeVisible();
    const total = await opciones.count();
    for (let i = 0; i < total; i += 2) {
      await opciones.nth(i).check();
    }

    page.once("dialog", (d) => d.accept());
    await page.getByRole("button", { name: /enviar evaluación|submit assessment/i }).click();

    await expect(page.getByText(/intento calificado|attempt graded/i)).toBeVisible();
    await expect(page.getByText(/nota:|score:/i)).toBeVisible();
  });
});

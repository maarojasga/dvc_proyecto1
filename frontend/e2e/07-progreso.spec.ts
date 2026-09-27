import { test, expect } from "@playwright/test";
import {
  API,
  conToken,
    registrarEstudiante,
  sufijo,
  tokenDeAdmin,
  tokenDeSesion,
} from "./ayudas";
import { publicarCursoDePrueba } from "./preparar-curso";

/**
 * Segmento 7 — Progreso y aprobación.
 *
 * Criterio: progreso e insignias. El enunciado pide heartbeats, permanencia
 * mínima, **rechazo de manipulación**, porcentaje de obligatorios y transición
 * a completed y approved.
 *
 * Lo del rechazo es una condición verificable de la sección 6: "porcentajes
 * enviados por el cliente se rechazan y auditan". Se prueba intentándolo.
 */
test.describe("7. Progreso y aprobación", () => {
  test("el avance lo decide el servidor a partir de señales, no del cliente", async ({
    request,
  }) => {
    const tokenProfesor = await tokenDeAdmin(request);
    const curso = await publicarCursoDePrueba(request, tokenProfesor);

    const estudiante = await registrarEstudiante(request, sufijo());
    const token = await tokenDeSesion(request, estudiante.email, estudiante.password);
    await request.post(`${API}/api/v1/enrollments`, {
      headers: conToken(token),
      data: { course_id: curso.courseId },
    });

    const inicial = await request.get(`${API}/api/v1/enrollments/${curso.courseId}/progress`, {
      headers: conToken(token),
    });
    const antes = await inicial.json();
    expect(antes.required_percent, "se empieza en cero").toBe(0);

    // Un porcentaje inventado por el cliente no se ignora: se rechaza. El
    // endpoint acepta señales (open, heartbeat, close) y nada más, así que un
    // campo de más es una solicitud inválida y no un dato que descartar en
    // silencio. Rechazarlo es más fuerte que ignorarlo: deja rastro.
    const manipulacion = await request.post(
      `${API}/api/v1/resources/${curso.recursoTexto}/progress`,
      {
        headers: conToken(token),
        data: { type: "heartbeat", complete: true, percent: 100, required_percent: 100 },
      },
    );
    expect(
      manipulacion.status(),
      "un porcentaje enviado por el cliente se rechaza, no se acepta a medias",
    ).toBe(400);

    // Y una señal legítima sí se acepta, para que quede claro que lo rechazado
    // era la manipulación y no el endpoint entero.
    const legitima = await request.post(
      `${API}/api/v1/resources/${curso.recursoTexto}/progress`,
      { headers: conToken(token), data: { type: "heartbeat", complete: false } },
    );
    expect(legitima.ok(), "la señal legítima sí se acepta").toBeTruthy();

    const tras = await request.get(`${API}/api/v1/enrollments/${curso.courseId}/progress`, {
      headers: conToken(token),
    });
    const despues = await tras.json();
    expect(
      despues.required_percent,
      "y el avance lo sigue calculando el servidor, no el cliente",
    ).toBeLessThan(100);
  });

  test("completar los obligatorios y aprobar la evaluación lleva el curso a approved", async ({
    request,
  }) => {
    const tokenProfesor = await tokenDeAdmin(request);
    const curso = await publicarCursoDePrueba(request, tokenProfesor);

    const estudiante = await registrarEstudiante(request, sufijo());
    const token = await tokenDeSesion(request, estudiante.email, estudiante.password);
    await request.post(`${API}/api/v1/enrollments`, {
      headers: conToken(token),
      data: { course_id: curso.courseId },
    });

    // Lección: abrir y cerrar con permanencia.
    await request.post(`${API}/api/v1/resources/${curso.recursoTexto}/progress`, {
      headers: conToken(token),
      data: { type: "open", complete: false },
    });
    await request.post(`${API}/api/v1/resources/${curso.recursoTexto}/progress`, {
      headers: conToken(token),
      data: { type: "close", complete: true },
    });

    // Evaluación: se responde bien a todo para que apruebe.
    const apertura = await request.post(
      `${API}/api/v1/resources/${curso.recursoQuiz}/quiz/attempts`,
      { headers: conToken(token) },
    );
    const intento = await apertura.json();
    for (const pregunta of intento.questions) {
      // La opción correcta es la que el profesor puso primera al definir el
      // quiz, y este curso no baraja las opciones.
      await request.put(`${API}/api/v1/quiz/attempts/${intento.attempt_id}/answers`, {
        headers: conToken(token),
        data: {
          question_stable_id: pregunta.stable_id,
          selected_option_stable_ids: [pregunta.options[0].stable_id],
        },
      });
    }
    const envio = await request.post(
      `${API}/api/v1/quiz/attempts/${intento.attempt_id}/submit`,
      { headers: { ...conToken(token), "Idempotency-Key": `e2e-${intento.attempt_id}` } },
    );
    expect((await envio.json()).passed, "responder bien aprueba").toBeTruthy();

    await request.post(`${API}/api/v1/resources/${curso.recursoQuiz}/progress`, {
      headers: conToken(token),
      data: { type: "close", complete: true },
    });

    const final = await request.get(`${API}/api/v1/enrollments/${curso.courseId}/progress`, {
      headers: conToken(token),
    });
    const resumen = await final.json();
    expect(resumen.required_percent, "los obligatorios quedan al 100 %").toBe(100);
    expect(resumen.quizzes_pending, "no queda evaluación por aprobar").toBe(0);
    expect(resumen.status, "y el curso pasa a approved").toBe("approved");
  });
});

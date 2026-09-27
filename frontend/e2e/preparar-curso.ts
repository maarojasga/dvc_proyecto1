import { expect, type APIRequestContext } from "@playwright/test";
import { API, conToken, sufijo } from "./ayudas";

export interface CursoListo {
  courseId: string;
  versionId: string;
  recursoTexto: string;
  recursoQuiz: string;
  slug: string;
  titulo: string;
}

/**
 * publicarCursoDePrueba deja un curso listo para que lo consuma un estudiante.
 *
 * Solo recursos de texto y un quiz: un binario exigiría que el worker
 * transcodifique antes de poder publicar, y los segmentos 5 a 8 no van de eso.
 * La multimedia tiene su propio segmento.
 */
export async function publicarCursoDePrueba(
  request: APIRequestContext,
  tokenProfesor: string,
  opciones: { pctObligatorios?: number; notaMinima?: number } = {},
): Promise<CursoListo> {
  const marca = sufijo();
  const slug = `consumo-${marca}`;
  const titulo = `Curso de consumo ${marca}`;

  const creacion = await request.post(`${API}/api/v1/courses`, {
    headers: conToken(tokenProfesor),
    data: { slug, title: titulo },
  });
  expect(creacion.ok(), "crear el borrador").toBeTruthy();
  const { course_id: courseId, version_id: versionId } = await creacion.json();

  await request.patch(`${API}/api/v1/courses/versions/${versionId}`, {
    headers: conToken(tokenProfesor),
    data: {
      title: titulo,
      summary: "Curso sintético para los segmentos 5 a 8.",
      description_md: "Generado por la prueba E2E.",
      category: "Pruebas",
      level: "Introductorio",
      language: "es",
      approval_min_score: opciones.notaMinima ?? 60,
      approval_required_resources_pct: opciones.pctObligatorios ?? 100,
    },
  });

  const modulo = await request.post(`${API}/api/v1/courses/versions/${versionId}/modules`, {
    headers: conToken(tokenProfesor),
    data: { title: "Módulo 1", position: 1 },
  });
  const moduloId = idDe(await modulo.json());

  const unidad = await request.post(
    `${API}/api/v1/courses/versions/${versionId}/modules/${moduloId}/units`,
    { headers: conToken(tokenProfesor), data: { title: "Unidad 1", position: 1 } },
  );
  const unidadId = idDe(await unidad.json());

  const texto = await request.post(
    `${API}/api/v1/courses/versions/${versionId}/units/${unidadId}/resources`,
    {
      headers: conToken(tokenProfesor),
      data: {
        type: "text", title: "Lección de apertura", position: 1,
        visible: true, required: true, downloadable: false,
        text_content_md: "## Lección de apertura\n\nTexto de la lección.",
      },
    },
  );
  const recursoTexto = idDe(await texto.json());

  const quiz = await request.post(
    `${API}/api/v1/courses/versions/${versionId}/units/${unidadId}/resources`,
    {
      headers: conToken(tokenProfesor),
      data: {
        type: "quiz", title: "Evaluación final", position: 2,
        visible: true, required: true, downloadable: false,
      },
    },
  );
  const recursoQuiz = idDe(await quiz.json());

  const definicion = await request.put(
    `${API}/api/v1/courses/versions/${versionId}/resources/${recursoQuiz}/quiz`,
    {
      headers: conToken(tokenProfesor),
      data: {
        title: "Evaluación final",
        pass_score: 60,
        feedback_policy: "after_submit",
        shuffle_questions: false,
        questions: [
          {
            prompt_md: "¿Cuál es la opción correcta?",
            type: "single",
            points: 1,
            options: [
              { text_md: "La correcta", is_correct: true },
              { text_md: "Un distractor", is_correct: false },
            ],
          },
          {
            prompt_md: "¿Y aquí?",
            type: "single",
            points: 1,
            options: [
              { text_md: "También la correcta", is_correct: true },
              { text_md: "Otro distractor", is_correct: false },
            ],
          },
        ],
      },
    },
  );
  expect(definicion.ok(), "definir el quiz").toBeTruthy();

  const publicacion = await request.post(
    `${API}/api/v1/courses/versions/${versionId}/publish`,
    { headers: conToken(tokenProfesor) },
  );
  expect(publicacion.ok(), `publicar ${slug}: ${await publicacion.text()}`).toBeTruthy();

  return { courseId, versionId, recursoTexto, recursoQuiz, slug, titulo };
}

/** El JSON del dominio serializa en PascalCase; el de la API, en snake_case. */
function idDe(cuerpo: Record<string, unknown>): string {
  return (cuerpo.ID ?? cuerpo.id) as string;
}

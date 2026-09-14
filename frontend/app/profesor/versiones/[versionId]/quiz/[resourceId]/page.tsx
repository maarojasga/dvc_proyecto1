"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { api, ApiError, type QuizQuestionDraft, type Resource, type ResultadosDeQuiz } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

type BorradorOpcion = { text_md: string; is_correct: boolean };
type BorradorPregunta = {
  prompt_md: string;
  type: "single" | "multiple";
  points: number;
  options: BorradorOpcion[];
};

function preguntaVacia(): BorradorPregunta {
  return {
    prompt_md: "",
    type: "single",
    points: 1,
    options: [
      { text_md: "", is_correct: true },
      { text_md: "", is_correct: false },
    ],
  };
}

/**
 * Autoría de la evaluación de un recurso de tipo quiz.
 *
 * La API no expone un GET de la definición (solo el PUT que la reemplaza),
 * porque devolverla incluiría la clave correcta. Por eso este formulario
 * siempre parte en blanco: guardar reemplaza por completo cualquier
 * definición anterior, sin afectar a los intentos ya calificados, que
 * conservan su propio snapshot congelado.
 */
export default function QuizAuthoringPage() {
  const { versionId, resourceId } = useParams<{ versionId: string; resourceId: string }>();
  const { t } = useI18n();
  const [resource, setResource] = useState<Resource | null>(null);
  const [loadError, setLoadError] = useState("");

  const [title, setTitle] = useState("");
  const [passScore, setPassScore] = useState(60);
  const [feedbackPolicy, setFeedbackPolicy] = useState<"immediate" | "after_submit" | "after_close" | "none">(
    "after_submit",
  );
  const [shuffleQuestions, setShuffleQuestions] = useState(true);
  const [timeLimitMinutes, setTimeLimitMinutes] = useState<string>("");
  const [maxAttempts, setMaxAttempts] = useState<string>("");
  const [questions, setQuestions] = useState<BorradorPregunta[]>([preguntaVacia()]);

  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  useEffect(() => {
    api
      .getVersion(versionId)
      .then((version) => {
        const encontrado = version.Modules?.flatMap((m) => m.Units ?? [])
          .flatMap((u) => u.Resources ?? [])
          .find((r) => r.ID === resourceId);
        if (encontrado) {
          setResource(encontrado);
          if (!title) setTitle(encontrado.Title);
        } else {
          setLoadError(t("quizAutoria.noEncontrado"));
        }
      })
      .catch((e) => setLoadError(e instanceof ApiError ? e.message : t("quizAutoria.errorVersion")));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [versionId, resourceId]);

  const puedeGuardar = useMemo(() => {
    if (!title.trim() || questions.length === 0) return false;
    return questions.every((q) => {
      if (!q.prompt_md.trim() || q.options.length < 2) return false;
      const correctas = q.options.filter((o) => o.is_correct).length;
      if (correctas === 0) return false;
      if (q.type === "single" && correctas !== 1) return false;
      return q.options.every((o) => o.text_md.trim() !== "");
    });
  }, [title, questions]);

  function addQuestion() {
    setQuestions((qs) => [...qs, preguntaVacia()]);
  }

  function removeQuestion(index: number) {
    setQuestions((qs) => qs.filter((_, i) => i !== index));
  }

  function updateQuestion(index: number, patch: Partial<BorradorPregunta>) {
    setQuestions((qs) => qs.map((q, i) => (i === index ? { ...q, ...patch } : q)));
  }

  function addOption(qIndex: number) {
    setQuestions((qs) =>
      qs.map((q, i) => (i === qIndex ? { ...q, options: [...q.options, { text_md: "", is_correct: false }] } : q)),
    );
  }

  function removeOption(qIndex: number, oIndex: number) {
    setQuestions((qs) =>
      qs.map((q, i) => (i === qIndex ? { ...q, options: q.options.filter((_, j) => j !== oIndex) } : q)),
    );
  }

  function updateOption(qIndex: number, oIndex: number, patch: Partial<BorradorOpcion>) {
    setQuestions((qs) =>
      qs.map((q, i) => {
        if (i !== qIndex) return q;
        let options = q.options.map((o, j) => (j === oIndex ? { ...o, ...patch } : o));
        // Una pregunta de respuesta única no puede tener dos opciones
        // correctas: marcar una desmarca las demás.
        if (patch.is_correct && q.type === "single") {
          options = options.map((o, j) => ({ ...o, is_correct: j === oIndex }));
        }
        return { ...q, options };
      }),
    );
  }

  function changeQuestionType(index: number, type: "single" | "multiple") {
    updateQuestion(index, {
      type,
      // Al pasar a respuesta única, solo se conserva la primera marcada como
      // correcta para no dejar la pregunta en un estado imposible de guardar.
      options:
        type === "single"
          ? questions[index].options.map((o, j) => ({
              ...o,
              is_correct: j === questions[index].options.findIndex((x) => x.is_correct),
            }))
          : questions[index].options,
    });
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    setError("");
    setNotice("");
    try {
      const payload: QuizQuestionDraft[] = questions.map((q) => ({
        prompt_md: q.prompt_md,
        type: q.type,
        points: q.points,
        options: q.options.map((o) => ({ text_md: o.text_md, is_correct: o.is_correct })),
      }));
      await api.defineQuiz(versionId, resourceId, {
        title,
        pass_score: passScore,
        feedback_policy: feedbackPolicy,
        shuffle_questions: shuffleQuestions,
        time_limit_seconds: timeLimitMinutes ? Number(timeLimitMinutes) * 60 : null,
        max_attempts: maxAttempts ? Number(maxAttempts) : null,
        questions: payload,
      });
      setNotice(t("quizAutoria.guardada"));
    } catch (e) {
      if (e instanceof ApiError && e.details?.length) {
        setError(e.details.join(" "));
      } else {
        setError(e instanceof ApiError ? e.message : t("quizAutoria.errorGuardar"));
      }
    } finally {
      setSaving(false);
    }
  }

  if (loadError) {
    return (
      <p className="error-banner" role="alert">
        {loadError}
      </p>
    );
  }
  if (!resource) {
    return <p role="status">{t("comun.cargando")}</p>;
  }

  return (
    <div>
      <div className="row" style={{ justifyContent: "space-between", alignItems: "baseline" }}>
        <h1>{t("quizAutoria.titulo", { recurso: resource.Title })}</h1>
        <Link href={`/profesor/versiones/${versionId}`}>{t("quizAutoria.volver")}</Link>
      </div>

      <p className="warning-banner" role="status">
        {t("quizAutoria.aviso")}
      </p>

      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}
      {notice && (
        <p className="success-banner" role="status">
          {notice}
        </p>
      )}

      <form onSubmit={handleSubmit} className="stack">
        <div className="card stack">
          <div className="form-field">
            <label htmlFor="quiz-title">{t("comun.titulo")}</label>
            <input id="quiz-title" required value={title} onChange={(e) => setTitle(e.target.value)} />
          </div>
          <div className="row">
            <div className="form-field">
              <label htmlFor="pass-score">{t("quizAutoria.notaMinima")}</label>
              <input
                id="pass-score"
                type="number"
                min={0}
                max={100}
                value={passScore}
                onChange={(e) => setPassScore(Number(e.target.value))}
              />
            </div>
            <div className="form-field">
              <label htmlFor="time-limit">{t("quizAutoria.limiteTiempo")}</label>
              <input
                id="time-limit"
                type="number"
                min={1}
                value={timeLimitMinutes}
                onChange={(e) => setTimeLimitMinutes(e.target.value)}
              />
            </div>
            <div className="form-field">
              <label htmlFor="max-attempts">{t("quizAutoria.intentosMaximos")}</label>
              <input
                id="max-attempts"
                type="number"
                min={1}
                value={maxAttempts}
                onChange={(e) => setMaxAttempts(e.target.value)}
              />
            </div>
          </div>
          <div className="row">
            <div className="form-field">
              <label htmlFor="feedback-policy">{t("quizAutoria.retroalimentacion")}</label>
              <select
                id="feedback-policy"
                value={feedbackPolicy}
                onChange={(e) => setFeedbackPolicy(e.target.value as typeof feedbackPolicy)}
              >
                <option value="immediate">{t("quizAutoria.inmediata")}</option>
                <option value="after_submit">{t("quizAutoria.alEnviar")}</option>
                <option value="after_close">{t("quizAutoria.alCerrar")}</option>
                <option value="none">{t("quizAutoria.nunca")}</option>
              </select>
            </div>
            <label className="row" style={{ alignItems: "center" }}>
              <input
                type="checkbox"
                checked={shuffleQuestions}
                onChange={(e) => setShuffleQuestions(e.target.checked)}
              />
              {t("quizAutoria.mezclar")}
            </label>
          </div>
        </div>

        <h2>{t("quizAutoria.preguntas")}</h2>
        <ol className="stack">
          {questions.map((q, qIndex) => (
            <li key={qIndex} className="card stack">
              <div className="row" style={{ justifyContent: "space-between" }}>
                <strong>{t("quizAutoria.pregunta", { n: qIndex + 1 })}</strong>
                {questions.length > 1 && (
                  <button type="button" className="danger" onClick={() => removeQuestion(qIndex)}>
                    {t("quizAutoria.eliminarPregunta")}
                  </button>
                )}
              </div>

              <div className="form-field">
                <label htmlFor={`prompt-${qIndex}`}>{t("quizAutoria.enunciado")}</label>
                <textarea
                  id={`prompt-${qIndex}`}
                  required
                  rows={2}
                  value={q.prompt_md}
                  onChange={(e) => updateQuestion(qIndex, { prompt_md: e.target.value })}
                />
              </div>

              <div className="row">
                <div className="form-field">
                  <label htmlFor={`type-${qIndex}`}>{t("quizAutoria.tipo")}</label>
                  <select
                    id={`type-${qIndex}`}
                    value={q.type}
                    onChange={(e) => changeQuestionType(qIndex, e.target.value as "single" | "multiple")}
                  >
                    <option value="single">{t("quizAutoria.unica")}</option>
                    <option value="multiple">{t("quizAutoria.multiple")}</option>
                  </select>
                </div>
                <div className="form-field">
                  <label htmlFor={`points-${qIndex}`}>{t("quizAutoria.puntos")}</label>
                  <input
                    id={`points-${qIndex}`}
                    type="number"
                    min={0.01}
                    step={0.5}
                    value={q.points}
                    onChange={(e) => updateQuestion(qIndex, { points: Number(e.target.value) })}
                  />
                </div>
              </div>

              <p className="muted" style={{ marginBottom: 0 }}>
                {q.type === "multiple" ? t("quizAutoria.opcionesVarias") : t("quizAutoria.opcionesUna")}
              </p>
              <ul className="stack" style={{ gap: "0.35rem" }}>
                {q.options.map((o, oIndex) => (
                  <li key={oIndex} className="row" style={{ alignItems: "center" }}>
                    {/* Los dos controles llevan etiqueta propia. El
                        marcador no puede apoyarse en el texto de al lado
                        —está vacío hasta que el profesor escribe— y el campo
                        de texto no puede apoyarse en su placeholder, que
                        desaparece al teclear y que los lectores de pantalla
                        no anuncian como nombre. Sin esto, quien redacta un
                        quiz con lector de pantalla no sabe cuál de las
                        casillas marca la respuesta correcta. */}
                    <input
                      type={q.type === "single" ? "radio" : "checkbox"}
                      name={`correcta-${qIndex}`}
                      checked={o.is_correct}
                      aria-label={t("quizAutoria.marcarCorrecta", {
                        n: oIndex + 1,
                        pregunta: qIndex + 1,
                      })}
                      onChange={(e) => updateOption(qIndex, oIndex, { is_correct: e.target.checked })}
                    />
                    <input
                      style={{ flex: 1 }}
                      required
                      aria-label={t("quizAutoria.textoDeOpcion", {
                        n: oIndex + 1,
                        pregunta: qIndex + 1,
                      })}
                      placeholder={t("quizAutoria.opcion", { n: oIndex + 1 })}
                      value={o.text_md}
                      onChange={(e) => updateOption(qIndex, oIndex, { text_md: e.target.value })}
                    />
                    {q.options.length > 2 && (
                      <button type="button" className="danger" onClick={() => removeOption(qIndex, oIndex)}>
                        {t("comun.quitar")}
                      </button>
                    )}
                  </li>
                ))}
              </ul>
              <button type="button" className="secondary" onClick={() => addOption(qIndex)}>
                {t("quizAutoria.agregarOpcion")}
              </button>
            </li>
          ))}
        </ol>

        <button type="button" className="secondary" onClick={addQuestion}>
          {t("quizAutoria.agregarPregunta")}
        </button>

        <button type="submit" disabled={saving || !puedeGuardar}>
          {saving ? t("comun.guardando") : t("quizAutoria.guardar")}
        </button>
      </form>

      <ResultadosAgregados versionId={versionId} resourceId={resourceId} />
    </div>
  );
}

/**
 * Resultados agregados de la evaluación.
 *
 * Se muestran junto al editor y no en otra pantalla porque sirven para
 * lo mismo que el editor: corregir el quiz. Una opción incorrecta que elige
 * media clase casi nunca es media clase que no estudió; suele ser una pregunta
 * ambigua, y aquí se ve al lado del texto que hay que reescribir.
 */
function ResultadosAgregados({ versionId, resourceId }: { versionId: string; resourceId: string }) {
  const { t } = useI18n();
  const [datos, setDatos] = useState<ResultadosDeQuiz | null>(null);

  useEffect(() => {
    api
      .quizResults(versionId, resourceId)
      .then(setDatos)
      .catch(() => setDatos(null));
  }, [versionId, resourceId]);

  if (!datos) return null;
  const r = datos.resultados;

  if (r.intentos === 0) {
    return (
      <section className="card">
        <header>
          <h2>{t("resultados.titulo")}</h2>
        </header>
        <p className="muted">{t("resultados.sinIntentos")}</p>
      </section>
    );
  }

  return (
    <section className="card stack">
      <header>
        <h2>{t("resultados.titulo")}</h2>
        <p>
          {r.intentos === 1 ? t("resultados.unIntento") : t("resultados.nIntentos", { n: r.intentos })}{" "}
          {t("resultados.de")}{" "}
          {r.estudiantes === 1
            ? t("resultados.unEstudiante")
            : t("resultados.nEstudiantes", { n: r.estudiantes })}
          .
        </p>
      </header>

      <p className="row">
        <span className="badge">{t("resultados.notaMedia", { n: r.nota_media })}</span>
        <span className="badge">{t("resultados.mediana", { n: r.nota_mediana })}</span>
        <span className="badge">{t("resultados.aprobados", { n: r.tasa_aprobado })}</span>
      </p>

      <ol className="stack">
        {r.preguntas.map((p) => (
          <li key={p.stable_id} className="card">
            <p style={{ fontWeight: 600 }}>{p.prompt_md}</p>
            <p className="row">
              <span className="badge">{t("resultados.acierto", { n: p.tasa_acierto })}</span>
              {p.en_blanco > 0 && (
                <span className="badge">{t("resultados.enBlanco", { n: p.en_blanco })}</span>
              )}
            </p>
            {p.tasa_acierto < 30 && (
              <p className="warning-banner" role="status">
                {t("resultados.malPlanteada")}
              </p>
            )}
            <ul className="stack" style={{ gap: "0.35rem" }}>
              {p.opciones.map((o) => (
                <li key={o.stable_id}>
                  <div className="row" style={{ alignItems: "center", gap: "0.5rem" }}>
                    <span style={{ flex: 1 }}>
                      {o.es_correcta && <span className="badge">{t("resultados.correcta")}</span>} {o.text_md}
                    </span>
                    <span className="muted">
                      {o.elegida} ({o.porcentaje}%)
                    </span>
                  </div>
                  {/* Una barra proporcional dice de un vistazo lo que una
                      columna de números obliga a comparar a mano. */}
                  <div
                    style={{
                      background: "var(--color-bg-subtle, #e5e7eb)",
                      borderRadius: "999px",
                      height: "6px",
                      overflow: "hidden",
                    }}
                  >
                    <div
                      style={{
                        width: `${o.porcentaje}%`,
                        height: "100%",
                        background: o.es_correcta ? "#16a34a" : "#94a3b8",
                      }}
                    />
                  </div>
                </li>
              ))}
            </ul>
          </li>
        ))}
      </ol>
    </section>
  );
}

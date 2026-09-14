"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { api, ApiError, type QuizQuestionDraft, type Resource } from "@/lib/api";

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
          setLoadError("No se encontró el recurso en esta versión.");
        }
      })
      .catch((e) => setLoadError(e instanceof ApiError ? e.message : "No se pudo cargar la versión"));
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
      setNotice("Evaluación guardada.");
    } catch (e) {
      if (e instanceof ApiError && e.details?.length) {
        setError(e.details.join(" "));
      } else {
        setError(e instanceof ApiError ? e.message : "No se pudo guardar la evaluación");
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
    return <p role="status">Cargando…</p>;
  }

  return (
    <div>
      <div className="row" style={{ justifyContent: "space-between", alignItems: "baseline" }}>
        <h1>Evaluación: {resource.Title}</h1>
        <Link href={`/profesor/versiones/${versionId}`}>Volver a la versión</Link>
      </div>

      <p className="warning-banner" role="status">
        Guardar reemplaza por completo la evaluación anterior de este recurso.
        Los intentos ya calificados no se ven afectados: conservan su propio
        snapshot de preguntas y opciones.
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
            <label htmlFor="quiz-title">Título</label>
            <input id="quiz-title" required value={title} onChange={(e) => setTitle(e.target.value)} />
          </div>
          <div className="row">
            <div className="form-field">
              <label htmlFor="pass-score">Nota mínima para aprobar (%)</label>
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
              <label htmlFor="time-limit">Límite de tiempo (minutos, opcional)</label>
              <input
                id="time-limit"
                type="number"
                min={1}
                value={timeLimitMinutes}
                onChange={(e) => setTimeLimitMinutes(e.target.value)}
              />
            </div>
            <div className="form-field">
              <label htmlFor="max-attempts">Intentos máximos (opcional)</label>
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
              <label htmlFor="feedback-policy">Retroalimentación</label>
              <select
                id="feedback-policy"
                value={feedbackPolicy}
                onChange={(e) => setFeedbackPolicy(e.target.value as typeof feedbackPolicy)}
              >
                <option value="immediate">Inmediata</option>
                <option value="after_submit">Al enviar el intento</option>
                <option value="after_close">Al cerrarse el quiz</option>
                <option value="none">Nunca</option>
              </select>
            </div>
            <label className="row" style={{ alignItems: "center" }}>
              <input
                type="checkbox"
                checked={shuffleQuestions}
                onChange={(e) => setShuffleQuestions(e.target.checked)}
              />
              Mezclar el orden de las preguntas
            </label>
          </div>
        </div>

        <h2>Preguntas</h2>
        <ol className="stack">
          {questions.map((q, qIndex) => (
            <li key={qIndex} className="card stack">
              <div className="row" style={{ justifyContent: "space-between" }}>
                <strong>Pregunta {qIndex + 1}</strong>
                {questions.length > 1 && (
                  <button type="button" className="danger" onClick={() => removeQuestion(qIndex)}>
                    Eliminar pregunta
                  </button>
                )}
              </div>

              <div className="form-field">
                <label htmlFor={`prompt-${qIndex}`}>Enunciado</label>
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
                  <label htmlFor={`type-${qIndex}`}>Tipo</label>
                  <select
                    id={`type-${qIndex}`}
                    value={q.type}
                    onChange={(e) => changeQuestionType(qIndex, e.target.value as "single" | "multiple")}
                  >
                    <option value="single">Respuesta única</option>
                    <option value="multiple">Respuesta múltiple</option>
                  </select>
                </div>
                <div className="form-field">
                  <label htmlFor={`points-${qIndex}`}>Puntos</label>
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
                Opciones (marca la correcta{q.type === "multiple" ? "s" : ""}):
              </p>
              <ul className="stack" style={{ gap: "0.35rem" }}>
                {q.options.map((o, oIndex) => (
                  <li key={oIndex} className="row" style={{ alignItems: "center" }}>
                    <input
                      type={q.type === "single" ? "radio" : "checkbox"}
                      name={`correcta-${qIndex}`}
                      checked={o.is_correct}
                      onChange={(e) => updateOption(qIndex, oIndex, { is_correct: e.target.checked })}
                    />
                    <input
                      style={{ flex: 1 }}
                      required
                      placeholder={`Opción ${oIndex + 1}`}
                      value={o.text_md}
                      onChange={(e) => updateOption(qIndex, oIndex, { text_md: e.target.value })}
                    />
                    {q.options.length > 2 && (
                      <button type="button" className="danger" onClick={() => removeOption(qIndex, oIndex)}>
                        Quitar
                      </button>
                    )}
                  </li>
                ))}
              </ul>
              <button type="button" className="secondary" onClick={() => addOption(qIndex)}>
                Agregar opción
              </button>
            </li>
          ))}
        </ol>

        <button type="button" className="secondary" onClick={addQuestion}>
          Agregar pregunta
        </button>

        <button type="submit" disabled={saving || !puedeGuardar}>
          {saving ? "Guardando…" : "Guardar evaluación"}
        </button>
      </form>
    </div>
  );
}

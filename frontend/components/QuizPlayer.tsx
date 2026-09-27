"use client";

import { useEffect, useMemo, useState } from "react";
import { api, ApiError, type QuizAttempt, type QuizQuestion } from "@/lib/api";
import { useI18n, useTraductorEstable } from "@/lib/i18n";

/**
 * Reproductor de un quiz para el estudiante.
 *
 * Al montar, abre un intento (o recupera el que ya estuviera en curso) y
 * muestra las preguntas en el orden congelado en ese momento. Cada cambio de
 * selección se guarda de inmediato en el servidor: nada se acumula solo en
 * memoria del navegador, así que recargar la página no pierde nada.
 *
 * La clave correcta nunca llega aquí: el servidor solo entrega el texto de
 * las opciones, nunca cuál es la correcta.
 */
export function QuizPlayer({ resourceId }: { resourceId: string }) {
  const { t } = useI18n();
  // Abrir el intento no debe repetirse porque alguien cambie de idioma.
  const traducir = useTraductorEstable();
  const [attempt, setAttempt] = useState<QuizAttempt | null>(null);
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [savingQuestion, setSavingQuestion] = useState<string | null>(null);

  useEffect(() => {
    let cancelado = false;
    api
      .startQuizAttempt(resourceId)
      .then((a) => {
        if (!cancelado) setAttempt(a);
      })
      .catch((e) => {
        if (!cancelado) {
          setError(e instanceof ApiError ? e.message : traducir("quiz.errorAbrir"));
        }
      });
    return () => {
      cancelado = true;
    };
  }, [resourceId, traducir]);

  async function handleSelect(question: QuizQuestion, optionStableId: string) {
    if (!attempt) return;
    const previa = attempt.answers[question.stable_id] ?? [];
    let seleccion: string[];
    if (question.type === "single") {
      seleccion = [optionStableId];
    } else {
      seleccion = previa.includes(optionStableId)
        ? previa.filter((id) => id !== optionStableId)
        : [...previa, optionStableId];
    }

    // Optimista: se refleja de inmediato y se corrige si el servidor rechaza.
    setAttempt({ ...attempt, answers: { ...attempt.answers, [question.stable_id]: seleccion } });
    setSavingQuestion(question.stable_id);
    setError("");
    try {
      await api.saveQuizAnswer(attempt.attempt_id, question.stable_id, seleccion);
    } catch (e) {
      setAttempt({ ...attempt, answers: { ...attempt.answers, [question.stable_id]: previa } });
      setError(e instanceof ApiError ? e.message : t("quiz.errorGuardarRespuesta"));
    } finally {
      setSavingQuestion(null);
    }
  }

  // Una clave estable por intento hace que reintentar tras un corte de red
  // (el usuario pulsa "Enviar" dos veces, o el navegador reintenta solo)
  // devuelva la misma nota en vez de calificar dos veces.
  const attemptId = attempt?.attempt_id;
  const idempotencyKey = useMemo(() => (attemptId ? `quiz-submit-${attemptId}` : ""), [attemptId]);

  async function handleSubmit() {
    if (!attempt) return;
    const faltantes = attempt.questions.filter((q) => (attempt.answers[q.stable_id] ?? []).length === 0);
    if (
      faltantes.length > 0 &&
      !confirm(t("quiz.confirmarFaltantes", { n: faltantes.length }))
    ) {
      return;
    }
    setSubmitting(true);
    setError("");
    try {
      const resultado = await api.submitQuizAttempt(attempt.attempt_id, idempotencyKey);
      setAttempt(resultado);
      // El avance del curso (completado, aprobado, insignia) se recalcula al
      // procesar una señal de progreso. Se fuerza aquí, en vez de esperar a
      // que el estudiante abandone la página, para que quien consulte el
      // resumen justo después del envío ya vea el resultado actualizado.
      void api.recordProgress(resourceId, "close", true).catch(() => undefined);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : t("quiz.errorEnviar"));
    } finally {
      setSubmitting(false);
    }
  }

  if (error && !attempt) {
    return (
      <p className="error-banner" role="alert">
        {error}
      </p>
    );
  }
  if (!attempt) {
    return <p role="status">{t("quiz.abriendo")}</p>;
  }

  const cerrado = attempt.status !== "in_progress";

  return (
    <div className="stack">
      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}

      {cerrado && (
        <div className={attempt.passed ? "success-banner" : "warning-banner"} role="status">
          <strong>
            {attempt.status === "expired" ? t("quiz.intentoExpirado") : t("quiz.intentoCalificado")}
          </strong>
          {typeof attempt.score === "number" && (
            <>
              {" "}
              {t("quiz.nota", { nota: attempt.score.toFixed(1) })}{" "}
              {attempt.passed ? t("quiz.aprobaste") : t("quiz.noAprobaste")}
            </>
          )}
        </div>
      )}

      <ol className="stack">
        {attempt.questions.map((q, i) => (
          <li key={q.stable_id} className="card">
            <p>
              <strong>
                {i + 1}. {q.prompt_md}
              </strong>{" "}
              <span className="muted">
                ({q.points === 1 ? t("quiz.unPunto") : t("quiz.nPuntos", { n: q.points })})
              </span>
            </p>
            <fieldset disabled={cerrado || savingQuestion === q.stable_id} style={{ border: "none", padding: 0, margin: 0 }}>
              <ul className="stack" style={{ gap: "0.35rem" }}>
                {q.options.map((o) => {
                  const marcada = (attempt.answers[q.stable_id] ?? []).includes(o.stable_id);
                  return (
                    <li key={o.stable_id}>
                      <label className="row" style={{ gap: "0.5rem" }}>
                        <input
                          type={q.type === "single" ? "radio" : "checkbox"}
                          name={`pregunta-${q.stable_id}`}
                          checked={marcada}
                          onChange={() => handleSelect(q, o.stable_id)}
                        />
                        {o.text_md}
                      </label>
                    </li>
                  );
                })}
              </ul>
            </fieldset>
          </li>
        ))}
      </ol>

      {!cerrado && (
        <button onClick={handleSubmit} disabled={submitting}>
          {submitting ? t("quiz.enviando") : t("quiz.enviar")}
        </button>
      )}
    </div>
  );
}

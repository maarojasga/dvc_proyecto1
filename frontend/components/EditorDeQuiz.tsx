"use client";

import { useState } from "react";
import { api, ApiError, type DefinicionQuiz } from "@/lib/api";

/**
 * Autoría de una evaluación.
 *
 * La definición viaja completa en un PUT, incluida la clave correcta: es el
 * único momento en que esa información cruza la red, y solo en esta dirección.
 * El servidor responde con el identificador y el recuento de preguntas, nunca
 * con el quiz entero, para no devolver la clave ni al propio profesor.
 */

interface OpcionBorrador {
  text_md: string;
  is_correct: boolean;
}

interface PreguntaBorrador {
  prompt_md: string;
  type: "single" | "multiple";
  points: number;
  options: OpcionBorrador[];
}

const POLITICAS: [string, string][] = [
  ["immediate", "Mostrar la nota al enviar"],
  ["after_close", "Mostrarla cuando cierre el intento"],
  ["never", "No mostrarla"],
];

function preguntaVacia(): PreguntaBorrador {
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

export function EditorDeQuiz({
  versionId,
  resourceId,
  tituloRecurso,
  onGuardado,
}: {
  versionId: string;
  resourceId: string;
  tituloRecurso: string;
  onGuardado?: () => void;
}) {
  const [titulo, setTitulo] = useState(tituloRecurso);
  const [minutos, setMinutos] = useState("");
  const [intentos, setIntentos] = useState("");
  const [notaMinima, setNotaMinima] = useState("60");
  const [politica, setPolitica] = useState("immediate");
  const [mezclar, setMezclar] = useState(false);
  const [preguntas, setPreguntas] = useState<PreguntaBorrador[]>([preguntaVacia()]);
  const [guardando, setGuardando] = useState(false);
  const [error, setError] = useState("");
  const [aviso, setAviso] = useState("");

  function actualizarPregunta(i: number, cambios: Partial<PreguntaBorrador>) {
    setPreguntas((ps) => ps.map((p, j) => (j === i ? { ...p, ...cambios } : p)));
  }

  function actualizarOpcion(i: number, j: number, cambios: Partial<OpcionBorrador>) {
    setPreguntas((ps) =>
      ps.map((p, pi) => {
        if (pi !== i) return p;
        let options = p.options.map((o, oj) => (oj === j ? { ...o, ...cambios } : o));
        // En una pregunta de respuesta única, marcar una desmarca las demás:
        // si no, se podría guardar un quiz sin clave única y la calificación
        // no tendría sentido.
        if (p.type === "single" && cambios.is_correct) {
          options = options.map((o, oj) => ({ ...o, is_correct: oj === j }));
        }
        return { ...p, options };
      }),
    );
  }

  /** problemas son las razones por las que el quiz no se puede guardar. */
  function problemas(): string[] {
    const fallos: string[] = [];
    if (!titulo.trim()) fallos.push("El título es obligatorio.");
    if (!preguntas.length) fallos.push("Hace falta al menos una pregunta.");
    preguntas.forEach((p, i) => {
      const n = i + 1;
      if (!p.prompt_md.trim()) fallos.push(`Pregunta ${n}: falta el enunciado.`);
      const conTexto = p.options.filter((o) => o.text_md.trim());
      if (conTexto.length < 2) fallos.push(`Pregunta ${n}: hacen falta al menos dos opciones.`);
      const correctas = conTexto.filter((o) => o.is_correct).length;
      if (correctas === 0) fallos.push(`Pregunta ${n}: ninguna opción está marcada como correcta.`);
      if (p.type === "single" && correctas > 1) {
        fallos.push(`Pregunta ${n}: es de respuesta única y tiene ${correctas} correctas.`);
      }
      if (p.points <= 0) fallos.push(`Pregunta ${n}: el puntaje debe ser mayor que cero.`);
    });
    return fallos;
  }

  async function guardar(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setAviso("");
    const fallos = problemas();
    if (fallos.length) {
      setError(fallos.join(" "));
      return;
    }
    setGuardando(true);
    const def: DefinicionQuiz = {
      title: titulo.trim(),
      time_limit_seconds: minutos ? Number(minutos) * 60 : null,
      max_attempts: intentos ? Number(intentos) : null,
      pass_score: Number(notaMinima),
      feedback_policy: politica,
      shuffle_questions: mezclar,
      questions: preguntas.map((p) => ({
        prompt_md: p.prompt_md.trim(),
        type: p.type,
        points: p.points,
        options: p.options
          .filter((o) => o.text_md.trim())
          .map((o) => ({ text_md: o.text_md.trim(), is_correct: o.is_correct })),
      })),
    };
    try {
      const r = await api.definirQuiz(versionId, resourceId, def);
      setAviso(`Evaluación guardada con ${r.questions} pregunta(s).`);
      onGuardado?.();
    } catch (err) {
      setError(
        err instanceof ApiError ? [err.message, ...(err.details ?? [])].join(" ") : "No se pudo guardar",
      );
    } finally {
      setGuardando(false);
    }
  }

  return (
    <form className="stack card" onSubmit={guardar}>
      <h3>Evaluación</h3>

      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}
      {aviso && (
        <p className="success-banner" role="status">
          {aviso}
        </p>
      )}

      <div className="form-field">
        <label htmlFor={`quiz-titulo-${resourceId}`}>Título</label>
        <input
          id={`quiz-titulo-${resourceId}`}
          value={titulo}
          onChange={(e) => setTitulo(e.target.value)}
          required
        />
      </div>

      <div className="rejilla">
        <div className="form-field">
          <label htmlFor={`quiz-minutos-${resourceId}`}>Límite de tiempo (minutos)</label>
          <input
            id={`quiz-minutos-${resourceId}`}
            type="number"
            min={1}
            value={minutos}
            onChange={(e) => setMinutos(e.target.value)}
            placeholder="sin límite"
          />
        </div>
        <div className="form-field">
          <label htmlFor={`quiz-intentos-${resourceId}`}>Intentos permitidos</label>
          <input
            id={`quiz-intentos-${resourceId}`}
            type="number"
            min={1}
            value={intentos}
            onChange={(e) => setIntentos(e.target.value)}
            placeholder="sin límite"
          />
        </div>
        <div className="form-field">
          <label htmlFor={`quiz-nota-${resourceId}`}>Nota mínima para aprobar</label>
          <input
            id={`quiz-nota-${resourceId}`}
            type="number"
            min={0}
            max={100}
            value={notaMinima}
            onChange={(e) => setNotaMinima(e.target.value)}
          />
        </div>
        <div className="form-field">
          <label htmlFor={`quiz-politica-${resourceId}`}>Retroalimentación</label>
          <select
            id={`quiz-politica-${resourceId}`}
            value={politica}
            onChange={(e) => setPolitica(e.target.value)}
          >
            {POLITICAS.map(([v, t]) => (
              <option key={v} value={v}>
                {t}
              </option>
            ))}
          </select>
        </div>
      </div>

      <label className="row" style={{ marginBottom: 0 }}>
        <input type="checkbox" checked={mezclar} onChange={(e) => setMezclar(e.target.checked)} />
        <span>Mezclar el orden de las preguntas en cada intento</span>
      </label>

      <ol className="stack">
        {preguntas.map((p, i) => (
          <li key={i} className="card">
            <fieldset style={{ border: 0, margin: 0, padding: 0 }}>
              <legend>
                <strong>Pregunta {i + 1}</strong>
              </legend>

              <div className="form-field">
                <label htmlFor={`p-${resourceId}-${i}`}>Enunciado</label>
                <textarea
                  id={`p-${resourceId}-${i}`}
                  rows={2}
                  value={p.prompt_md}
                  onChange={(e) => actualizarPregunta(i, { prompt_md: e.target.value })}
                />
              </div>

              <div className="row">
                <div className="form-field">
                  <label htmlFor={`tipo-${resourceId}-${i}`}>Tipo</label>
                  <select
                    id={`tipo-${resourceId}-${i}`}
                    value={p.type}
                    onChange={(e) => {
                      const type = e.target.value as "single" | "multiple";
                      // Al pasar a respuesta única se conserva solo la primera
                      // correcta, para no dejar una clave ambigua.
                      const options =
                        type === "single"
                          ? p.options.map((o, oj) => ({
                              ...o,
                              is_correct: oj === p.options.findIndex((x) => x.is_correct),
                            }))
                          : p.options;
                      actualizarPregunta(i, { type, options });
                    }}
                  >
                    <option value="single">Respuesta única</option>
                    <option value="multiple">Respuesta múltiple</option>
                  </select>
                </div>
                <div className="form-field">
                  <label htmlFor={`pts-${resourceId}-${i}`}>Puntaje</label>
                  <input
                    id={`pts-${resourceId}-${i}`}
                    type="number"
                    min={1}
                    value={p.points}
                    onChange={(e) => actualizarPregunta(i, { points: Number(e.target.value) })}
                  />
                </div>
              </div>

              <div className="stack" style={{ gap: "0.35rem" }}>
                {p.options.map((o, j) => (
                  <div key={j} className="row" style={{ marginBottom: 0 }}>
                    <input
                      type={p.type === "multiple" ? "checkbox" : "radio"}
                      name={`correcta-${resourceId}-${i}`}
                      checked={o.is_correct}
                      aria-label={`Opción ${j + 1} de la pregunta ${i + 1} es correcta`}
                      onChange={(e) => actualizarOpcion(i, j, { is_correct: e.target.checked })}
                    />
                    <input
                      style={{ flex: 1 }}
                      value={o.text_md}
                      aria-label={`Texto de la opción ${j + 1} de la pregunta ${i + 1}`}
                      placeholder={`Opción ${j + 1}`}
                      onChange={(e) => actualizarOpcion(i, j, { text_md: e.target.value })}
                    />
                    {p.options.length > 2 && (
                      <button
                        type="button"
                        className="danger"
                        onClick={() =>
                          actualizarPregunta(i, { options: p.options.filter((_, oj) => oj !== j) })
                        }
                      >
                        Quitar
                      </button>
                    )}
                  </div>
                ))}
                <button
                  type="button"
                  onClick={() =>
                    actualizarPregunta(i, { options: [...p.options, { text_md: "", is_correct: false }] })
                  }
                >
                  Añadir opción
                </button>
              </div>

              {preguntas.length > 1 && (
                <button
                  type="button"
                  className="danger"
                  style={{ marginTop: "0.5rem" }}
                  onClick={() => setPreguntas((ps) => ps.filter((_, j) => j !== i))}
                >
                  Eliminar la pregunta {i + 1}
                </button>
              )}
            </fieldset>
          </li>
        ))}
      </ol>

      <div className="row">
        <button type="button" onClick={() => setPreguntas((ps) => [...ps, preguntaVacia()])}>
          Añadir pregunta
        </button>
        <button type="submit" disabled={guardando}>
          {guardando ? "Guardando…" : "Guardar la evaluación"}
        </button>
      </div>
    </form>
  );
}

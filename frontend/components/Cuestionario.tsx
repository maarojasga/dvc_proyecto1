"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { api, ApiError, type VistaIntento } from "@/lib/api";

/**
 * Cuestionario presenta un intento de quiz.
 *
 * Dos cosas que no hace, y son deliberadas:
 *
 *  - No calcula la nota. Las opciones que llegan no dicen cuál es correcta, así
 *    que aquí no hay con qué calcularla ni con qué adivinarla. La nota llega
 *    del servidor al enviar.
 *  - No decide cuándo expira. El servidor rechaza los guardados fuera de
 *    plazo; el reloj de abajo es informativo, y si el navegador va desfasado el
 *    veredicto sigue siendo del servidor.
 */

/** INTERVALO_GUARDADO es la pausa tras marcar antes de guardar en el servidor. */
const INTERVALO_GUARDADO = 600;

type EstadoGuardado = "sin_cambios" | "guardando" | "guardado" | "fallido";

export function Cuestionario({
  resourceId,
  onCerrado,
}: {
  resourceId: string;
  /** onCerrado avisa al enviar, para que la página refresque el avance: tras
      aprobar una evaluación el resumen que se estaba mostrando queda obsoleto. */
  onCerrado?: () => void;
}) {
  const [intento, setIntento] = useState<VistaIntento | null>(null);
  const [error, setError] = useState("");
  const [guardado, setGuardado] = useState<EstadoGuardado>("sin_cambios");
  const [enviando, setEnviando] = useState(false);
  const [ahora, setAhora] = useState(() => Date.now());

  // La clave de idempotencia se fija al empezar y se reutiliza en cada
  // reintento de envío: es lo que hace que un corte de red al enviar no gaste
  // otro intento ni produzca otra nota.
  const claveEnvio = useRef<string>("");
  const temporizadores = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());

  useEffect(() => {
    let vivo = true;
    // El mapa se copia a una variable del efecto: en la limpieza, la
    // referencia puede apuntar ya a otra cosa.
    const pendientes = temporizadores.current;
    api
      .iniciarIntento(resourceId)
      .then((v) => {
        if (!vivo) return;
        setIntento(v);
        claveEnvio.current = `${v.attempt_id}:${crypto.randomUUID()}`;
      })
      .catch((e) => vivo && setError(e instanceof ApiError ? e.message : "No se pudo iniciar el intento"));
    return () => {
      vivo = false;
      pendientes.forEach((t) => clearTimeout(t));
    };
  }, [resourceId]);

  // El reloj solo se mueve mientras hay plazo que mostrar.
  useEffect(() => {
    if (!intento?.expires_at || intento.status !== "in_progress") return;
    const t = setInterval(() => setAhora(Date.now()), 1000);
    return () => clearInterval(t);
  }, [intento?.expires_at, intento?.status]);

  const guardar = useCallback(
    (attemptId: string, preguntaId: string, seleccion: string[]) => {
      const previo = temporizadores.current.get(preguntaId);
      if (previo) clearTimeout(previo);
      setGuardado("guardando");
      temporizadores.current.set(
        preguntaId,
        setTimeout(async () => {
          try {
            await api.guardarRespuesta(attemptId, preguntaId, seleccion);
            setGuardado("guardado");
          } catch {
            // El guardado parcial que falla no borra lo marcado en pantalla,
            // pero hay que decirlo: el servidor no lo tiene.
            setGuardado("fallido");
          }
        }, INTERVALO_GUARDADO),
      );
    },
    [],
  );

  function marcar(preguntaId: string, opcionId: string, tipo: string) {
    if (!intento || intento.status !== "in_progress") return;
    const actuales = intento.answers[preguntaId] ?? [];
    const seleccion =
      tipo === "multiple"
        ? actuales.includes(opcionId)
          ? actuales.filter((o) => o !== opcionId)
          : [...actuales, opcionId]
        : [opcionId];
    setIntento({ ...intento, answers: { ...intento.answers, [preguntaId]: seleccion } });
    guardar(intento.attempt_id, preguntaId, seleccion);
  }

  async function enviar() {
    if (!intento) return;
    setEnviando(true);
    setError("");
    // Los guardados en vuelo se fuerzan antes de enviar: si no, la última
    // respuesta marcada podría no haber llegado.
    temporizadores.current.forEach((t) => clearTimeout(t));
    temporizadores.current.clear();
    try {
      for (const [preguntaId, seleccion] of Object.entries(intento.answers)) {
        await api.guardarRespuesta(intento.attempt_id, preguntaId, seleccion);
      }
      const resultado = await api.enviarIntento(intento.attempt_id, claveEnvio.current);
      setIntento(resultado);
      onCerrado?.();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "No se pudo enviar el intento");
    } finally {
      setEnviando(false);
    }
  }

  if (error && !intento) {
    return (
      <p className="error-banner" role="alert">
        {error}
      </p>
    );
  }
  if (!intento) return <p>Preparando la evaluación…</p>;

  const cerrado = intento.status !== "in_progress";
  const restante = intento.expires_at ? Math.max(0, new Date(intento.expires_at).getTime() - ahora) : null;
  const sinRespuesta = intento.questions.filter((p) => !(intento.answers[p.stable_id] ?? []).length).length;

  return (
    <div className="stack">
      <header className="page-header">
        <div>
          {/* El título del recurso lo pinta la página que incrusta esto, así
              que aquí solo va el estado del intento. */}
          <p className="muted">
            Intento {intento.attempt_number}
            {restante !== null && !cerrado && ` · quedan ${formatearPlazo(restante)}`}
          </p>
        </div>
        {!cerrado && (
          <span className="badge" role="status" aria-live="polite">
            {textoDeGuardado(guardado)}
          </span>
        )}
      </header>

      {guardado === "fallido" && !cerrado && (
        <p className="warning-banner" role="alert">
          La última respuesta no llegó al servidor. Se reintenta al enviar; si el
          problema sigue, no cierres la página.
        </p>
      )}

      {cerrado && <Resultado intento={intento} />}

      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}

      {/* Sin viñeta ni número: el enunciado ya lleva su número, y el lector de
          pantalla anunciaría dos. */}
      <ol className="stack" style={{ listStyle: "none", paddingLeft: 0 }}>
        {intento.questions.map((p, i) => {
          const seleccion = intento.answers[p.stable_id] ?? [];
          const multiple = p.type === "multiple";
          return (
            <li key={p.stable_id} className="card">
              {/* fieldset y legend para que un lector de pantalla anuncie a qué
                  pregunta pertenece cada opción. */}
              <fieldset style={{ border: 0, margin: 0, padding: 0 }}>
                <legend>
                  <strong>
                    {i + 1}. {p.prompt_md}
                  </strong>{" "}
                  <span className="badge">{p.points} pts</span>
                  {multiple && <span className="badge">selección múltiple</span>}
                </legend>
                <div className="stack" style={{ gap: "0.35rem", marginTop: "0.5rem" }}>
                  {p.options.map((o) => (
                    <label key={o.stable_id} className="row" style={{ marginBottom: 0, alignItems: "flex-start" }}>
                      <input
                        type={multiple ? "checkbox" : "radio"}
                        name={p.stable_id}
                        value={o.stable_id}
                        checked={seleccion.includes(o.stable_id)}
                        disabled={cerrado}
                        onChange={() => marcar(p.stable_id, o.stable_id, p.type)}
                      />
                      <span>{o.text_md}</span>
                    </label>
                  ))}
                </div>
              </fieldset>
            </li>
          );
        })}
      </ol>

      {!cerrado && (
        <div className="card">
          {sinRespuesta > 0 && (
            <p className="muted">
              {sinRespuesta === 1
                ? "Queda 1 pregunta sin responder."
                : `Quedan ${sinRespuesta} preguntas sin responder.`}
            </p>
          )}
          <button onClick={enviar} disabled={enviando}>
            {enviando ? "Enviando…" : "Enviar la evaluación"}
          </button>
        </div>
      )}
    </div>
  );
}

/**
 * Resultado muestra la nota cuando la política de retroalimentación del quiz
 * permite revelarla. Si el profesor la configuró para no mostrarla, el
 * servidor no manda score ni passed, y aquí se dice eso en lugar de inventar
 * un cero.
 */
function Resultado({ intento }: { intento: VistaIntento }) {
  if (intento.score === undefined) {
    return (
      <p className="success-banner" role="status">
        Evaluación enviada. El resultado no se muestra de inmediato en este curso.
      </p>
    );
  }
  return (
    <p className={intento.passed ? "success-banner" : "warning-banner"} role="status">
      Nota: <strong>{intento.score.toFixed(1)}</strong> ·{" "}
      {intento.passed ? "aprobada" : "no alcanza el mínimo del curso"}
    </p>
  );
}

function textoDeGuardado(e: EstadoGuardado): string {
  switch (e) {
    case "sin_cambios":
      return "Sin cambios";
    case "guardando":
      return "Guardando…";
    case "guardado":
      return "Respuestas guardadas";
    case "fallido":
      return "Sin guardar";
  }
}

function formatearPlazo(ms: number): string {
  const total = Math.floor(ms / 1000);
  const min = Math.floor(total / 60);
  const seg = total % 60;
  return `${min}:${String(seg).padStart(2, "0")}`;
}

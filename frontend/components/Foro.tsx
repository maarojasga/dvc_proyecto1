"use client";

import { useCallback, useEffect, useState } from "react";

import { api, ApiError, type Hilo, type RespuestaDeForo } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

/**
 * Foro asíncrono de un curso.
 *
 * Se puede acotar a un recurso concreto, y entonces enseña solo lo que se
 * discute sobre esa lección. El hilo se ata al stable_id del recurso y no a su
 * identificador de fila: una conversación sobre una lección sigue valiendo
 * cuando el profesor publica una versión nueva del curso.
 */
export function Foro({
  courseId,
  resourceStableId,
  puedeModerar = false,
}: {
  courseId: string;
  resourceStableId?: string;
  /** El profesor del curso puede cerrar hilos y borrar respuestas ajenas. */
  puedeModerar?: boolean;
}) {
  const { t, locale } = useI18n();
  const [hilos, setHilos] = useState<Hilo[] | null>(null);
  const [abierto, setAbierto] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [titulo, setTitulo] = useState("");
  const [cuerpo, setCuerpo] = useState("");
  const [ocupado, setOcupado] = useState(false);

  const cargar = useCallback(async () => {
    try {
      const res = await api.listThreads(courseId, resourceStableId);
      setHilos(res.items ?? []);
    } catch {
      // Sin acceso al foro la sección simplemente no se pinta.
      setHilos(null);
    }
  }, [courseId, resourceStableId]);

  useEffect(() => {
    cargar();
  }, [cargar]);

  if (hilos === null) return null;

  async function abrirHilo(e: React.FormEvent) {
    e.preventDefault();
    setOcupado(true);
    setError("");
    try {
      await api.createThread(courseId, {
        title: titulo,
        body_md: cuerpo,
        resource_stable_id: resourceStableId,
      });
      setTitulo("");
      setCuerpo("");
      await cargar();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("foro.errorAbrir"));
    } finally {
      setOcupado(false);
    }
  }

  return (
    <section className="card stack">
      <header>
        <h2>{resourceStableId ? t("foro.dudasLeccion") : t("foro.titulo")}</h2>
      </header>

      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}

      <form className="stack" onSubmit={abrirHilo}>
        <div className="form-field">
          <label htmlFor="hilo-titulo">{t("comun.titulo")}</label>
          <input
            id="hilo-titulo"
            value={titulo}
            onChange={(e) => setTitulo(e.target.value)}
            placeholder={t("foro.tituloPlaceholder")}
            required
          />
        </div>
        <div className="form-field">
          <label htmlFor="hilo-cuerpo">{t("foro.mensaje")}</label>
          <textarea
            id="hilo-cuerpo"
            rows={3}
            value={cuerpo}
            onChange={(e) => setCuerpo(e.target.value)}
            required
          />
        </div>
        <div>
          <button type="submit" disabled={ocupado || !titulo.trim() || !cuerpo.trim()}>
            {t("foro.abrirHilo")}
          </button>
        </div>
      </form>

      {hilos.length === 0 ? (
        <p className="muted">{t("foro.vacio")}</p>
      ) : (
        <ul className="lista-filas">
          {hilos.map((h) => (
            <li key={h.id} className="fila" style={{ flexDirection: "column", alignItems: "stretch" }}>
              <div className="row" style={{ alignItems: "baseline" }}>
                <button
                  className="secondary"
                  onClick={() => setAbierto(abierto === h.id ? null : h.id)}
                  aria-expanded={abierto === h.id}
                  style={{ flex: 1, textAlign: "left" }}
                >
                  <strong>{h.title}</strong>
                </button>
                {h.locked && <span className="badge">{t("foro.cerrado")}</span>}
                <span className="badge">
                  {h.replies === 1 ? t("foro.unaRespuesta") : t("foro.nRespuestas", { n: h.replies })}
                </span>
              </div>
              <p className="muted">
                {h.author_name} · {new Date(h.last_activity_at).toLocaleString(locale)}
              </p>
              {abierto === h.id && (
                <HiloAbierto threadId={h.id} puedeModerar={puedeModerar} onCambio={cargar} />
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function HiloAbierto({
  threadId,
  puedeModerar,
  onCambio,
}: {
  threadId: string;
  puedeModerar: boolean;
  onCambio: () => void;
}) {
  const { t, locale } = useI18n();
  const [datos, setDatos] = useState<{ thread: Hilo; replies: RespuestaDeForo[] } | null>(null);
  const [texto, setTexto] = useState("");
  const [error, setError] = useState("");
  const [ocupado, setOcupado] = useState(false);

  const cargar = useCallback(async () => {
    try {
      setDatos(await api.getThread(threadId));
    } catch (e) {
      setError(e instanceof ApiError ? e.message : t("foro.errorAbrir"));
    }
  }, [threadId, t]);

  useEffect(() => {
    cargar();
  }, [cargar]);

  if (!datos) return null;

  async function responder(e: React.FormEvent) {
    e.preventDefault();
    setOcupado(true);
    setError("");
    try {
      await api.replyToThread(threadId, texto);
      setTexto("");
      await cargar();
      onCambio();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("foro.errorResponder"));
    } finally {
      setOcupado(false);
    }
  }

  async function cerrar(locked: boolean) {
    try {
      await api.lockThread(threadId, locked);
      await cargar();
      onCambio();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("foro.errorEstado"));
    }
  }

  async function borrar(postId: string) {
    try {
      await api.deleteForumPost(postId);
      await cargar();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("foro.errorBorrar"));
    }
  }

  return (
    <div className="stack" style={{ paddingLeft: "1rem", borderLeft: "3px solid var(--color-border, #e5e7eb)" }}>
      <p style={{ whiteSpace: "pre-wrap" }}>{datos.thread.body_md}</p>

      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}

      <ul className="stack">
        {datos.replies.map((r) => (
          <li key={r.id} className="card">
            {r.deleted ? (
              // La respuesta borrada conserva su lugar: quitarla dejaría sin
              // sentido a las que le respondían.
              <p className="muted">
                <em>{t("foro.eliminado")}</em>
              </p>
            ) : (
              <>
                <p style={{ whiteSpace: "pre-wrap", margin: 0 }}>{r.body_md}</p>
                <p className="muted" style={{ margin: "0.25rem 0 0" }}>
                  {r.author_name} · {new Date(r.created_at).toLocaleString(locale)}{" "}
                  <button className="secondary" onClick={() => borrar(r.id)}>
                    {t("foro.borrar")}
                  </button>
                </p>
              </>
            )}
          </li>
        ))}
      </ul>

      {datos.thread.locked ? (
        <p className="muted">{t("foro.hiloCerrado")}</p>
      ) : (
        <form className="stack" onSubmit={responder}>
          <div className="form-field">
            <label htmlFor={`respuesta-${threadId}`}>{t("foro.responder")}</label>
            <textarea
              id={`respuesta-${threadId}`}
              rows={2}
              value={texto}
              onChange={(e) => setTexto(e.target.value)}
              required
            />
          </div>
          <div>
            <button type="submit" disabled={ocupado || !texto.trim()}>
              {t("foro.responder")}
            </button>
          </div>
        </form>
      )}

      {puedeModerar && (
        <div>
          <button className="secondary" onClick={() => cerrar(!datos.thread.locked)}>
            {datos.thread.locked ? t("foro.reabrir") : t("foro.cerrarHilo")}
          </button>
        </div>
      )}
    </div>
  );
}

"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { api, ApiError, type ResourceContent } from "@/lib/api";
import { ReproductorHLS } from "@/components/ReproductorHLS";
import { VisorPDF } from "@/components/VisorPDF";
import { Cuestionario } from "@/components/Cuestionario";
import { useLatidosDeProgreso } from "@/lib/progreso";

/**
 * Consumo de un recurso por parte del estudiante.
 *
 * La API decide qué se puede entregar y en qué forma; aquí solo se elige el
 * visor según el tipo. Si el recurso no está disponible todavía o no hay
 * derecho de acceso, se explica en lugar de dejar la pantalla en blanco.
 */
export default function RecursoPage() {
  const params = useParams<{ courseId: string; resourceId: string }>();
  const { courseId, resourceId } = params;

  const [contenido, setContenido] = useState<ResourceContent | null>(null);
  const [error, setError] = useState("");
  const [aviso, setAviso] = useState("");

  useEffect(() => {
    let cancelado = false;
    (async () => {
      try {
        const c = await api.resourceContent(resourceId);
        if (!cancelado) setContenido(c);
      } catch (e) {
        if (cancelado) return;
        if (e instanceof ApiError && e.code === "media_not_ready") {
          setAviso("Este recurso todavía se está procesando. Vuelve en unos minutos.");
        } else if (e instanceof ApiError && e.status === 404) {
          setError("El recurso no existe o no tienes acceso a él.");
        } else {
          setError(e instanceof ApiError ? e.message : "No se pudo cargar el recurso");
        }
      }
    })();
    return () => {
      cancelado = true;
    };
  }, [resourceId]);

  // Reportar la posición no debe interrumpir la reproducción si falla: es una
  // comodidad, no parte del contenido.
  const ultimaReportada = useRef(-1);
  const reportarPosicion = useCallback(
    (segundos: number) => {
      if (segundos === ultimaReportada.current) return;
      ultimaReportada.current = segundos;
      void api.saveResourcePosition(resourceId, segundos).catch(() => undefined);
    },
    [resourceId],
  );

  // Los latidos son la evidencia de avance: el servidor decide con ellos, y el
  // cliente no manda porcentajes. Solo se emiten cuando hay contenido cargado,
  // porque antes no hay nada que se esté consumiendo.
  const { resumen, refrescar } = useLatidosDeProgreso(courseId, resourceId, Boolean(contenido) && !error && !aviso);

  if (error) {
    return (
      <div className="columna-estrecha">
        <p className="error-banner" role="alert">
          {error}
        </p>
        <p>
          <Link href={`/cursos/${courseId}`}>Volver al curso</Link>
        </p>
      </div>
    );
  }

  if (aviso) {
    return (
      <div className="columna-estrecha">
        <p className="warning-banner" role="status">
          {aviso}
        </p>
        <p>
          <Link href={`/cursos/${courseId}`}>Volver al curso</Link>
        </p>
      </div>
    );
  }

  if (!contenido) {
    return <p role="status">Cargando recurso…</p>;
  }

  return (
    <div>
      <header className="page-header">
        <div>
          <h1>{contenido.title}</h1>
          <p>
            <span className="badge">{contenido.type}</span>
          </p>
        </div>
        <Link href={`/cursos/${courseId}`}>Volver al curso</Link>
      </header>

      {(contenido.type === "video" || contenido.type === "audio") && contenido.url && (
        <>
          <ReproductorHLS
            src={contenido.url}
            tipo={contenido.type}
            titulo={contenido.title}
            posicionInicial={contenido.position_seconds ?? 0}
            onPosicion={reportarPosicion}
          />
          {(contenido.position_seconds ?? 0) > 0 && (
            <p className="muted">Se reanuda donde lo dejaste.</p>
          )}
        </>
      )}

      {contenido.type === "pdf" && contenido.url && (
        <VisorPDF src={contenido.url} titulo={contenido.title} descargable={contenido.downloadable} />
      )}

      {contenido.type === "image" && contenido.url && (
        // next/image exige declarar los hosts remotos en tiempo de
        // compilación, y aquí la URL viene firmada de un almacenamiento o un
        // CDN que se configura por entorno.
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={contenido.url}
          alt={contenido.title}
          style={{ maxWidth: "100%", borderRadius: "var(--radius)" }}
        />
      )}

      {contenido.type === "text" && contenido.markdown && (
        <article className="card">
          <pre style={{ whiteSpace: "pre-wrap", margin: 0, fontFamily: "inherit" }}>
            {contenido.markdown}
          </pre>
        </article>
      )}

      {(contenido.type === "link" || contenido.type === "iframe") && contenido.external_url && (
        <p>
          <a href={contenido.external_url} target="_blank" rel="noopener noreferrer">
            Abrir el recurso externo
          </a>
        </p>
      )}

      {contenido.type === "file" && contenido.url && (
        <p>
          <a href={contenido.url} target="_blank" rel="noopener noreferrer">
            Abrir el archivo
          </a>
        </p>
      )}

      {contenido.type === "quiz" && <Cuestionario resourceId={resourceId} onCerrado={refrescar} />}

      {resumen && (
        <p className="muted" role="status">
          Avance del curso: {Math.round(resumen.required_percent)}% de los recursos
          obligatorios ({resumen.required_completed} de {resumen.required_total})
          {resumen.quizzes_pending > 0 && ` · ${resumen.quizzes_pending} evaluación(es) pendiente(s)`}
        </p>
      )}
    </div>
  );
}

"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { api, ApiError, type ResourceContent } from "@/lib/api";
import { ReproductorHLS, type PistaDeSubtitulos } from "@/components/ReproductorHLS";
import { Foro } from "@/components/Foro";
import { VisorPDF } from "@/components/VisorPDF";
import { QuizPlayer } from "@/components/QuizPlayer";
import { MarcoIncrustado } from "@/components/MarcoIncrustado";
import { useProgressReporting } from "@/lib/useProgressReporting";
import { useI18n } from "@/lib/i18n";

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
  const { t } = useI18n();
  // Las pistas se piden aparte del contenido: el servidor las autoriza por la
  // misma vía, y un recurso sin subtítulos no debería fallar por eso.
  const [pistas, setPistas] = useState<PistaDeSubtitulos[]>([]);

  useEffect(() => {
    api
      .listCaptions(resourceId)
      .then((r) => setPistas(r.items ?? []))
      .catch(() => setPistas([]));
  }, [resourceId]);

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
          setAviso(t("recurso.procesando"));
        } else if (e instanceof ApiError && e.code === "not_enrolled") {
          // No inscrito también viaja como 404 —confirmar que la lección
          // existe delataría el contenido a quien no tiene derecho a verlo—,
          // pero para el estudiante es otra cosa: no le falta acceso, le falta
          // inscribirse, y decirle "no existe" lo deja sin saber qué hacer.
          setAviso(t("recurso.noInscrito"));
        } else if (e instanceof ApiError && e.status === 404) {
          setError(t("recurso.sinAcceso"));
        } else {
          setError(e instanceof ApiError ? e.message : t("recurso.error"));
        }
      }
    })();
    return () => {
      cancelado = true;
    };
  }, [resourceId, t]);

  // Solo se reporta progreso una vez confirmado el acceso: reportarlo antes
  // filtraría, por temporización, que el recurso existe aunque el estudiante
  // no tenga derecho a verlo.
  useProgressReporting(contenido ? resourceId : undefined);

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

  if (error) {
    return (
      <div className="columna-estrecha">
        <p className="error-banner" role="alert">
          {error}
        </p>
        <p>
          <Link href={`/cursos/${courseId}`}>{t("recurso.volver")}</Link>
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
          <Link href={`/cursos/${courseId}`}>{t("recurso.volver")}</Link>
        </p>
      </div>
    );
  }

  if (!contenido) {
    return <p role="status">{t("recurso.cargando")}</p>;
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
        <Link href={`/cursos/${courseId}`}>{t("recurso.volver")}</Link>
      </header>

      {(contenido.type === "video" || contenido.type === "audio") && contenido.url && (
        <>
          <ReproductorHLS
            src={contenido.url}
            tipo={contenido.type}
            titulo={contenido.title}
            posicionInicial={contenido.position_seconds ?? 0}
            onPosicion={reportarPosicion}
            pistas={pistas}
          />
          {(contenido.position_seconds ?? 0) > 0 && (
            <p className="muted">{t("recurso.reanuda")}</p>
          )}
          <Transcripcion resourceId={resourceId} pistas={pistas} />
        </>
      )}

      {contenido.type === "pdf" && contenido.url && (
        <VisorPDF src={contenido.url} titulo={contenido.title} descargable={contenido.downloadable} />
      )}

      {/* Una presentación se previsualiza con el mismo visor: lo que llega es
          el PDF que produjo la conversión, porque el navegador no abre un PPTX
          ni un ODP. El original sigue disponible cuando el profesor lo marcó
          descargable. */}
      {contenido.type === "presentation" && contenido.url && (
        <>
          <VisorPDF src={contenido.url} titulo={contenido.title} descargable={false} />
          {contenido.original_url && (
            <p>
              <a href={contenido.original_url} target="_blank" rel="noopener noreferrer">
                {t("recurso.descargarOriginal")}
              </a>
            </p>
          )}
        </>
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

      {contenido.type === "link" && contenido.external_url && (
        <p>
          <a href={contenido.external_url} target="_blank" rel="noopener noreferrer">
            {t("recurso.abrirExterno")}
          </a>
        </p>
      )}

      {contenido.type === "iframe" && contenido.external_url && (
        <MarcoIncrustado contenido={contenido} />
      )}

      {contenido.type === "file" && contenido.url && (
        <p>
          <a href={contenido.url} target="_blank" rel="noopener noreferrer">
            {t("recurso.abrirArchivo")}
          </a>
        </p>
      )}

      {contenido.type === "quiz" && <QuizPlayer resourceId={resourceId} />}

      {/* El foro va acotado a este recurso: junto a una lección interesa lo
          que se discute sobre ella, no el foro entero del curso. */}
      <Foro courseId={courseId} resourceStableId={contenido.stable_id} />
    </div>
  );
}

/**
 * Transcripción del material reproducible.
 *
 * Se deriva de los subtítulos, así que solo aparece cuando hay pista. Va
 * plegada porque es larga: lo que se quiere casi siempre es reproducir, y la
 * transcripción está para quien no puede o no quiere hacerlo, y para buscar
 * dentro de una clase grabada con la búsqueda del navegador.
 */
function Transcripcion({ resourceId, pistas }: { resourceId: string; pistas: PistaDeSubtitulos[] }) {
  const { t } = useI18n();
  const [abierta, setAbierta] = useState(false);
  const [texto, setTexto] = useState<string | null>(null);
  const idioma = pistas[0]?.language;

  useEffect(() => {
    if (!abierta || !idioma || texto !== null) return;
    api
      .getTranscript(resourceId, idioma)
      .then((r) => setTexto(r.transcript))
      .catch(() => setTexto(""));
  }, [abierta, idioma, resourceId, texto]);

  if (!idioma) return null;

  return (
    <section className="stack">
      <div>
        <button className="secondary" onClick={() => setAbierta(!abierta)} aria-expanded={abierta}>
          {abierta ? t("recurso.ocultarTranscripcion") : t("recurso.verTranscripcion")}
        </button>
      </div>
      {abierta && (
        <div className="card">
          {texto === null ? (
            <p role="status">{t("recurso.cargandoTranscripcion")}</p>
          ) : texto === "" ? (
            <p className="muted">{t("recurso.sinTranscripcion")}</p>
          ) : (
            <p style={{ whiteSpace: "pre-wrap", margin: 0 }}>{texto}</p>
          )}
        </div>
      )}
    </section>
  );
}

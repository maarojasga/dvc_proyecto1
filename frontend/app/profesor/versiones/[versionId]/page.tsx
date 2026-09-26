"use client";

import { useEffect, useState, useCallback } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { api, type Version, type Module, type Unit, type Clasificacion, ApiError } from "@/lib/api";
import { BlockEditor } from "@/components/BlockEditor";
import { HistorialDeRevisiones } from "@/components/HistorialDeRevisiones";
import { Coautoria } from "@/components/Coautoria";
import { subirArchivo, hayCargaPendiente, olvidarCarga, ErrorDeCarga } from "@/lib/carga";
import { useI18n, type Clave, type Traducir } from "@/lib/i18n";

// El valor es el que entiende la API; la clave, cómo se llama en pantalla.
const RESOURCE_TYPES: ReadonlyArray<readonly [string, Clave]> = [
  ["text", "tipo.text"],
  ["image", "tipo.image"],
  ["video", "tipo.video"],
  ["audio", "tipo.audio"],
  ["pdf", "tipo.pdf"],
  ["presentation", "tipo.presentation"],
  ["file", "tipo.file"],
  ["iframe", "tipo.iframe"],
  ["link", "tipo.link"],
  ["quiz", "tipo.quiz"],
];

/**
 * mensajeDeCarga traduce un fallo de la subida.
 *
 * lib/carga no ve el idioma, así que devuelve una clave con sus datos; aquí
 * se convierte en el texto que lee el profesor.
 */
function mensajeDeCarga(e: unknown, t: Traducir): string {
  if (e instanceof ErrorDeCarga) return t(`carga.error.${e.clave}`, e.datos);
  return e instanceof Error ? e.message : t("carga.error.generico");
}

const BINARY_TYPES = new Set(["image", "video", "audio", "pdf", "presentation", "file"]);

export default function VersionEditorPage() {
  const { versionId } = useParams<{ versionId: string }>();
  const { t } = useI18n();
  const [version, setVersion] = useState<Version | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [publishErrors, setPublishErrors] = useState<string[]>([]);

  const load = useCallback(async () => {
    try {
      const v = await api.getVersion(versionId);
      setVersion(v);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : t("version.error"));
    }
  }, [versionId, t]);

  useEffect(() => {
    load();
  }, [load]);

  async function handlePublish() {
    setPublishErrors([]);
    setNotice("");
    try {
      await api.publishVersion(versionId);
      setNotice(t("version.publicada"));
      load();
    } catch (e) {
      if (e instanceof ApiError && e.details?.length) {
        setPublishErrors(e.details);
      } else {
        setError(e instanceof ApiError ? e.message : t("version.errorPublicar"));
      }
    }
  }

  if (error) {
    return (
      <p className="error-banner" role="alert">
        {error}
      </p>
    );
  }
  if (!version) {
    return <p>{t("comun.cargando")}</p>;
  }

  const isDraft = version.Status === "draft";

  return (
    <div>
      <div className="row" style={{ justifyContent: "space-between", alignItems: "baseline" }}>
        <h1>{version.Title || t("version.sinTitulo")}</h1>
        <Link href={`/profesor/versiones/${versionId}/previsualizacion`}>
          {t("version.previsualizar")}
        </Link>
      </div>
      <p className="badge">
        {t("version.etiqueta", { n: version.VersionNumber, estado: version.Status })}
      </p>

      {/* Una sola advertencia: antes se pintaban dos banners seguidos, uno de
          aviso y otro de error, diciendo lo mismo con distinto color. */}
      {!isDraft && (
        <p className="warning-banner" role="status">
          {t("version.inmutable")}{" "}
          <Link href="/profesor">{t("version.irADespublicar")}</Link>
        </p>
      )}

      {notice && <p className="success-banner">{notice}</p>}
      {publishErrors.length > 0 && (
        <div className="error-banner" role="alert">
          <strong>{t("version.motivos")}</strong>
          <ul>
            {publishErrors.map((d, i) => (
              <li key={i}>{d}</li>
            ))}
          </ul>
        </div>
      )}

      <MetadataForm version={version} disabled={!isDraft} onSaved={load} />

      <Coautoria courseId={version.CourseID} />

      {isDraft && (
        <div className="card stack">
          <CambiosDelBorrador versionId={versionId} />
          <button onClick={handlePublish}>{t("version.publicar")}</button>
        </div>
      )}

      <h2>{t("version.estructura")}</h2>
      {isDraft && <AddModuleForm versionId={versionId} nextPosition={(version.Modules?.length ?? 0) + 1} onAdded={load} />}

      <ol className="stack">
        {version.Modules?.map((m) => (
          <ModuleCard key={m.ID} versionId={versionId} module={m} editable={isDraft} onChange={load} />
        ))}
      </ol>
    </div>
  );
}

function MetadataForm({ version, disabled, onSaved }: { version: Version; disabled: boolean; onSaved: () => void }) {
  const { t } = useI18n();
  const [form, setForm] = useState(version);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => setForm(version), [version]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    setError("");
    try {
      await api.updateVersionMetadata(version.ID, {
        title: form.Title,
        summary: form.Summary,
        description_md: form.DescriptionMD,
        category: form.Category,
        level: form.Level,
        language: form.Language,
        approval_min_score: Number(form.ApprovalMinScore),
        approval_required_resources_pct: Number(form.ApprovalRequiredResourcesPct),
      } as any);
      onSaved();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : t("version.errorGuardar"));
    } finally {
      setSaving(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="card stack">
      <h2 style={{ marginTop: 0 }}>{t("version.metadatos")}</h2>
      {error && <p className="error-banner">{error}</p>}
      <div className="form-field">
        <label htmlFor="title">{t("comun.titulo")}</label>
        <input id="title" disabled={disabled} value={form.Title} onChange={(e) => setForm({ ...form, Title: e.target.value })} />
      </div>
      <div className="form-field">
        <label htmlFor="summary">{t("version.resumen")}</label>
        <input
          id="summary"
          disabled={disabled}
          value={form.Summary}
          onChange={(e) => setForm({ ...form, Summary: e.target.value })}
        />
      </div>
      <div className="form-field">
        <label htmlFor="description">{t("version.descripcion")}</label>
        <textarea
          id="description"
          rows={5}
          disabled={disabled}
          value={form.DescriptionMD}
          onChange={(e) => setForm({ ...form, DescriptionMD: e.target.value })}
        />
      </div>
      <div className="row">
        <div className="form-field">
          <label htmlFor="category">{t("version.categoria")}</label>
          <input id="category" disabled={disabled} value={form.Category} onChange={(e) => setForm({ ...form, Category: e.target.value })} />
        </div>
        <div className="form-field">
          <label htmlFor="level">{t("version.nivel")}</label>
          <input id="level" disabled={disabled} value={form.Level} onChange={(e) => setForm({ ...form, Level: e.target.value })} />
        </div>
        <div className="form-field">
          <label htmlFor="language">{t("version.idioma")}</label>
          <input id="language" disabled={disabled} value={form.Language} onChange={(e) => setForm({ ...form, Language: e.target.value })} />
        </div>
      </div>
      <div className="row">
        <div className="form-field">
          <label htmlFor="minscore">{t("version.notaMinima")}</label>
          <input
            id="minscore"
            type="number"
            min={0}
            max={100}
            disabled={disabled}
            value={form.ApprovalMinScore}
            onChange={(e) => setForm({ ...form, ApprovalMinScore: Number(e.target.value) })}
          />
        </div>
        <div className="form-field">
          <label htmlFor="reqpct">{t("version.pctObligatorios")}</label>
          <input
            id="reqpct"
            type="number"
            min={0}
            max={100}
            disabled={disabled}
            value={form.ApprovalRequiredResourcesPct}
            onChange={(e) => setForm({ ...form, ApprovalRequiredResourcesPct: Number(e.target.value) })}
          />
        </div>
      </div>
      <button type="submit" disabled={disabled || saving}>
        {saving ? t("comun.guardando") : t("version.guardarMetadatos")}
      </button>
    </form>
  );
}

function AddModuleForm({ versionId, nextPosition, onAdded }: { versionId: string; nextPosition: number; onAdded: () => void }) {
  const { t } = useI18n();
  const [title, setTitle] = useState("");
  const [error, setError] = useState("");

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    try {
      await api.addModule(versionId, { title, position: nextPosition });
      setTitle("");
      onAdded();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : t("version.errorModulo"));
    }
  }

  return (
    <form onSubmit={handleSubmit} className="row card">
      {error && <p className="error-banner">{error}</p>}
      <div className="form-field" style={{ flex: 1, marginBottom: 0 }}>
        <label htmlFor="new-module">{t("version.nuevoModulo")}</label>
        <input id="new-module" required value={title} onChange={(e) => setTitle(e.target.value)} />
      </div>
      <button type="submit">{t("version.agregarModulo")}</button>
    </form>
  );
}

function ModuleCard({
  versionId,
  module: m,
  editable,
  onChange,
}: {
  versionId: string;
  module: Module;
  editable: boolean;
  onChange: () => void;
}) {
  const { t } = useI18n();

  async function handleDelete() {
    if (!confirm(t("version.confirmarModulo", { titulo: m.Title }))) return;
    await api.deleteModule(versionId, m.ID);
    onChange();
  }

  return (
    <li className="card">
      <div className="row">
        <h3 style={{ flex: 1 }}>{m.Title}</h3>
        {editable && (
          <button className="danger" onClick={handleDelete}>
            {t("version.eliminarModulo")}
          </button>
        )}
      </div>

      {editable && <AddUnitForm versionId={versionId} moduleId={m.ID} nextPosition={(m.Units?.length ?? 0) + 1} onAdded={onChange} />}

      <ol className="stack">
        {m.Units?.map((u) => (
          <UnitCard key={u.ID} versionId={versionId} unit={u} editable={editable} onChange={onChange} />
        ))}
      </ol>
    </li>
  );
}

function AddUnitForm({
  versionId,
  moduleId,
  nextPosition,
  onAdded,
}: {
  versionId: string;
  moduleId: string;
  nextPosition: number;
  onAdded: () => void;
}) {
  const { t } = useI18n();
  const [title, setTitle] = useState("");

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    await api.addUnit(versionId, moduleId, { title, position: nextPosition });
    setTitle("");
    onAdded();
  }

  return (
    <form onSubmit={handleSubmit} className="row">
      <div className="form-field" style={{ flex: 1, marginBottom: "0.5rem" }}>
        <label htmlFor={`new-unit-${moduleId}`}>{t("version.nuevaUnidad")}</label>
        <input id={`new-unit-${moduleId}`} required value={title} onChange={(e) => setTitle(e.target.value)} />
      </div>
      <button type="submit">{t("version.agregarUnidad")}</button>
    </form>
  );
}

function UnitCard({
  versionId,
  unit: u,
  editable,
  onChange,
}: {
  versionId: string;
  unit: Unit;
  editable: boolean;
  onChange: () => void;
}) {
  const { t } = useI18n();

  async function handleDelete() {
    if (!confirm(t("version.confirmarUnidad", { titulo: u.Title }))) return;
    await api.deleteUnit(versionId, u.ID);
    onChange();
  }

  return (
    <li className="card" style={{ background: "transparent" }}>
      <div className="row">
        <strong style={{ flex: 1 }}>{u.Title}</strong>
        {editable && (
          <button className="danger" onClick={handleDelete}>
            {t("version.eliminarUnidad")}
          </button>
        )}
      </div>

      <ul className="stack">
        {u.Resources?.map((r) => (
          <ResourceRow key={r.ID} versionId={versionId} resource={r} editable={editable} onChange={onChange} />
        ))}
      </ul>

      {editable && (
        <AddResourceForm versionId={versionId} unitId={u.ID} nextPosition={(u.Resources?.length ?? 0) + 1} onAdded={onChange} />
      )}
    </li>
  );
}

function ResourceRow({
  versionId,
  resource: r,
  editable,
  onChange,
}: {
  versionId: string;
  resource: import("@/lib/api").Resource;
  editable: boolean;
  onChange: () => void;
}) {
  const { t } = useI18n();
  const [uploading, setUploading] = useState(false);
  const [uploadPercent, setUploadPercent] = useState(0);
  const [uploadStatusText, setUploadStatusText] = useState("");
  const [error, setError] = useState("");
  // Una subida a medias de este recurso, detectada al montar. El archivo no
  // se puede recuperar —ningún navegador deja releer un File de otra sesión—,
  // así que lo que se ofrece es volver a elegirlo para continuar.
  const [pendiente, setPendiente] = useState(false);

  useEffect(() => {
    setPendiente(hayCargaPendiente(r.ID));
  }, [r.ID]);

  async function handleDelete() {
    if (!confirm(t("version.confirmarRecurso", { titulo: r.Title }))) return;
    await api.deleteResource(versionId, r.ID);
    onChange();
  }

  async function handleFile(file: File) {
    setUploading(true);
    setError("");
    setUploadPercent(0);
    setUploadStatusText(t("carga.fase.preparando"));

    try {
      await subirArchivo({
        versionId,
        resourceId: r.ID,
        archivo: file,
        alAvanzar: ({ porcentaje, fase, numero, total }) => {
          setUploadPercent(porcentaje);
          setUploadStatusText(
            numero === undefined || total === undefined
              ? t(`carga.fase.${fase}`)
              : t(`carga.fase.${fase}`, { numero, total }),
          );
        },
      });
      setPendiente(false);
      setTimeout(() => {
        setUploadStatusText("");
        setUploadPercent(0);
      }, 1500);
      onChange();
    } catch (e) {
      // La carga interrumpida sigue registrada: al volver a elegir el mismo
      // archivo se reanuda desde donde se quedó, sin reenviar lo que llegó.
      setPendiente(hayCargaPendiente(r.ID));
      setError(mensajeDeCarga(e, t));
    } finally {
      setUploading(false);
    }
  }

  function descartarCarga() {
    olvidarCarga(r.ID);
    setPendiente(false);
    setError("");
  }

  return (
    <li className="row" style={{ flexDirection: "column", alignItems: "stretch", gap: "0.4rem" }}>
      <div className="row" style={{ alignItems: "center" }}>
        <span style={{ flex: 1 }}>
          {r.Title} <span className="badge">{r.Type}</span>{" "}
          {r.ProcessingStatus !== "none" && (
            <span className="badge">
              {t("version.procesamiento", { estado: r.ProcessingStatus })}
            </span>
          )}
          {!r.Visible && <span className="badge">{t("version.oculto")}</span>}
        </span>
        {r.Type === "quiz" && (
          <Link href={`/profesor/versiones/${versionId}/quiz/${r.ID}`}>
            {t("version.definirEvaluacion")}
          </Link>
        )}
        {editable && BINARY_TYPES.has(r.Type) && (
          <label className="row" style={{ marginBottom: 0 }}>
            <span className="badge" style={{ cursor: uploading ? "not-allowed" : "pointer", background: uploading ? "#3b82f6" : undefined, color: uploading ? "white" : undefined }}>
              {uploading
                ? uploadStatusText || t("comun.cargando")
                : pendiente
                  ? t("version.reanudarSubida")
                  : t("version.subirArchivo")}
            </span>
            <input
              type="file"
              style={{ position: "absolute", width: 1, height: 1, overflow: "hidden", clip: "rect(0,0,0,0)" }}
              disabled={uploading}
              onChange={(e) => {
                const file = e.target.files?.[0];
                if (file) handleFile(file);
              }}
            />
          </label>
        )}
        {editable && (
          <button className="danger" onClick={handleDelete} disabled={uploading}>
            {t("comun.eliminar")}
          </button>
        )}
      </div>

      {editable && r.Type === "text" && (
        <EditorDeContenido versionId={versionId} resource={r} onChange={onChange} />
      )}

      {pendiente && !uploading && (
        <p className="muted" role="status">
          {t("version.cargaPendiente")}{" "}
          <button type="button" className="secondary" onClick={descartarCarga}>
            {t("version.empezarDeNuevo")}
          </button>
        </p>
      )}

      {uploading && (
        <div
          role="progressbar"
          aria-label={t("version.subiendoAria", { titulo: r.Title })}
          aria-valuenow={uploadPercent}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuetext={uploadStatusText}
          style={{ width: "100%", background: "#e5e7eb", borderRadius: "999px", height: "6px", overflow: "hidden" }}
        >
          <div
            style={{
              width: `${uploadPercent}%`,
              background: "#3b82f6",
              height: "100%",
              transition: "width 0.3s ease-in-out",
            }}
          />
        </div>
      )}

      {error && <span className="error-banner">{error}</span>}
    </li>
  );
}

/**
 * Edición del contenido de un recurso de texto, con su historial.
 *
 * Va plegado por defecto: una unidad con varios recursos abriría media docena
 * de editores a la vez, y ninguno es el que se quiere tocar.
 *
 * Guardar escribe el contenido y deja una revisión en el servidor. El
 * autoguardado del editor sigue conservando el borrador en el navegador, que
 * es lo que salva un cierre accidental de la pestaña; lo que añade el guardado
 * es el historial, que sobrevive al equipo y dice quién escribió qué.
 */
function EditorDeContenido({
  versionId,
  resource,
  onChange,
}: {
  versionId: string;
  resource: import("@/lib/api").Resource;
  onChange: () => void;
}) {
  const { t } = useI18n();
  const [abierto, setAbierto] = useState(false);
  const [markdown, setMarkdown] = useState(resource.TextContentMD ?? "");
  const [estado, setEstado] = useState("");
  const [recarga, setRecarga] = useState(0);

  async function guardar() {
    setEstado(t("comun.guardando"));
    try {
      await api.saveRevision(versionId, resource.ID, markdown);
      setEstado(t("version.guardado"));
      setRecarga((n) => n + 1);
      onChange();
    } catch (e) {
      setEstado(e instanceof ApiError ? e.message : t("version.errorGuardar"));
    }
  }

  if (!abierto) {
    return (
      <button className="secondary" onClick={() => setAbierto(true)}>
        {t("version.editarContenido")}
      </button>
    );
  }

  return (
    <div className="stack">
      <BlockEditor
        key={recarga}
        initialMarkdown={markdown}
        draftKey={`draft_resource_${resource.ID}`}
        onChange={setMarkdown}
      />
      <div className="row" style={{ alignItems: "center", gap: "0.5rem" }}>
        <button onClick={guardar}>{t("version.guardarRevision")}</button>
        <button className="secondary" onClick={() => setAbierto(false)}>
          {t("comun.cerrar")}
        </button>
        {estado && (
          <span className="muted" role="status">
            {estado}
          </span>
        )}
      </div>

      <HistorialDeRevisiones
        key={`hist-${recarga}`}
        versionId={versionId}
        resourceId={resource.ID}
        onRestaurado={async () => {
          const rev = await api
            .listRevisions(versionId, resource.ID)
            .then((r) => r.items[0])
            .catch(() => null);
          if (rev) {
            const completa = await api.getRevision(versionId, resource.ID, rev.revision_number);
            setMarkdown(completa.content_md ?? "");
            setRecarga((n) => n + 1);
          }
          onChange();
        }}
      />
    </div>
  );
}

/**
 * Lo que este borrador cambia respecto a la versión publicada.
 *
 * Se enseña junto al botón de publicar y no en una pantalla aparte porque el
 * momento en que importa es justo antes de publicar: es cuando el profesor
 * puede todavía decidir que no quiere alterar lo que sus estudiantes tienen
 * que completar.
 */
function CambiosDelBorrador({ versionId }: { versionId: string }) {
  const { t } = useI18n();
  const [clasificacion, setClasificacion] = useState<Clasificacion | null>(null);

  useEffect(() => {
    api
      .versionChanges(versionId)
      .then(setClasificacion)
      .catch(() => setClasificacion(null));
  }, [versionId]);

  if (!clasificacion) return null;

  if (clasificacion.alcance === "ninguno") {
    return <p className="muted">{t("cambios.identico")}</p>;
  }

  const mayor = clasificacion.alcance === "mayor";

  return (
    <div className="stack">
      <h3 style={{ margin: 0 }}>{t("cambios.titulo")}</h3>

      {mayor ? (
        <p className="warning-banner" role="status">
          {t("cambios.mayor")}
        </p>
      ) : (
        <p className="muted">{t("cambios.menor")}</p>
      )}

      <ul className="lista-filas">
        {clasificacion.cambios.map((c) => (
          <li key={`${c.elemento}-${c.stable_id}`} className="fila">
            <div className="fila__datos">
              <p>
                <span className="badge">{c.tipo}</span> <span className="badge">{c.elemento}</span>{" "}
                <strong>{c.titulo}</strong>
                {c.afecta_progreso && <span className="badge">{t("cambios.afectaProgreso")}</span>}
              </p>
              {c.detalle && <p className="muted">{c.detalle}</p>}
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}

/**
 * Enumera los dominios que se pueden incrustar.
 *
 * Se enseña junto al campo para que el profesor no descubra la restricción a
 * base de chocar contra un 422: la lista la fija la administración y él no
 * puede ampliarla.
 */
function DestinosAutorizados() {
  const { t } = useI18n();
  const [hosts, setHosts] = useState<string[] | null>(null);

  useEffect(() => {
    api
      .listIframeAllowlist()
      .then((res) =>
        setHosts((res.items ?? []).map((d) => (d.include_subdomains ? `*.${d.host}` : d.host))),
      )
      .catch(() => setHosts([]));
  }, []);

  if (hosts === null) return null;
  if (hosts.length === 0) {
    return (
      <p className="muted">{t("version.sinDestinos")}</p>
    );
  }
  return <p className="muted">{t("version.destinos", { hosts: hosts.join(", ") })}</p>;
}

function AddResourceForm({
  versionId,
  unitId,
  nextPosition,
  onAdded,
}: {
  versionId: string;
  unitId: string;
  nextPosition: number;
  onAdded: () => void;
}) {
  const { t } = useI18n();
  const [type, setType] = useState<string>("text");
  const [title, setTitle] = useState("");
  const [textContent, setTextContent] = useState("");
  const [externalUrl, setExternalUrl] = useState("");
  const [required, setRequired] = useState(true);
  const [visible, setVisible] = useState(true);
  const [error, setError] = useState("");

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    try {
      await api.addResource(versionId, unitId, {
        type,
        title,
        position: nextPosition,
        visible,
        required,
        downloadable: false,
        text_content_md: type === "text" ? textContent : undefined,
        external_url: type === "link" || type === "iframe" ? externalUrl : undefined,
      });
      setTitle("");
      setTextContent("");
      setExternalUrl("");
      onAdded();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : t("version.errorRecurso"));
    }
  }

  return (
    <form onSubmit={handleSubmit} className="stack" style={{ borderTop: "1px dashed var(--color-border)", paddingTop: "0.75rem" }}>
      {error && <p className="error-banner">{error}</p>}
      <div className="row">
        <div className="form-field" style={{ marginBottom: 0 }}>
          <label htmlFor={`type-${unitId}`}>{t("version.tipoRecurso")}</label>
          <select id={`type-${unitId}`} value={type} onChange={(e) => setType(e.target.value)}>
            {RESOURCE_TYPES.map(([value, clave]) => (
              <option key={value} value={value}>
                {t(clave)}
              </option>
            ))}
          </select>
        </div>
        <div className="form-field" style={{ flex: 1, marginBottom: 0 }}>
          <label htmlFor={`title-${unitId}`}>{t("version.tituloRecurso")}</label>
          <input id={`title-${unitId}`} required value={title} onChange={(e) => setTitle(e.target.value)} />
        </div>
      </div>

      {type === "text" && (
        <div className="form-field">
          <label>{t("version.contenidoBloques")}</label>
          <BlockEditor
            initialMarkdown={textContent}
            draftKey={`draft_unit_${unitId}`}
            onChange={(canonicalMd) => setTextContent(canonicalMd)}
          />
        </div>
      )}
      {(type === "link" || type === "iframe") && (
        <div className="form-field">
          <label htmlFor={`url-${unitId}`}>{t("version.url")}</label>
          <input id={`url-${unitId}`} type="url" value={externalUrl} onChange={(e) => setExternalUrl(e.target.value)} />
          {type === "iframe" && <DestinosAutorizados />}
        </div>
      )}

      <div className="row">
        <label className="row">
          <input type="checkbox" checked={visible} onChange={(e) => setVisible(e.target.checked)} />{" "}
          {t("version.visible")}
        </label>
        <label className="row">
          <input type="checkbox" checked={required} onChange={(e) => setRequired(e.target.checked)} />{" "}
          {t("version.obligatorio")}
        </label>
      </div>

      <button type="submit">{t("version.agregarRecurso")}</button>
    </form>
  );
}

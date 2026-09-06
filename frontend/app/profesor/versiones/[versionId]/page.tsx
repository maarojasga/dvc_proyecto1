"use client";

import { useEffect, useState, useCallback } from "react";
import { useParams } from "next/navigation";
import { api, type Version, type Module, type Unit, ApiError } from "@/lib/api";

const RESOURCE_TYPES = [
  ["text", "Texto enriquecido"],
  ["image", "Imagen"],
  ["video", "Video"],
  ["audio", "Audio"],
  ["pdf", "PDF"],
  ["presentation", "Presentación"],
  ["file", "Archivo descargable"],
  ["iframe", "Iframe autorizado"],
  ["link", "Enlace externo"],
  ["quiz", "Quiz"],
] as const;

const BINARY_TYPES = new Set(["image", "video", "audio", "pdf", "presentation", "file"]);

export default function VersionEditorPage() {
  const { versionId } = useParams<{ versionId: string }>();
  const [version, setVersion] = useState<Version | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [publishErrors, setPublishErrors] = useState<string[]>([]);

  const load = useCallback(async () => {
    try {
      const v = await api.getVersion(versionId);
      setVersion(v);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "No se pudo cargar la versión");
    }
  }, [versionId]);

  useEffect(() => {
    load();
  }, [load]);

  async function handlePublish() {
    setPublishErrors([]);
    setNotice("");
    try {
      await api.publishVersion(versionId);
      setNotice("¡Versión publicada!");
      load();
    } catch (e) {
      if (e instanceof ApiError && e.details?.length) {
        setPublishErrors(e.details);
      } else {
        setError(e instanceof ApiError ? e.message : "No se pudo publicar");
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
    return <p>Cargando…</p>;
  }

  const isDraft = version.Status === "draft";

  return (
    <div>
      <h1>{version.Title || "(sin título)"}</h1>
      <p className="badge">Versión {version.VersionNumber} · {version.Status}</p>

      {notice && <p className="success-banner">{notice}</p>}
      {publishErrors.length > 0 && (
        <div className="error-banner" role="alert">
          <strong>No se pudo publicar. Motivos:</strong>
          <ul>
            {publishErrors.map((d, i) => (
              <li key={i}>{d}</li>
            ))}
          </ul>
        </div>
      )}

      {!isDraft && (
        <p className="error-banner">
          Esta versión no es un borrador ({version.Status}); no se puede editar.
        </p>
      )}

      <MetadataForm version={version} disabled={!isDraft} onSaved={load} />

      {isDraft && (
        <div className="card">
          <button onClick={handlePublish}>Publicar versión</button>
        </div>
      )}

      <h2>Estructura</h2>
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
      setError(e instanceof ApiError ? e.message : "No se pudo guardar");
    } finally {
      setSaving(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="card stack">
      <h2 style={{ marginTop: 0 }}>Metadatos</h2>
      {error && <p className="error-banner">{error}</p>}
      <div className="form-field">
        <label htmlFor="title">Título</label>
        <input id="title" disabled={disabled} value={form.Title} onChange={(e) => setForm({ ...form, Title: e.target.value })} />
      </div>
      <div className="form-field">
        <label htmlFor="summary">Resumen</label>
        <input
          id="summary"
          disabled={disabled}
          value={form.Summary}
          onChange={(e) => setForm({ ...form, Summary: e.target.value })}
        />
      </div>
      <div className="form-field">
        <label htmlFor="description">Descripción (Markdown)</label>
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
          <label htmlFor="category">Categoría</label>
          <input id="category" disabled={disabled} value={form.Category} onChange={(e) => setForm({ ...form, Category: e.target.value })} />
        </div>
        <div className="form-field">
          <label htmlFor="level">Nivel</label>
          <input id="level" disabled={disabled} value={form.Level} onChange={(e) => setForm({ ...form, Level: e.target.value })} />
        </div>
        <div className="form-field">
          <label htmlFor="language">Idioma</label>
          <input id="language" disabled={disabled} value={form.Language} onChange={(e) => setForm({ ...form, Language: e.target.value })} />
        </div>
      </div>
      <div className="row">
        <div className="form-field">
          <label htmlFor="minscore">Nota mínima de aprobación (%)</label>
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
          <label htmlFor="reqpct">% de recursos obligatorios requeridos</label>
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
        {saving ? "Guardando…" : "Guardar metadatos"}
      </button>
    </form>
  );
}

function AddModuleForm({ versionId, nextPosition, onAdded }: { versionId: string; nextPosition: number; onAdded: () => void }) {
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
      setError(e instanceof ApiError ? e.message : "No se pudo agregar el módulo");
    }
  }

  return (
    <form onSubmit={handleSubmit} className="row card">
      {error && <p className="error-banner">{error}</p>}
      <div className="form-field" style={{ flex: 1, marginBottom: 0 }}>
        <label htmlFor="new-module">Nuevo módulo</label>
        <input id="new-module" required value={title} onChange={(e) => setTitle(e.target.value)} />
      </div>
      <button type="submit">Agregar módulo</button>
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
  async function handleDelete() {
    if (!confirm(`¿Eliminar el módulo "${m.Title}" y todo su contenido?`)) return;
    await api.deleteModule(versionId, m.ID);
    onChange();
  }

  return (
    <li className="card">
      <div className="row">
        <h3 style={{ flex: 1 }}>{m.Title}</h3>
        {editable && (
          <button className="danger" onClick={handleDelete}>
            Eliminar módulo
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
        <label htmlFor={`new-unit-${moduleId}`}>Nueva unidad</label>
        <input id={`new-unit-${moduleId}`} required value={title} onChange={(e) => setTitle(e.target.value)} />
      </div>
      <button type="submit">Agregar unidad</button>
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
  async function handleDelete() {
    if (!confirm(`¿Eliminar la unidad "${u.Title}" y sus recursos?`)) return;
    await api.deleteUnit(versionId, u.ID);
    onChange();
  }

  return (
    <li className="card" style={{ background: "transparent" }}>
      <div className="row">
        <strong style={{ flex: 1 }}>{u.Title}</strong>
        {editable && (
          <button className="danger" onClick={handleDelete}>
            Eliminar unidad
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
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState("");

  async function handleDelete() {
    if (!confirm(`¿Eliminar el recurso "${r.Title}"?`)) return;
    await api.deleteResource(versionId, r.ID);
    onChange();
  }

  async function handleFile(file: File) {
    setUploading(true);
    setError("");
    try {
      const { upload_url } = await api.requestUploadUrl(versionId, r.ID, file.type || "application/octet-stream");
      const putRes = await fetch(upload_url, { method: "PUT", body: file });
      if (!putRes.ok) throw new Error("La subida al almacenamiento falló");
      await api.confirmUpload(versionId, r.ID);
      onChange();
    } catch (e) {
      setError(e instanceof Error ? e.message : "No se pudo subir el archivo");
    } finally {
      setUploading(false);
    }
  }

  return (
    <li className="row">
      <span style={{ flex: 1 }}>
        {r.Title} <span className="badge">{r.Type}</span>{" "}
        {r.ProcessingStatus !== "none" && <span className="badge">procesamiento: {r.ProcessingStatus}</span>}
        {!r.Visible && <span className="badge">oculto</span>}
      </span>
      {editable && BINARY_TYPES.has(r.Type) && (
        <label className="row" style={{ marginBottom: 0 }}>
          <span className="badge" style={{ cursor: "pointer" }}>
            {uploading ? "Subiendo…" : "Subir archivo"}
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
        <button className="danger" onClick={handleDelete}>
          Eliminar
        </button>
      )}
      {error && <span className="error-banner">{error}</span>}
    </li>
  );
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
        Type: type,
        Title: title,
        Position: nextPosition,
        Visible: visible,
        Required: required,
        Downloadable: false,
        TextContentMD: type === "text" ? textContent : undefined,
        ExternalURL: type === "link" || type === "iframe" ? externalUrl : undefined,
      });
      setTitle("");
      setTextContent("");
      setExternalUrl("");
      onAdded();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "No se pudo agregar el recurso");
    }
  }

  return (
    <form onSubmit={handleSubmit} className="stack" style={{ borderTop: "1px dashed var(--color-border)", paddingTop: "0.75rem" }}>
      {error && <p className="error-banner">{error}</p>}
      <div className="row">
        <div className="form-field" style={{ marginBottom: 0 }}>
          <label htmlFor={`type-${unitId}`}>Tipo de recurso</label>
          <select id={`type-${unitId}`} value={type} onChange={(e) => setType(e.target.value)}>
            {RESOURCE_TYPES.map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
        </div>
        <div className="form-field" style={{ flex: 1, marginBottom: 0 }}>
          <label htmlFor={`title-${unitId}`}>Título del recurso</label>
          <input id={`title-${unitId}`} required value={title} onChange={(e) => setTitle(e.target.value)} />
        </div>
      </div>

      {type === "text" && (
        <div className="form-field">
          <label htmlFor={`content-${unitId}`}>Contenido (Markdown)</label>
          <textarea id={`content-${unitId}`} rows={3} value={textContent} onChange={(e) => setTextContent(e.target.value)} />
        </div>
      )}
      {(type === "link" || type === "iframe") && (
        <div className="form-field">
          <label htmlFor={`url-${unitId}`}>URL</label>
          <input id={`url-${unitId}`} type="url" value={externalUrl} onChange={(e) => setExternalUrl(e.target.value)} />
        </div>
      )}

      <div className="row">
        <label className="row">
          <input type="checkbox" checked={visible} onChange={(e) => setVisible(e.target.checked)} /> Visible
        </label>
        <label className="row">
          <input type="checkbox" checked={required} onChange={(e) => setRequired(e.target.checked)} /> Obligatorio
        </label>
      </div>

      <button type="submit">Agregar recurso</button>
    </form>
  );
}

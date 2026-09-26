"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { api, type Course, ApiError } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

export default function TeacherDashboardPage() {
  const [courses, setCourses] = useState<Course[] | null>(null);
  const [slug, setSlug] = useState("");
  const [title, setTitle] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const { t } = useI18n();
  const router = useRouter();

  // En useCallback y declarada antes del efecto: leerla desde un efecto que
  // no la lista como dependencia dejaba una versión vieja capturada, y era el
  // único aviso de lint que arrastraba el proyecto.
  const load = useCallback(async () => {
    try {
      const res = await api.listMyCourses();
      setCourses(res.items ?? []);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : t("profesor.error"));
    }
  }, [t]);

  useEffect(() => {
    void load();
  }, [load]);

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      const res = await api.createCourse({ slug, title });
      // router.push y no window.location.href: navegar por el router conserva
      // el estado del cliente y evita recargar la aplicación entera para ir a
      // una pantalla que ya está cargada.
      router.push(`/profesor/versiones/${res.version_id}`);
    } catch (e) {
      setError(e instanceof ApiError ? [e.message, ...(e.details ?? [])].join(" ") : t("profesor.errorCrear"));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div>
      <header className="page-header">
        <div>
          <h1>{t("profesor.titulo")}</h1>
          <p>{t("profesor.subtitulo")}</p>
        </div>
      </header>

      <section className="card">
        <header>
          <h2>{t("profesor.nuevoCurso")}</h2>
          <p>{t("profesor.nuevoCursoAyuda")}</p>
        </header>
        {error && (
          <p className="error-banner" role="alert">
            {error}
          </p>
        )}
        <form onSubmit={handleCreate} className="stack">
          <div className="form-field">
            <label htmlFor="title">{t("comun.titulo")}</label>
            <input id="title" required value={title} onChange={(e) => setTitle(e.target.value)} />
          </div>
          <div className="form-field">
            <label htmlFor="slug">{t("profesor.slug")}</label>
            <input
              id="slug"
              required
              pattern="[a-z0-9-]+"
              placeholder={t("profesor.slugPlaceholder")}
              value={slug}
              onChange={(e) => setSlug(e.target.value)}
            />
          </div>
          <button type="submit" disabled={submitting}>
            {submitting ? t("profesor.creando") : t("profesor.crearBorrador")}
          </button>
        </form>
      </section>

      <h2>{t("nav.misCursos")}</h2>
      {courses === null && <p role="status">{t("comun.cargando")}</p>}
      {courses?.length === 0 && <p>{t("profesor.sinCursos")}</p>}
      <ul className="stack" style={{ listStyle: "none", padding: 0 }}>
        {courses?.map((c) => (
          <CourseRow key={c.ID} course={c} onError={setError} onChange={load} />
        ))}
      </ul>
    </div>
  );
}

function CourseRow({
  course: c,
  onError,
  onChange,
}: {
  course: Course;
  onError: (msg: string) => void;
  onChange: () => Promise<void>;
}) {
  const router = useRouter();
  const { t } = useI18n();
  const [trabajando, setTrabajando] = useState(false);

  const publicado = Boolean(c.CurrentPublishedVersionID);

  // Editar siempre ocurre sobre un borrador: si ya hay uno abierto se entra
  // directo, y si no, se abre copiando la última versión. A un curso publicado
  // no se le ofrece, porque el servidor lo rechaza: primero se despublica.
  async function handleEditar() {
    if (c.LatestDraftVersionID) {
      router.push(`/profesor/versiones/${c.LatestDraftVersionID}`);
      return;
    }
    setTrabajando(true);
    try {
      const v = await api.createUpdateDraft(c.ID);
      router.push(`/profesor/versiones/${v.ID}`);
    } catch (e) {
      onError(e instanceof ApiError ? e.message : t("profesor.errorBorrador"));
    } finally {
      setTrabajando(false);
    }
  }

  // Se confirma porque despublicar se nota fuera: el curso desaparece del
  // catálogo y nadie más puede inscribirse hasta que vuelva a publicarse.
  async function handleDespublicar() {
    if (!confirm(t("profesor.confirmarDespublicar", { slug: c.Slug }))) return;
    setTrabajando(true);
    try {
      await api.unpublishCourse(c.ID);
      await onChange();
    } catch (e) {
      onError(e instanceof ApiError ? e.message : t("profesor.errorDespublicar"));
    } finally {
      setTrabajando(false);
    }
  }

  return (
    <li className="card">
      <strong>{c.Slug}</strong>
      <p className="badge">{publicado ? t("profesor.publicado") : t("profesor.sinPublicar")}</p>
      {publicado && <p className="muted">{t("profesor.publicadoAyuda")}</p>}
      <div style={{ display: "flex", gap: "10px", alignItems: "center" }}>
        {publicado ? (
          <>
            <button type="button" onClick={handleDespublicar} disabled={trabajando}>
              {trabajando ? t("profesor.despublicando") : t("profesor.despublicarParaEditar")}
            </button>
            <Link href={`/profesor/versiones/${c.CurrentPublishedVersionID}`}>{t("profesor.verPublicada")}</Link>
          </>
        ) : (
          <button type="button" onClick={handleEditar} disabled={trabajando}>
            {trabajando
              ? t("profesor.creando")
              : c.LatestDraftVersionID
                ? t("profesor.editarBorrador")
                : t("profesor.crearVersion")}
          </button>
        )}
      </div>
    </li>
  );
}

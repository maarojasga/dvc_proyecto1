"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { api, type Course, ApiError } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

export default function TeacherDashboardPage() {
  const [courses, setCourses] = useState<Course[] | null>(null);
  const [slug, setSlug] = useState("");
  const [title, setTitle] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const { t } = useI18n();

  useEffect(() => {
    load();
  }, []);

  async function load() {
    try {
      const res = await api.listMyCourses();
      setCourses(res.items ?? []);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : t("profesor.error"));
    }
  }

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      const res = await api.createCourse({ slug, title });
      window.location.href = `/profesor/versiones/${res.version_id}`;
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
          <li key={c.ID} className="card">
            <strong>{c.Slug}</strong>
            <p className="badge">{c.CurrentPublishedVersionID ? t("profesor.publicado") : t("profesor.sinPublicar")}</p>
            {c.CurrentPublishedVersionID && (
              <p>
                <Link href={`/profesor/versiones/${c.CurrentPublishedVersionID}`}>{t("profesor.verPublicada")}</Link>
              </p>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}

"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { api, type Course, ApiError } from "@/lib/api";

export default function TeacherDashboardPage() {
  const [courses, setCourses] = useState<Course[] | null>(null);
  const [slug, setSlug] = useState("");
  const [title, setTitle] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    load();
  }, []);

  async function load() {
    try {
      const res = await api.listMyCourses();
      setCourses(res.items ?? []);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "No se pudieron cargar tus cursos");
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
      setError(e instanceof ApiError ? [e.message, ...(e.details ?? [])].join(" ") : "No se pudo crear el curso");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div>
      <header className="page-header">
        <div>
          <h1>Autoría de cursos</h1>
          <p>Crea borradores y publica versiones. Una versión publicada es inmutable.</p>
        </div>
      </header>

      <section className="card">
        <header>
          <h2>Nuevo curso</h2>
          <p>El curso nace como borrador editable.</p>
        </header>
        {error && (
          <p className="error-banner" role="alert">
            {error}
          </p>
        )}
        <form onSubmit={handleCreate} className="stack">
          <div className="form-field">
            <label htmlFor="title">Título</label>
            <input id="title" required value={title} onChange={(e) => setTitle(e.target.value)} />
          </div>
          <div className="form-field">
            <label htmlFor="slug">Slug (identificador en la URL)</label>
            <input
              id="slug"
              required
              pattern="[a-z0-9-]+"
              placeholder="introduccion-a-cloud"
              value={slug}
              onChange={(e) => setSlug(e.target.value)}
            />
          </div>
          <button type="submit" disabled={submitting}>
            {submitting ? "Creando…" : "Crear borrador"}
          </button>
        </form>
      </section>

      <h2>Mis cursos</h2>
      {courses === null && <p role="status">Cargando…</p>}
      {courses?.length === 0 && <p>Aún no has creado ningún curso.</p>}
      <ul className="stack" style={{ listStyle: "none", padding: 0 }}>
        {courses?.map((c) => (
          <li key={c.ID} className="card">
            <strong>{c.Slug}</strong>
            <p className="badge">{c.CurrentPublishedVersionID ? "Publicado" : "Sin publicar"}</p>
            {c.CurrentPublishedVersionID && (
              <p>
                <Link href={`/profesor/versiones/${c.CurrentPublishedVersionID}`}>Ver versión publicada</Link>
              </p>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}

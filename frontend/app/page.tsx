"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { api, type Version, ApiError } from "@/lib/api";

export default function HomePage() {
  const [courses, setCourses] = useState<Version[] | null>(null);
  const [error, setError] = useState("");
  const [q, setQ] = useState("");

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function load(search = "") {
    setError("");
    try {
      const res = await api.listCatalog(search ? { q: search } : {});
      setCourses(res.items ?? []);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "No se pudo cargar el catálogo");
    }
  }

  return (
    <div>
      <header className="page-header">
        <div>
          <h1>Catálogo de cursos</h1>
          <p>Cursos publicados y abiertos a inscripción.</p>
        </div>
      </header>

      <form
        role="search"
        className="barra-busqueda"
        onSubmit={(e) => {
          e.preventDefault();
          load(q);
        }}
      >
        <div className="form-field">
          <label htmlFor="q">Buscar cursos</label>
          <input id="q" value={q} onChange={(e) => setQ(e.target.value)} placeholder="Título o resumen" />
        </div>
        <button type="submit">Buscar</button>
      </form>

      {error && (
        <p className="error-banner" role="alert">
          {error}
        </p>
      )}

      {courses === null && !error && <p role="status">Cargando cursos…</p>}

      {courses?.length === 0 && (
        <div className="estado-vacio">
          <p>No hay cursos publicados que coincidan con la búsqueda.</p>
          <p>Prueba con otro término o vuelve más tarde.</p>
        </div>
      )}

      <ul className="rejilla">
        {courses?.map((v) => (
          <li key={v.ID}>
            <article className="card">
              <h2>
                <Link href={`/cursos/${v.CourseID}`}>{v.Title}</Link>
              </h2>
              <p style={{ flex: 1 }}>{v.Summary}</p>
              <p className="row">
                {v.Category && <span className="badge">{v.Category}</span>}
                {v.Level && <span className="badge">{v.Level}</span>}
                <span className="badge">{v.Language}</span>
              </p>
            </article>
          </li>
        ))}
      </ul>
    </div>
  );
}

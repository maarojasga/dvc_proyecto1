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
      <h1>Catálogo de cursos</h1>
      <form
        role="search"
        className="row"
        onSubmit={(e) => {
          e.preventDefault();
          load(q);
        }}
      >
        <div className="form-field" style={{ marginBottom: 0, flex: 1 }}>
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

      {courses === null && !error && <p>Cargando cursos…</p>}
      {courses?.length === 0 && <p>No hay cursos publicados que coincidan con la búsqueda.</p>}

      <ul className="stack" style={{ listStyle: "none", padding: 0 }}>
        {courses?.map((v) => (
          <li key={v.ID} className="card">
            <h2 style={{ marginTop: 0 }}>
              <Link href={`/cursos/${v.CourseID}`}>{v.Title}</Link>
            </h2>
            <p>{v.Summary}</p>
            <p className="row">
              {v.Category && <span className="badge">{v.Category}</span>}
              {v.Level && <span className="badge">{v.Level}</span>}
              <span className="badge">{v.Language}</span>
            </p>
          </li>
        ))}
      </ul>
    </div>
  );
}

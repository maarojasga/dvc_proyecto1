"use client";

import { useState, useEffect, useRef, useCallback } from "react";

import { aBloques, aMarkdown, bloqueVacio, nuevoID, type Block, type TipoDeBloque } from "@/lib/markdown";

// El editor delega la conversión a lib/markdown, que es lógica pura y tiene
// pruebas de ida y vuelta por tipo de nodo. Aquí solo queda la interfaz.
export type { Block } from "@/lib/markdown";

interface BlockEditorProps {
  initialMarkdown?: string;
  /** Clave bajo la que se guarda el borrador local. */
  draftKey?: string;
  onChange: (canonicalMarkdown: string) => void;
}

// conBloqueInicial evita presentar un editor sin nada donde escribir: un
// documento vacío se convierte en cero bloques, que es correcto al
// serializar pero inservible al editar.
function conBloqueInicial(bloques: Block[]): Block[] {
  return bloques.length > 0 ? bloques : [bloqueVacio()];
}

// contenidoInicial da a los bloques estructurados algo con lo que arrancar.
// Una tabla vacía no se puede editar: no tiene celdas donde escribir.
function contenidoInicial(tipo: TipoDeBloque): string {
  switch (tipo) {
    case "table":
      return "Columna 1|Columna 2\n|";
    case "task":
      return "Primera tarea";
    default:
      return "";
  }
}

export function BlockEditor({ initialMarkdown = "", draftKey = "mooc_block_draft", onChange }: BlockEditorProps) {
  const [blocks, setBlocks] = useState<Block[]>(() => {
    if (typeof window !== "undefined") {
      const saved = localStorage.getItem(draftKey);
      if (saved) {
        try {
          return JSON.parse(saved);
        } catch {}
      }
    }
    return conBloqueInicial(aBloques(initialMarkdown));
  });

  const [autosaveStatus, setAutosaveStatus] = useState<string>("Borrador guardado");
  const saveTimeoutRef = useRef<NodeJS.Timeout | null>(null);

  const triggerAutosave = useCallback(
    (newBlocks: Block[]) => {
      setAutosaveStatus("Guardando...");
      if (saveTimeoutRef.current) clearTimeout(saveTimeoutRef.current);

      saveTimeoutRef.current = setTimeout(() => {
        try {
          if (typeof window !== "undefined") {
            localStorage.setItem(draftKey, JSON.stringify(newBlocks));
          }
          const md = aMarkdown(newBlocks);
          onChange(md);
          const timeStr = new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
          setAutosaveStatus(`Autoguardado a las ${timeStr}`);
        } catch {
          setAutosaveStatus("Error al autoguardar");
        }
      }, 800);
    },
    [draftKey, onChange],
  );

  useEffect(() => {
    return () => {
      if (saveTimeoutRef.current) clearTimeout(saveTimeoutRef.current);
    };
  }, []);

  const updateBlock = (id: string, updates: Partial<Block>) => {
    const updated = blocks.map((b) => (b.id === id ? { ...b, ...updates } : b));
    setBlocks(updated);
    triggerAutosave(updated);
  };

  const addBlock = (type: TipoDeBloque, afterId?: string) => {
    const newBlock: Block = {
      id: nuevoID(),
      type,
      level: type === "heading" ? 2 : undefined,
      content: contenidoInicial(type),
      language: type === "code" ? "javascript" : undefined,
      done: type === "task" ? [false] : undefined,
    };

    let updated: Block[];
    if (afterId) {
      const idx = blocks.findIndex((b) => b.id === afterId);
      updated = [...blocks.slice(0, idx + 1), newBlock, ...blocks.slice(idx + 1)];
    } else {
      updated = [...blocks, newBlock];
    }
    setBlocks(updated);
    triggerAutosave(updated);
  };

  const removeBlock = (id: string) => {
    if (blocks.length <= 1) {
      updateBlock(id, { content: "" });
      return;
    }
    const updated = blocks.filter((b) => b.id !== id);
    setBlocks(updated);
    triggerAutosave(updated);
  };

  const moveBlock = (index: number, direction: "up" | "down") => {
    const targetIdx = direction === "up" ? index - 1 : index + 1;
    if (targetIdx < 0 || targetIdx >= blocks.length) return;
    const updated = [...blocks];
    const temp = updated[index];
    updated[index] = updated[targetIdx];
    updated[targetIdx] = temp;
    setBlocks(updated);
    triggerAutosave(updated);
  };

  const clearDraft = () => {
    if (typeof window !== "undefined") {
      localStorage.removeItem(draftKey);
    }
    const reset = conBloqueInicial(aBloques(initialMarkdown));
    setBlocks(reset);
    triggerAutosave(reset);
  };

  return (
    <div className="stack" style={{ background: "var(--color-bg-subtle, #f8f9fa)", padding: "1rem", borderRadius: "8px", border: "1px solid var(--color-border, #e5e7eb)" }}>
      <div className="row" style={{ justifyContent: "space-between", alignItems: "center", marginBottom: "0.5rem" }}>
        <strong style={{ fontSize: "0.9rem" }}>Editor de Bloques (Markdown Canónico)</strong>
        <span className="badge" style={{ fontSize: "0.75rem", opacity: 0.8 }}>
          {autosaveStatus}
        </span>
      </div>

      <div className="stack" style={{ gap: "0.75rem" }}>
        {blocks.map((block, index) => (
          <div
            key={block.id}
            style={{
              background: "white",
              padding: "0.75rem",
              borderRadius: "6px",
              boxShadow: "0 1px 3px rgba(0,0,0,0.05)",
              border: "1px solid var(--color-border, #e5e7eb)",
            }}
          >
            <div className="row" style={{ justifyContent: "space-between", marginBottom: "0.4rem" }}>
              <div className="row" style={{ gap: "0.3rem", alignItems: "center" }}>
                <span className="badge" style={{ textTransform: "capitalize", fontSize: "0.75rem" }}>
                  {block.type} {block.type === "heading" ? `H${block.level}` : ""}
                </span>
                {block.type === "heading" && (
                  <select
                    value={block.level || 2}
                    onChange={(e) => updateBlock(block.id, { level: Number(e.target.value) as 1 | 2 | 3 })}
                    style={{ fontSize: "0.75rem", padding: "2px 4px" }}
                  >
                    <option value={1}>H1</option>
                    <option value={2}>H2</option>
                    <option value={3}>H3</option>
                  </select>
                )}
                {block.type === "code" && (
                  <input
                    type="text"
                    placeholder="lenguaje (ej: python, go)"
                    value={block.language || ""}
                    onChange={(e) => updateBlock(block.id, { language: e.target.value })}
                    style={{ fontSize: "0.75rem", padding: "2px 6px", width: "120px" }}
                  />
                )}
              </div>

              <div className="row" style={{ gap: "0.2rem" }}>
                <button
                  type="button"
                  onClick={() => moveBlock(index, "up")}
                  disabled={index === 0}
                  style={{ padding: "2px 6px", fontSize: "0.75rem" }}
                  title="Mover arriba"
                >
                  ↑
                </button>
                <button
                  type="button"
                  onClick={() => moveBlock(index, "down")}
                  disabled={index === blocks.length - 1}
                  style={{ padding: "2px 6px", fontSize: "0.75rem" }}
                  title="Mover abajo"
                >
                  ↓
                </button>
                <button
                  type="button"
                  onClick={() => removeBlock(block.id)}
                  style={{ padding: "2px 6px", fontSize: "0.75rem", color: "#dc2626" }}
                  title="Eliminar bloque"
                >
                  ✕
                </button>
              </div>
            </div>

            {block.type === "heading" ? (
              <input
                type="text"
                value={block.content}
                placeholder="Texto del encabezado..."
                onChange={(e) => updateBlock(block.id, { content: e.target.value })}
                style={{
                  width: "100%",
                  fontWeight: "bold",
                  fontSize: block.level === 1 ? "1.25rem" : block.level === 2 ? "1.1rem" : "1rem",
                }}
              />
            ) : block.type === "code" ? (
              <textarea
                rows={4}
                value={block.content}
                placeholder="// Código aquí..."
                onChange={(e) => updateBlock(block.id, { content: e.target.value })}
                style={{ width: "100%", fontFamily: "monospace", fontSize: "0.85rem", background: "#1e1e1e", color: "#f8f8f2", borderRadius: "4px" }}
              />
            ) : block.type === "callout" ? (
              <textarea
                rows={2}
                value={block.content}
                placeholder="Nota importante o advertencia..."
                onChange={(e) => updateBlock(block.id, { content: e.target.value })}
                style={{ width: "100%", borderLeft: "4px solid #3b82f6", background: "#eff6ff" }}
              />
            ) : block.type === "formula" ? (
              <>
                <textarea
                  rows={2}
                  value={block.content}
                  placeholder="\\int_0^1 x^2 dx = \\frac{1}{3}"
                  onChange={(e) => updateBlock(block.id, { content: e.target.value })}
                  style={{ width: "100%", fontFamily: "monospace", fontSize: "0.9rem" }}
                />
                <p className="muted" style={{ fontSize: "0.75rem", margin: "0.25rem 0 0" }}>
                  Notación LaTeX. Se guarda entre <code>$$</code>, que es lo que reconocen los
                  renderizadores de fórmulas.
                </p>
              </>
            ) : block.type === "table" ? (
              <EditorDeTabla
                contenido={block.content}
                onChange={(contenido) => updateBlock(block.id, { content: contenido })}
              />
            ) : block.type === "task" ? (
              <EditorDeTareas
                contenido={block.content}
                hechas={block.done ?? []}
                onChange={(contenido, hechas) => updateBlock(block.id, { content: contenido, done: hechas })}
              />
            ) : (
              <textarea
                rows={3}
                value={block.content}
                placeholder={block.type === "list" ? "Un elemento por línea..." : "Escribe el contenido del párrafo..."}
                onChange={(e) => updateBlock(block.id, { content: e.target.value })}
                style={{ width: "100%" }}
              />
            )}
          </div>
        ))}
      </div>

      <div className="row" style={{ gap: "0.5rem", marginTop: "0.5rem", flexWrap: "wrap" }}>
        <button type="button" onClick={() => addBlock("paragraph")}>
          + Párrafo
        </button>
        <button type="button" onClick={() => addBlock("heading")}>
          + Encabezado
        </button>
        <button type="button" onClick={() => addBlock("code")}>
          + Código
        </button>
        <button type="button" onClick={() => addBlock("callout")}>
          + Nota / Cita
        </button>
        <button type="button" onClick={() => addBlock("list")}>
          + Lista
        </button>
        <button type="button" onClick={() => addBlock("task")}>
          + Tareas
        </button>
        <button type="button" onClick={() => addBlock("table")}>
          + Tabla
        </button>
        <button type="button" onClick={() => addBlock("formula")}>
          + Fórmula
        </button>
        <button type="button" onClick={clearDraft} style={{ marginLeft: "auto", fontSize: "0.75rem", opacity: 0.7 }}>
          Limpiar borrador
        </button>
      </div>
    </div>
  );
}

/**
 * Edición de una tabla celda a celda.
 *
 * El contenido se guarda como filas separadas por saltos de línea y celdas por
 * "|", que es lo que espera lib/markdown. Se edita en una cuadrícula y no en un
 * textarea con pipes porque alinear una tabla a mano es justo lo que un editor
 * de bloques debería ahorrar.
 */
function EditorDeTabla({
  contenido,
  onChange,
}: {
  contenido: string;
  onChange: (contenido: string) => void;
}) {
  const filas = contenido.split("\n").map((f) => f.split("|"));
  const columnas = Math.max(1, ...filas.map((f) => f.length));

  const emitir = (nuevas: string[][]) => onChange(nuevas.map((f) => f.join("|")).join("\n"));

  const cambiarCelda = (fila: number, columna: number, valor: string) => {
    const nuevas = filas.map((f) => [...f]);
    while (nuevas[fila].length < columnas) nuevas[fila].push("");
    // El "|" es el separador: dejarlo entrar en una celda partiría la fila.
    nuevas[fila][columna] = valor.replace(/\|/g, "/");
    emitir(nuevas);
  };

  return (
    <div className="stack" style={{ gap: "0.4rem" }}>
      <div style={{ overflowX: "auto" }}>
        <table style={{ borderCollapse: "collapse", width: "100%" }}>
          <tbody>
            {filas.map((fila, i) => (
              <tr key={i}>
                {Array.from({ length: columnas }).map((_, j) => (
                  <td key={j} style={{ border: "1px solid var(--color-border, #e5e7eb)", padding: 0 }}>
                    <label className="visually-hidden" htmlFor={`celda-${i}-${j}`}>
                      {i === 0 ? `Encabezado de la columna ${j + 1}` : `Fila ${i}, columna ${j + 1}`}
                    </label>
                    <input
                      id={`celda-${i}-${j}`}
                      value={fila[j] ?? ""}
                      onChange={(e) => cambiarCelda(i, j, e.target.value)}
                      style={{
                        width: "100%",
                        border: 0,
                        padding: "0.35rem",
                        fontWeight: i === 0 ? 600 : 400,
                        background: i === 0 ? "var(--color-bg-subtle, #f1f5f9)" : "transparent",
                      }}
                    />
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className="row" style={{ gap: "0.3rem" }}>
        <button
          type="button"
          className="secondary"
          onClick={() => emitir([...filas, Array(columnas).fill("")])}
        >
          + Fila
        </button>
        <button
          type="button"
          className="secondary"
          onClick={() => emitir(filas.map((f) => [...f, ""]))}
        >
          + Columna
        </button>
        {filas.length > 1 && (
          <button type="button" className="secondary" onClick={() => emitir(filas.slice(0, -1))}>
            − Fila
          </button>
        )}
      </div>
      <p className="muted" style={{ fontSize: "0.75rem", margin: 0 }}>
        La primera fila es el encabezado.
      </p>
    </div>
  );
}

/**
 * Lista de tareas con su marcado.
 *
 * El estado de cada casilla viaja en paralelo al texto, en el mismo orden,
 * porque así se serializa a "- [x] texto" sin tener que interpretar el texto
 * para saber si estaba marcada.
 */
function EditorDeTareas({
  contenido,
  hechas,
  onChange,
}: {
  contenido: string;
  hechas: boolean[];
  onChange: (contenido: string, hechas: boolean[]) => void;
}) {
  const lineas = contenido.split("\n");

  const cambiarTexto = (i: number, valor: string) => {
    const nuevas = [...lineas];
    nuevas[i] = valor;
    onChange(nuevas.join("\n"), hechas);
  };

  const alternar = (i: number) => {
    const nuevas = [...hechas];
    while (nuevas.length < lineas.length) nuevas.push(false);
    nuevas[i] = !nuevas[i];
    onChange(contenido, nuevas);
  };

  return (
    <div className="stack" style={{ gap: "0.3rem" }}>
      {lineas.map((linea, i) => (
        <div key={i} className="row" style={{ alignItems: "center", gap: "0.4rem" }}>
          <input
            type="checkbox"
            checked={hechas[i] ?? false}
            onChange={() => alternar(i)}
            aria-label={`Marcar "${linea || `tarea ${i + 1}`}" como hecha`}
          />
          <input
            value={linea}
            onChange={(e) => cambiarTexto(i, e.target.value)}
            placeholder="Descripción de la tarea"
            style={{ flex: 1 }}
          />
          {lineas.length > 1 && (
            <button
              type="button"
              className="secondary"
              onClick={() =>
                onChange(
                  lineas.filter((_, j) => j !== i).join("\n"),
                  hechas.filter((_, j) => j !== i),
                )
              }
              aria-label={`Quitar la tarea ${i + 1}`}
            >
              ✕
            </button>
          )}
        </div>
      ))}
      <button
        type="button"
        className="secondary"
        onClick={() => onChange([...lineas, ""].join("\n"), [...hechas, false])}
      >
        + Tarea
      </button>
    </div>
  );
}

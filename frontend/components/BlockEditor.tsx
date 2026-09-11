"use client";

import { useState, useEffect, useRef, useCallback } from "react";

export interface Block {
  id: string;
  type: "heading" | "paragraph" | "code" | "callout" | "list";
  level?: 1 | 2 | 3;
  content: string;
  language?: string;
}

interface BlockEditorProps {
  initialMarkdown?: string;
  draftKey?: string;
  onChange: (canonicalMarkdown: string) => void;
  /**
   * onAutosave persiste el contenido en el servidor. Cuando se pasa, el
   * borrador local deja de ser el guardado y pasa a ser solo la red: se usa
   * para no perder cambios si el guardado falla.
   *
   * Sin él —al crear un recurso que todavía no existe en el servidor— el
   * borrador local es lo único que hay, y se dice así en la interfaz.
   */
  onAutosave?: (canonicalMarkdown: string) => Promise<void>;
}

export function markdownToBlocks(md: string): Block[] {
  if (!md || !md.trim()) {
    return [{ id: "b-1", type: "paragraph", content: "" }];
  }

  const lines = md.split("\n");
  const blocks: Block[] = [];
  let currentParagraph = "";
  let inCode = false;
  let codeContent: string[] = [];
  let codeLang = "";

  const flushParagraph = () => {
    if (currentParagraph.trim()) {
      blocks.push({
        id: `b-${Date.now()}-${Math.random().toString(36).substr(2, 4)}`,
        type: "paragraph",
        content: currentParagraph.trim(),
      });
      currentParagraph = "";
    }
  };

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];

    if (line.startsWith("```")) {
      if (inCode) {
        blocks.push({
          id: `b-${Date.now()}-${Math.random().toString(36).substr(2, 4)}`,
          type: "code",
          content: codeContent.join("\n"),
          language: codeLang,
        });
        inCode = false;
        codeContent = [];
        codeLang = "";
      } else {
        flushParagraph();
        inCode = true;
        codeLang = line.slice(3).trim();
      }
      continue;
    }

    if (inCode) {
      codeContent.push(line);
      continue;
    }

    if (line.startsWith("# ")) {
      flushParagraph();
      blocks.push({
        id: `b-${Date.now()}-${Math.random().toString(36).substr(2, 4)}`,
        type: "heading",
        level: 1,
        content: line.slice(2).trim(),
      });
    } else if (line.startsWith("## ")) {
      flushParagraph();
      blocks.push({
        id: `b-${Date.now()}-${Math.random().toString(36).substr(2, 4)}`,
        type: "heading",
        level: 2,
        content: line.slice(3).trim(),
      });
    } else if (line.startsWith("### ")) {
      flushParagraph();
      blocks.push({
        id: `b-${Date.now()}-${Math.random().toString(36).substr(2, 4)}`,
        type: "heading",
        level: 3,
        content: line.slice(4).trim(),
      });
    } else if (line.startsWith("> ")) {
      flushParagraph();
      blocks.push({
        id: `b-${Date.now()}-${Math.random().toString(36).substr(2, 4)}`,
        type: "callout",
        content: line.slice(2).trim(),
      });
    } else if (line.startsWith("- ") || line.startsWith("* ")) {
      flushParagraph();
      blocks.push({
        id: `b-${Date.now()}-${Math.random().toString(36).substr(2, 4)}`,
        type: "list",
        content: line.slice(2).trim(),
      });
    } else if (line.trim() === "") {
      flushParagraph();
    } else {
      currentParagraph += (currentParagraph ? "\n" : "") + line;
    }
  }

  flushParagraph();
  if (inCode) {
    blocks.push({
      id: `b-${Date.now()}-${Math.random().toString(36).substr(2, 4)}`,
      type: "code",
      content: codeContent.join("\n"),
      language: codeLang,
    });
  }

  return blocks.length ? blocks : [{ id: "b-1", type: "paragraph", content: "" }];
}

function serializarBloque(b: Block): string {
  switch (b.type) {
    case "heading": {
      const prefix = "#".repeat(b.level || 2);
      return `${prefix} ${b.content.trim()}`;
    }
    case "paragraph":
      return b.content.trim();
    case "code":
      return `\`\`\`${b.language || ""}\n${b.content}\n\`\`\``;
    case "callout":
      return `> ${b.content.trim()}`;
    case "list":
      return `- ${b.content.trim()}`;
    default:
      return b.content;
  }
}

export function blocksToMarkdown(blocks: Block[]): string {
  const lineas = blocks.map(serializarBloque);
  let salida = "";

  for (let i = 0; i < blocks.length; i++) {
    if (!lineas[i].length) continue;
    if (salida.length) {
      // Los elementos de lista consecutivos van pegados. Separarlos con línea
      // en blanco produce una lista "suelta", que Markdown renderiza con un
      // párrafo dentro de cada punto: es otro documento, y el editor no tiene
      // forma de expresar esa diferencia. Todo lo demás va separado por línea
      // en blanco, que es lo que separa bloques en Markdown.
      const pegar = blocks[i].type === "list" && bloqueAnteriorVisible(blocks, lineas, i)?.type === "list";
      salida += pegar ? "\n" : "\n\n";
    }
    salida += lineas[i];
  }
  return salida;
}

/** bloqueAnteriorVisible salta los bloques que no producen texto. */
function bloqueAnteriorVisible(blocks: Block[], lineas: string[], i: number): Block | undefined {
  for (let j = i - 1; j >= 0; j--) {
    if (lineas[j].length) return blocks[j];
  }
  return undefined;
}

/** RETARDO_AUTOGUARDADO es la pausa al teclear antes de guardar. */
const RETARDO_AUTOGUARDADO = 800;

type EstadoGuardado =
  | { tipo: "limpio" }
  | { tipo: "guardando" }
  | { tipo: "guardado"; cuando: Date; enServidor: boolean }
  | { tipo: "fallido"; motivo: string };

function leerBorradorLocal(clave: string): Block[] | null {
  if (typeof window === "undefined") return null;
  try {
    const guardado = localStorage.getItem(clave);
    if (!guardado) return null;
    const bloques = JSON.parse(guardado);
    return Array.isArray(bloques) && bloques.length ? bloques : null;
  } catch {
    return null;
  }
}

export function BlockEditor({
  initialMarkdown = "",
  draftKey = "mooc_block_draft",
  onChange,
  onAutosave,
}: BlockEditorProps) {
  // El contenido del servidor es el punto de partida. Cargar el borrador local
  // por encima —como se hacía— pierde ediciones sin avisar: basta editar en un
  // equipo y abrir en otro donde quedó un borrador viejo para que el
  // autoguardado lo escriba encima. El borrador se ofrece, no se impone.
  const [blocks, setBlocks] = useState<Block[]>(() => markdownToBlocks(initialMarkdown));
  const [borradorLocal, setBorradorLocal] = useState<Block[] | null>(null);
  const [estado, setEstado] = useState<EstadoGuardado>({ tipo: "limpio" });
  const saveTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    const local = leerBorradorLocal(draftKey);
    if (!local) return;
    // Solo se ofrece si dice algo distinto de lo que ya hay.
    if (blocksToMarkdown(local) !== blocksToMarkdown(markdownToBlocks(initialMarkdown))) {
      setBorradorLocal(local);
    } else {
      try {
        localStorage.removeItem(draftKey);
      } catch {}
    }
    // Se mira una vez, al montar: después el estado del editor es la verdad.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const triggerAutosave = useCallback(
    (newBlocks: Block[]) => {
      setEstado({ tipo: "guardando" });
      if (saveTimeoutRef.current) clearTimeout(saveTimeoutRef.current);

      saveTimeoutRef.current = setTimeout(async () => {
        const md = blocksToMarkdown(newBlocks);
        // El borrador local se escribe antes de intentar el servidor: si el
        // guardado falla, es lo que permite no perder lo escrito.
        try {
          localStorage.setItem(draftKey, JSON.stringify(newBlocks));
        } catch {
          // Sin almacenamiento local se pierde la red de seguridad, no el
          // guardado en servidor.
        }
        onChange(md);

        if (!onAutosave) {
          setEstado({ tipo: "guardado", cuando: new Date(), enServidor: false });
          return;
        }
        try {
          await onAutosave(md);
          setEstado({ tipo: "guardado", cuando: new Date(), enServidor: true });
          // Guardado en el servidor, el borrador local ya no hace falta.
          try {
            localStorage.removeItem(draftKey);
          } catch {}
        } catch (e) {
          setEstado({
            tipo: "fallido",
            motivo: e instanceof Error ? e.message : "no se pudo guardar en el servidor",
          });
        }
      }, RETARDO_AUTOGUARDADO);
    },
    [draftKey, onChange, onAutosave],
  );

  useEffect(() => {
    return () => {
      if (saveTimeoutRef.current) clearTimeout(saveTimeoutRef.current);
    };
  }, []);

  function recuperarBorrador() {
    if (!borradorLocal) return;
    setBlocks(borradorLocal);
    setBorradorLocal(null);
    triggerAutosave(borradorLocal);
  }

  function descartarBorrador() {
    setBorradorLocal(null);
    try {
      localStorage.removeItem(draftKey);
    } catch {}
  }

  const updateBlock = (id: string, updates: Partial<Block>) => {
    const updated = blocks.map((b) => (b.id === id ? { ...b, ...updates } : b));
    setBlocks(updated);
    triggerAutosave(updated);
  };

  const addBlock = (type: Block["type"], afterId?: string) => {
    const newBlock: Block = {
      id: `b-${Date.now()}-${Math.random().toString(36).substr(2, 4)}`,
      type,
      level: type === "heading" ? 2 : undefined,
      content: "",
      language: type === "code" ? "javascript" : undefined,
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
    const reset = markdownToBlocks(initialMarkdown);
    setBlocks(reset);
    triggerAutosave(reset);
  };

  return (
    <div className="stack" style={{ background: "var(--color-bg-subtle, #f8f9fa)", padding: "1rem", borderRadius: "8px", border: "1px solid var(--color-border, #e5e7eb)" }}>
      <div className="row" style={{ justifyContent: "space-between", alignItems: "center", marginBottom: "0.5rem" }}>
        <strong style={{ fontSize: "0.9rem" }}>Editor de bloques (Markdown canónico)</strong>
        {/* role="status" para que un lector de pantalla anuncie el guardado
            sin robar el foco al profesor mientras escribe. */}
        <span className="badge" role="status" aria-live="polite" style={{ fontSize: "0.75rem", opacity: 0.8 }}>
          {textoDeEstado(estado, Boolean(onAutosave))}
        </span>
      </div>

      {estado.tipo === "fallido" && (
        <p className="error-banner" role="alert">
          No se pudo guardar en el servidor: {estado.motivo}. Lo escrito queda en este
          navegador y se ofrecerá recuperarlo al volver a abrir el recurso.
        </p>
      )}

      {borradorLocal && (
        <div className="warning-banner" role="alert">
          <p>
            Hay un borrador sin guardar de una sesión anterior en este navegador. Puede
            ser más viejo que lo que se ve arriba.
          </p>
          <div className="row">
            <button type="button" onClick={recuperarBorrador}>
              Recuperar el borrador
            </button>
            <button type="button" className="danger" onClick={descartarBorrador}>
              Descartar
            </button>
          </div>
        </div>
      )}

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
            ) : (
              <textarea
                rows={3}
                value={block.content}
                placeholder={block.type === "list" ? "Elemento de lista..." : "Escribe el contenido del párrafo..."}
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
        <button type="button" onClick={clearDraft} style={{ marginLeft: "auto", fontSize: "0.75rem", opacity: 0.7 }}>
          Limpiar borrador
        </button>
      </div>
    </div>
  );
}

/**
 * textoDeEstado dice donde quedo lo escrito, no solo que se guardo. La
 * diferencia importa: "guardado en este navegador" y "guardado en el
 * servidor" son garantias distintas, y un profesor que cierra el portatil
 * merece saber cual tiene.
 */
function textoDeEstado(estado: EstadoGuardado, haciaServidor: boolean): string {
  switch (estado.tipo) {
    case "limpio":
      return haciaServidor ? "Sin cambios" : "Borrador local";
    case "guardando":
      return "Guardando…";
    case "guardado": {
      const hora = estado.cuando.toLocaleTimeString([], {
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
      });
      return estado.enServidor
        ? `Guardado en el servidor a las ${hora}`
        : `Borrador local guardado a las ${hora}`;
    }
    case "fallido":
      return "Sin guardar en el servidor";
  }
}

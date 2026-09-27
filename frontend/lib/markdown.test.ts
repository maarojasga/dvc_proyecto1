import { describe, expect, it } from "vitest";

import { aBloques, aMarkdown, type Block, type TipoDeBloque } from "./markdown";

// Pruebas de ida y vuelta por tipo de nodo, que es lo que recomienda el
// enunciado (sección 11) y lo que impide que un tipo de bloque nuevo pierda
// contenido en silencio.
//
// La propiedad que se comprueba no es que el Markdown de entrada vuelva letra
// por letra —eso obligaría a conservar las grafías alternativas que el formato
// admite—, sino que la forma canónica sea un punto fijo: convertir a bloques y
// de vuelta dos veces da lo mismo que una. Esa es la garantía que de verdad
// hace falta, porque un documento se abre y se guarda muchas veces.

/** ida y vuelta: md -> bloques -> md */
function vuelta(md: string): string {
  return aMarkdown(aBloques(md));
}

/** tipos devuelve los tipos de bloque en orden, para afirmar sobre el AST. */
function tipos(md: string): TipoDeBloque[] {
  return aBloques(md).map((b) => b.type);
}

const casos: Record<string, string> = {
  encabezado: "# Título\n\n## Subtítulo\n\n### Apartado",
  parrafo: "Un párrafo con **negritas** y `código`.",
  lista: "- Primero\n- Segundo\n- Tercero",
  aviso: "> Ojo con esto.\n> Va en dos líneas.",
  codigo: "```go\nfunc main() {\n\tfmt.Println(1)\n}\n```",
  tabla: "| Curso | Nivel |\n| --- | --- |\n| Cloud | Avanzado |\n| Redes | Básico |",
  formula: "$$\n\\int_0^1 x^2 dx = \\frac{1}{3}\n$$",
  tareas: "- [x] Leer el enunciado\n- [ ] Escribir las pruebas\n- [ ] Publicar",
};

describe("biyección AST ↔ Markdown por tipo de nodo", () => {
  for (const [nombre, md] of Object.entries(casos)) {
    it(`${nombre}: la forma canónica es un punto fijo`, () => {
      const unaVez = vuelta(md);
      expect(vuelta(unaVez)).toBe(unaVez);
    });

    it(`${nombre}: la ida y vuelta conserva el contenido`, () => {
      expect(vuelta(md)).toBe(md);
    });
  }
});

describe("reconocimiento de cada tipo", () => {
  it("distingue una tarea de una lista normal", () => {
    // "- [ ] x" también empieza por "- ", así que el orden de comprobación
    // importa: sin él, las tareas se leerían como lista y perderían la marca.
    expect(tipos("- [ ] Pendiente")).toEqual(["task"]);
    expect(tipos("- Un elemento")).toEqual(["list"]);
  });

  it("conserva qué tareas estaban marcadas", () => {
    const bloques = aBloques("- [x] Hecha\n- [ ] Pendiente");
    expect(bloques[0].done).toEqual([true, false]);
  });

  it("agrupa las líneas contiguas de una lista en un solo bloque", () => {
    const bloques = aBloques("- Uno\n- Dos\n- Tres");
    expect(bloques).toHaveLength(1);
    expect(bloques[0].content.split("\n")).toEqual(["Uno", "Dos", "Tres"]);
  });

  it("no interpreta el interior de un bloque de código", () => {
    const md = "```\n# Esto no es un encabezado\n- ni una lista\n| ni | una tabla |\n```";
    expect(tipos(md)).toEqual(["code"]);
    expect(vuelta(md)).toBe(md);
  });

  it("conserva el lenguaje del bloque de código", () => {
    expect(aBloques("```python\nprint(1)\n```")[0].language).toBe("python");
  });

  it("separa la tabla de sus celdas", () => {
    const bloques = aBloques("| A | B |\n| --- | --- |\n| 1 | 2 |");
    expect(bloques[0].type).toBe("table");
    expect(bloques[0].content).toBe("A|B\n1|2");
  });
});

describe("normalización", () => {
  it("regenera el separador de la tabla, se escriba como se escriba", () => {
    const flojo = "| A | B |\n|:---|----------:|\n| 1 | 2 |";
    expect(vuelta(flojo)).toBe("| A | B |\n| --- | --- |\n| 1 | 2 |");
  });

  it("completa las filas a las que les faltan celdas", () => {
    // Una fila corta rompería la tabla al renderizarla.
    const desigual = "| A | B | C |\n| --- | --- | --- |\n| 1 |";
    expect(vuelta(desigual)).toBe("| A | B | C |\n| --- | --- | --- |\n| 1 |  |  |");
  });

  it("acepta asteriscos como viñeta y los canoniza a guiones", () => {
    expect(vuelta("* Uno\n* Dos")).toBe("- Uno\n- Dos");
  });

  it("no persiste los bloques vacíos que deja el editor al escribir", () => {
    const bloques: Block[] = [
      { id: "1", type: "paragraph", content: "Contenido" },
      { id: "2", type: "paragraph", content: "   " },
      { id: "3", type: "heading", level: 2, content: "" },
    ];
    expect(aMarkdown(bloques)).toBe("Contenido");
  });

  it("un documento vacío no inventa bloques", () => {
    expect(aBloques("")).toEqual([]);
    expect(aBloques("   \n  \n")).toEqual([]);
  });
});

describe("contenido que el editor no conoce", () => {
  it("lo conserva como párrafo en vez de perderlo", () => {
    // Abrir en el editor algo escrito a mano no puede mutilarlo: lo que no se
    // reconoce cae en un párrafo y vuelve a salir tal cual.
    const md = "Una línea con <abbr>HTML</abbr> y una imagen ![alt](/x.png)";
    expect(tipos(md)).toEqual(["paragraph"]);
    expect(vuelta(md)).toBe(md);
  });
});

describe("documento completo", () => {
  it("mantiene el orden y el tipo de cada bloque", () => {
    const md = [
      "# Unidad 1",
      "",
      "Texto introductorio.",
      "",
      "## Objetivos",
      "",
      "- Entender HLS",
      "- Medir el progreso",
      "",
      "- [x] Leer",
      "- [ ] Practicar",
      "",
      "| Concepto | Valor |",
      "| --- | --- |",
      "| p95 | 300 ms |",
      "",
      "$$\nE = mc^2\n$$",
      "",
      "```sql",
      "SELECT 1;",
      "```",
      "",
      "> Recuerda guardar.",
    ].join("\n");

    expect(tipos(md)).toEqual([
      "heading",
      "paragraph",
      "heading",
      "list",
      "task",
      "table",
      "formula",
      "code",
      "callout",
    ]);
    expect(vuelta(md)).toBe(md);
  });
});

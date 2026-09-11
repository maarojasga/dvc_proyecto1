import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { cursoConTextoYQuiz, entrar, estudianteRegistrado, profesorInvitado, unico } from "./apoyo";

/**
 * Auditoría automática de accesibilidad.
 *
 * La condición de aceptación es que no haya incumplimientos críticos, así que
 * la prueba falla con lo serio (critical y serious) y solo informa de lo menor.
 * Poner el umbral en "cualquier aviso" haría que nadie mirase el informe, que
 * es peor que no tenerlo.
 *
 * Una auditoría automática no sustituye una revisión manual: detecta contraste,
 * etiquetas, roles y orden de encabezados, y no detecta si el texto de un
 * enlace tiene sentido. Lo que acredita es que no hay incumplimientos
 * automáticamente detectables.
 */

const REGLAS = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"];

/** auditar revuelve los incumplimientos serios de la página actual. */
async function auditar(page: import("@playwright/test").Page) {
  const { violations } = await new AxeBuilder({ page }).withTags(REGLAS).analyze();
  const graves = violations.filter((v) => v.impact === "critical" || v.impact === "serious");
  const menores = violations.filter((v) => v.impact !== "critical" && v.impact !== "serious");
  if (menores.length) {
    console.log(
      `avisos menores (${menores.length}): ${menores.map((v) => `${v.id} (${v.impact})`).join(", ")}`,
    );
  }
  return graves;
}

function describir(graves: Awaited<ReturnType<typeof auditar>>): string {
  return graves
    .map((v) => `[${v.impact}] ${v.id}: ${v.help}\n  ${v.nodes.map((n) => n.target.join(" ")).join("\n  ")}`)
    .join("\n");
}

test.describe("páginas públicas", () => {
  for (const [nombre, ruta] of [
    ["catálogo", "/"],
    ["inicio de sesión", "/login"],
    ["registro", "/registro"],
  ] as const) {
    test(`${nombre} sin incumplimientos graves`, async ({ page }) => {
      await page.goto(ruta);
      await page.waitForLoadState("networkidle");
      const graves = await auditar(page);
      expect(graves.length, describir(graves)).toBe(0);
    });
  }
});

test("las pantallas del estudiante no tienen incumplimientos graves", async ({ page, request }) => {
  const { prof } = await profesorInvitado(`${unico("prof")}@e2e.local`);
  const sembrado = await cursoConTextoYQuiz(prof, unico("a11y"), "Curso accesible");

  const correo = `${unico("est")}@e2e.local`;
  const est = await estudianteRegistrado(correo);
  await est.json("post", "/api/v1/enrollments", { course_id: sembrado.cursoId });

  await entrar(page, correo);

  for (const [nombre, ruta] of [
    ["mis cursos", "/mis-cursos"],
    ["mis insignias", "/mis-insignias"],
    ["detalle del curso", `/cursos/${sembrado.cursoId}`],
    ["recurso de texto", `/cursos/${sembrado.cursoId}/recursos/${sembrado.recursoTextoId}`],
    ["evaluación", `/cursos/${sembrado.cursoId}/recursos/${sembrado.recursoQuizId}`],
  ] as const) {
    await page.goto(ruta);
    await page.waitForLoadState("networkidle");
    const graves = await auditar(page);
    expect(graves.length, `${nombre}:\n${describir(graves)}`).toBe(0);
  }
});

test("las pantallas de autoría no tienen incumplimientos graves", async ({ page, request }) => {
  const correo = `${unico("prof")}@e2e.local`;
  const { prof } = await profesorInvitado(correo);
  // Un borrador, que es donde el editor está activo: en una versión publicada
  // los controles están desactivados y no se auditaría lo que importa.
  const borrador = await prof.json<any>("post", "/api/v1/courses", {
    slug: unico("borrador"),
    title: "Curso en borrador",
  });
  const modulo = await prof.json<any>("post", `/api/v1/courses/versions/${borrador.version_id}/modules`, {
    title: "Módulo",
    position: 1,
  });
  const unidad = await prof.json<any>(
    "post",
    `/api/v1/courses/versions/${borrador.version_id}/modules/${modulo.ID}/units`,
    { title: "Unidad", position: 1 },
  );
  await prof.json("post", `/api/v1/courses/versions/${borrador.version_id}/units/${unidad.ID}/resources`, {
    type: "text",
    title: "Lectura",
    position: 1,
    visible: true,
    required: true,
    text_content_md: "# Borrador",
  });
  await prof.json("post", `/api/v1/courses/versions/${borrador.version_id}/units/${unidad.ID}/resources`, {
    type: "quiz",
    title: "Evaluación",
    position: 2,
    visible: true,
    required: true,
  });

  await entrar(page, correo);

  await page.goto("/profesor");
  await page.waitForLoadState("networkidle");
  let graves = await auditar(page);
  expect(graves.length, `panel del profesor:\n${describir(graves)}`).toBe(0);

  await page.goto(`/profesor/versiones/${borrador.version_id}`);
  await page.waitForLoadState("networkidle");
  graves = await auditar(page);
  expect(graves.length, `editor de versión:\n${describir(graves)}`).toBe(0);

  // Con los editores abiertos, que es cuando hay más controles.
  await page.getByRole("button", { name: /Editar contenido/ }).first().click();
  await page.getByRole("button", { name: /Definir evaluación/ }).first().click();
  await page.waitForTimeout(500);
  graves = await auditar(page);
  expect(graves.length, `editores abiertos:\n${describir(graves)}`).toBe(0);
});

test("la verificación pública de una insignia es accesible", async ({ page }) => {
  // Un código inexistente: la página de error también tiene que ser accesible,
  // y es el estado que más se olvida auditar.
  await page.goto("/insignias/codigo-que-no-existe");
  await page.waitForLoadState("networkidle");
  const graves = await auditar(page);
  expect(graves.length, describir(graves)).toBe(0);
});

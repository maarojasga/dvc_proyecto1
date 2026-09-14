import { test, expect } from "@playwright/test";
import { API, conToken, credencialesDeAdmin, sufijo, tokenDeSesion } from "./ayudas";

/**
 * Segmento 2 — Autoría y publicación.
 *
 * Criterio: autoría y publicación. El enunciado pide creación del borrador,
 * módulos, unidades y recursos; previsualización; **lista exhaustiva de
 * errores**; y publicación de una versión inmutable.
 *
 * Lo de "exhaustiva" no es un adorno: un formulario que devuelve el primer
 * error obliga a publicar, leer, corregir y repetir. Se comprueba que vengan
 * todos de una vez.
 */
test.describe("2. Autoría y publicación", () => {
  test("un borrador incompleto no se publica y devuelve todos los motivos a la vez", async ({
    request,
  }) => {
    const admin = credencialesDeAdmin();
    const token = await tokenDeSesion(request, admin.email, admin.password);
    const marca = sufijo();

    const creacion = await request.post(`${API}/api/v1/courses`, {
      headers: conToken(token),
      data: { slug: `incompleto-${marca}`, title: "Borrador incompleto" },
    });
    expect(creacion.ok()).toBeTruthy();
    const versionId = (await creacion.json()).version_id;

    // Se publica sin estructura ni metadatos: debe fallar por varias razones.
    const publicacion = await request.post(
      `${API}/api/v1/courses/versions/${versionId}/publish`,
      { headers: conToken(token) },
    );
    expect(publicacion.status(), "un borrador vacío no es publicable").toBe(422);

    const cuerpo = await publicacion.json();
    const motivos: string[] = cuerpo.error?.details ?? cuerpo.details ?? [];
    expect(motivos.length, "la respuesta enumera todos los motivos, no el primero").toBeGreaterThan(1);
  });

  test("el flujo completo de autoría publica una versión que queda inmutable", async ({
    page,
    request,
  }) => {
    const admin = credencialesDeAdmin();
    const token = await tokenDeSesion(request, admin.email, admin.password);
    const marca = sufijo();

    const creacion = await request.post(`${API}/api/v1/courses`, {
      headers: conToken(token),
      data: { slug: `curso-e2e-${marca}`, title: `Curso E2E ${marca}` },
    });
    const { version_id: versionId } = await creacion.json();

    await request.patch(`${API}/api/v1/courses/versions/${versionId}`, {
      headers: conToken(token),
      data: {
        title: `Curso E2E ${marca}`,
        summary: "Curso creado por la prueba E2E del segmento 2.",
        description_md: "Contenido de prueba.",
        category: "Pruebas",
        level: "Introductorio",
        language: "es",
        approval_min_score: 60,
        approval_required_resources_pct: 100,
      },
    });

    const modulo = await request.post(`${API}/api/v1/courses/versions/${versionId}/modules`, {
      headers: conToken(token),
      data: { title: "Módulo único", position: 1 },
    });
    const moduloId = (await modulo.json()).ID ?? (await modulo.json()).id;

    const unidad = await request.post(
      `${API}/api/v1/courses/versions/${versionId}/modules/${moduloId}/units`,
      { headers: conToken(token), data: { title: "Unidad única", position: 1 } },
    );
    const unidadId = (await unidad.json()).ID ?? (await unidad.json()).id;

    await request.post(
      `${API}/api/v1/courses/versions/${versionId}/units/${unidadId}/resources`,
      {
        headers: conToken(token),
        data: {
          type: "text",
          title: "Lección de prueba",
          position: 1,
          visible: true,
          required: true,
          downloadable: false,
          text_content_md: "## Lección\n\nContenido.",
        },
      },
    );

    // La previsualización es parte del segmento: el autor comprueba antes de
    // publicar qué se va a encontrar quien se inscriba.
    await page.goto("/login");
    await page.getByLabel(/correo electrónico|email address/i).fill(admin.email);
    await page.getByLabel(/contraseña|password/i).fill(admin.password);
    await page.getByRole("button", { name: /ingresar|sign in/i }).click();
    await expect(page.getByRole("button", { name: /salir|sign out/i })).toBeVisible();

    await page.goto(`/profesor/versiones/${versionId}/previsualizacion`);
    await expect(page.getByRole("heading", { name: /previsualización|preview/i })).toBeVisible();
    await expect(page.getByText("Lección de prueba")).toBeVisible();

    const publicacion = await request.post(
      `${API}/api/v1/courses/versions/${versionId}/publish`,
      { headers: conToken(token) },
    );
    expect(publicacion.ok(), "ya completo, el borrador se publica").toBeTruthy();

    // Inmutable: la versión publicada rechaza cambios de metadatos.
    const intentoDeEdicion = await request.patch(`${API}/api/v1/courses/versions/${versionId}`, {
      headers: conToken(token),
      data: { title: "Título cambiado a traición" },
    });
    expect(
      intentoDeEdicion.ok(),
      "una versión publicada no se puede editar sin despublicar el curso",
    ).toBeFalsy();

    // Y aparece en el catálogo público, que es el efecto de publicar.
    await page.goto("/");
    await page.getByLabel(/buscar cursos|search courses/i).fill(`Curso E2E ${marca}`);
    await page.getByRole("button", { name: /^buscar$|^search$/i }).click();
    await expect(page.getByRole("link", { name: `Curso E2E ${marca}` })).toBeVisible();
  });
});

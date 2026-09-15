import { test, expect } from "@playwright/test";
import {
  API,
  conToken,
    entrarPorLaInterfaz,
  registrarEstudiante,
  sufijo,
  tokenDeAdmin,
  tokenDeSesion,
  CLAVE_VALIDA,
} from "./ayudas";

/**
 * Segmento 1 — Identidad y administración.
 *
 * Criterio: identidad, autorización y seguridad. El enunciado pide verificar
 * registro de estudiante, invitación de profesor, revocación inmediata de
 * sesiones, suspensión auditada y rechazo de operaciones no autorizadas.
 */
test.describe("1. Identidad y administración", () => {
  test("el registro público exige verificar el correo antes de dejar entrar", async ({
    page,
    request,
  }) => {
    const marca = sufijo();
    const email = `sinverificar-${marca}@e2e.local`;

    const alta = await request.post(`${API}/api/v1/auth/register`, {
      data: { email, password: CLAVE_VALIDA, full_name: "Sin verificar" },
    });
    expect(alta.status()).toBe(202);

    // La cuenta existe pero está pendiente: entrar tiene que fallar. Es la
    // diferencia entre "te mandamos un correo" y "te creímos".
    await page.goto("/login");
    await page.getByLabel(/correo electrónico|email address/i).fill(email);
    await page.getByLabel(/contraseña|password/i).fill(CLAVE_VALIDA);
    await page.getByRole("button", { name: /ingresar|sign in/i }).click();
    await expect(page.getByRole("alert")).toBeVisible();
    await expect(page.getByRole("button", { name: /salir|sign out/i })).toHaveCount(0);
  });

  test("una cuenta verificada entra y ve sus pantallas de estudiante", async ({
    page,
    request,
  }) => {
    const cuenta = await registrarEstudiante(request, sufijo());
    await entrarPorLaInterfaz(page, cuenta);

    await expect(page.getByRole("link", { name: /mis cursos|my courses/i })).toBeVisible();
    await expect(page.getByRole("link", { name: /mis insignias|my badges/i })).toBeVisible();
    // Un estudiante no ve autoría ni administración: el menú no ofrece lo que
    // la API va a negar después.
    await expect(page.getByRole("link", { name: /^autoría$|^authoring$/i })).toHaveCount(0);
    await expect(page.getByRole("link", { name: /administración|administration/i })).toHaveCount(0);
  });

  test("el estudiante no puede operar endpoints de autoría", async ({ request }) => {
    const cuenta = await registrarEstudiante(request, sufijo());
    const token = await tokenDeSesion(request, cuenta.email, cuenta.password);

    const intento = await request.post(`${API}/api/v1/courses`, {
      headers: conToken(token),
      data: { slug: `robado-${sufijo()}`, title: "Curso ajeno" },
    });
    expect(intento.status(), "crear cursos es de profesores").toBe(403);

    const listado = await request.get(`${API}/api/v1/admin/users`, { headers: conToken(token) });
    expect(listado.status(), "la administración es de administradores").toBe(403);
  });

  test("revocar las demás sesiones invalida el otro dispositivo de inmediato", async ({
    request,
  }) => {
    const cuenta = await registrarEstudiante(request, sufijo());

    // Dos sesiones: el "otro dispositivo" y el actual.
    const otroDispositivo = await tokenDeSesion(request, cuenta.email, cuenta.password);
    const esteDispositivo = await tokenDeSesion(request, cuenta.email, cuenta.password);

    const antes = await request.get(`${API}/api/v1/auth/me`, {
      headers: conToken(otroDispositivo),
    });
    expect(antes.ok(), "la primera sesión funciona antes de revocar").toBeTruthy();

    const revocacion = await request.delete(`${API}/api/v1/auth/sessions`, {
      headers: conToken(esteDispositivo),
    });
    expect(revocacion.ok()).toBeTruthy();

    // Sin esperar a que expire nada: la siguiente petición ya no pasa.
    const despues = await request.get(`${API}/api/v1/auth/me`, {
      headers: conToken(otroDispositivo),
    });
    expect(despues.status(), "la sesión revocada muere en la siguiente petición").toBe(401);

    const propia = await request.get(`${API}/api/v1/auth/me`, {
      headers: conToken(esteDispositivo),
    });
    expect(propia.ok(), "revocar las demás no cierra la propia").toBeTruthy();
  });

  test("la administración invita profesores, suspende cuentas y lo deja en la bitácora", async ({
    request,
  }) => {
    const tokenAdmin = await tokenDeAdmin(request);
    const marca = sufijo();

    // Los profesores no se autorregistran: los da de alta la administración.
    const correoProfesor = `profesor-${marca}@e2e.local`;
    const invitacion = await request.post(`${API}/api/v1/admin/teachers`, {
      headers: conToken(tokenAdmin),
      data: { email: correoProfesor, full_name: `Profesor ${marca}` },
    });
    expect(invitacion.ok(), "la invitación de profesor debe aceptarse").toBeTruthy();

    const estudiante = await registrarEstudiante(request, marca);
    const tokenEstudiante = await tokenDeSesion(request, estudiante.email, estudiante.password);

    const usuarios = await request.get(
      `${API}/api/v1/admin/users?q=${encodeURIComponent(estudiante.email)}`,
      { headers: conToken(tokenAdmin) },
    );
    const lista = (await usuarios.json()).items ?? [];
    const objetivo = lista.find((u: { email: string }) => u.email === estudiante.email);
    expect(objetivo, "el administrador encuentra la cuenta").toBeTruthy();

    const suspension = await request.patch(`${API}/api/v1/admin/users/${objetivo.id}/status`, {
      headers: conToken(tokenAdmin),
      data: { status: "suspended" },
    });
    expect(suspension.ok()).toBeTruthy();

    // Suspender no es cosmético: la sesión que ya existía deja de servir.
    const tras = await request.get(`${API}/api/v1/auth/me`, { headers: conToken(tokenEstudiante) });
    expect([401, 403]).toContain(tras.status());

    const bitacora = await request.get(`${API}/api/v1/admin/audit?action=user.status_updated`, {
      headers: conToken(tokenAdmin),
    });
    const entradas = (await bitacora.json()).items ?? [];
    expect(
      entradas.some((e: { entity_id?: string }) => e.entity_id === objetivo.id),
      "la suspensión queda registrada en la bitácora inmutable",
    ).toBeTruthy();
  });
});

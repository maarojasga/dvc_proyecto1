/** Sonda de salud del frontend, usada por el healthcheck de Docker. */
export const dynamic = "force-dynamic";

export function GET() {
  return Response.json({ estado: "ok" });
}

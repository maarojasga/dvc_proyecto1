import { PageHeader } from "@/components/ui/page-header";
import { EstadoPendiente } from "@/components/ui/estado-pendiente";

export const metadata = { title: "Recurso" };

/**
 * Visor de recurso: texto, imagen, video o audio HLS, PDF, presentacion,
 * descargable, iframe autorizado o enlace externo.
 */
export default async function PaginaRecurso({
  params,
}: {
  params: Promise<{ cursoId: string; recursoId: string }>;
}) {
  await params;

  return (
    <div className="flex-1 space-y-6">
      <PageHeader
        titulo="Visor de recurso"
        descripcion="Entrega autorizada mediante URLs firmadas; el avance lo determina el servidor."
      />
      <EstadoPendiente
        detalle="Reproduccion adaptativa HLS con reanudacion, visor PDF accesible, envio de heartbeats y navegacion por teclado."
        criterio="Multimedia y distribucion / Progreso e insignias"
      />
    </div>
  );
}

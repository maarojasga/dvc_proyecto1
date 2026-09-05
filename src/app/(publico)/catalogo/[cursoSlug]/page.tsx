import { PageHeader } from "@/components/ui/page-header";
import { EstadoPendiente } from "@/components/ui/estado-pendiente";
import { Etiqueta } from "@/components/ui/etiqueta";

export const metadata = { title: "Detalle del curso" };

export default async function PaginaDetalleCurso({
  params,
}: {
  params: Promise<{ cursoSlug: string }>;
}) {
  const { cursoSlug } = await params;

  return (
    <div className="space-y-6">
      <PageHeader
        titulo="Detalle del curso"
        descripcion="Ficha publica de la version vigente: metadatos, temario y criterios de aprobacion."
        acciones={<Etiqueta>{cursoSlug}</Etiqueta>}
      />
      <EstadoPendiente
        detalle="Lectura del curso publicado, temario por modulos y unidades, e inscripcion o reinscripcion conservando el progreso."
        criterio="Autoria y publicacion"
      />
    </div>
  );
}

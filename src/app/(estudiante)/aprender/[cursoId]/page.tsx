import { PageHeader } from "@/components/ui/page-header";
import { EstadoPendiente } from "@/components/ui/estado-pendiente";

export const metadata = { title: "Aprender" };

export default async function PaginaCursoEstudiante({
  params,
}: {
  params: Promise<{ cursoId: string }>;
}) {
  await params;

  return (
    <div className="flex-1 space-y-6">
      <PageHeader
        titulo="Contenido del curso"
        descripcion="Indice de modulos y unidades de la version en la que el estudiante esta inscrito."
      />
      <EstadoPendiente
        detalle="Arbol navegable de la version inscrita, marcado de recursos obligatorios y reanudacion desde la ultima posicion reportada."
        criterio="Multimedia y distribucion"
      />
    </div>
  );
}

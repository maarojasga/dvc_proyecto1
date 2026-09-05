import { PageHeader } from "@/components/ui/page-header";
import { EstadoPendiente } from "@/components/ui/estado-pendiente";

export const metadata = { title: "Versiones del curso" };

export default function PaginaVersionesCurso() {
  return (
    <div className="space-y-6">
      <PageHeader
        titulo="Versiones"
        descripcion="Historial de versiones publicadas, inmutables una vez emitidas."
      />
      <EstadoPendiente
        detalle="Previsualizacion, lista exhaustiva de errores de validacion, publicacion de una nueva version y despublicacion temporal para editar."
        criterio="Autoria y publicacion"
      />
    </div>
  );
}

import { PageHeader } from "@/components/ui/page-header";
import { EstadoPendiente } from "@/components/ui/estado-pendiente";

export const metadata = { title: "Mis cursos" };

export default function PaginaMisCursos() {
  return (
    <div className="space-y-6">
      <PageHeader
        titulo="Mis cursos"
        descripcion="Inscripciones activas, avance por curso e insignias obtenidas."
      />
      <EstadoPendiente
        detalle="Listado de inscripciones con porcentaje de recursos obligatorios completados, retiro y reinscripcion."
        criterio="Progreso e insignias"
      />
    </div>
  );
}

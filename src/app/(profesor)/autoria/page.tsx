import { PageHeader } from "@/components/ui/page-header";
import { EstadoPendiente } from "@/components/ui/estado-pendiente";

export const metadata = { title: "Autoria" };

export default function PaginaAutoria() {
  return (
    <div className="space-y-6">
      <PageHeader
        titulo="Autoria de cursos"
        descripcion="Cursos propios, borradores en edicion y versiones publicadas."
      />
      <EstadoPendiente
        detalle="Listado de cursos del profesor por estado, creacion de borrador y acceso a la estructura y a las versiones."
        criterio="Autoria y publicacion"
      />
    </div>
  );
}

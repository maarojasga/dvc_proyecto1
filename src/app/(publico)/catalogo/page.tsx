import { PageHeader } from "@/components/ui/page-header";
import { EstadoPendiente } from "@/components/ui/estado-pendiente";

export const metadata = { title: "Catalogo de cursos" };

export default function PaginaCatalogo() {
  return (
    <div className="space-y-6">
      <PageHeader
        titulo="Catalogo de cursos"
        descripcion="Cursos publicados con busqueda y filtros; desde aqui se realiza la inscripcion."
      />
      <EstadoPendiente
        detalle="Listado paginado por cursor, busqueda, filtros y accion de inscripcion."
        criterio="Autoria y publicacion"
      />
    </div>
  );
}

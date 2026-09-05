import { PageHeader } from "@/components/ui/page-header";
import { EstadoPendiente } from "@/components/ui/estado-pendiente";

export const metadata = { title: "Estructura del curso" };

/** Jerarquia Curso -> Modulo -> Unidad -> Recurso con posiciones ordenadas. */
export default function PaginaEstructuraCurso() {
  return (
    <div className="space-y-6">
      <PageHeader
        titulo="Estructura"
        descripcion="Modulos, unidades y recursos ordenados, con identificadores estables."
      />
      <EstadoPendiente
        detalle="Arbol editable con reordenamiento, visibilidad y obligatoriedad de recursos, editor de bloques con autosave y carga multipart directa al almacenamiento de objetos."
        criterio="Autoria y publicacion / Multimedia y distribucion"
      />
    </div>
  );
}

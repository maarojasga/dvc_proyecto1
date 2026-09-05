import { PageHeader } from "@/components/ui/page-header";
import { EstadoPendiente } from "@/components/ui/estado-pendiente";

export const metadata = { title: "Metadatos del curso" };

export default function PaginaMetadatosCurso() {
  return (
    <div className="space-y-6">
      <PageHeader
        titulo="Metadatos del curso"
        descripcion="Titulo, resumen, descripcion, portada, etiquetas y criterios de aprobacion."
      />
      <EstadoPendiente
        detalle="Edicion de metadatos con autosave y validaciones previas a la publicacion."
        criterio="Autoria y publicacion"
      />
    </div>
  );
}

import { PageHeader } from "@/components/ui/page-header";
import { EstadoPendiente } from "@/components/ui/estado-pendiente";

export const metadata = { title: "Crear cuenta" };

export default function PaginaRegistro() {
  return (
    <div className="space-y-6">
      <PageHeader
        titulo="Crear cuenta"
        descripcion="Registro publico de estudiantes. Los profesores se crean unicamente por administracion."
      />
      <EstadoPendiente
        detalle="Alta de estudiante con validaciones y envio del correo de verificacion."
        criterio="Identidad, autorizacion y seguridad"
      />
    </div>
  );
}

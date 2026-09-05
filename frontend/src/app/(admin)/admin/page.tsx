import { PageHeader } from "@/components/ui/page-header";
import { EstadoPendiente } from "@/components/ui/estado-pendiente";

export const metadata = { title: "Administracion" };

export default function PaginaAdmin() {
  return (
    <div className="space-y-6">
      <PageHeader
        titulo="Administracion"
        descripcion="Gestion transversal de la plataforma: cuentas, auditoria y operacion."
      />
      <EstadoPendiente
        detalle="Resumen de la instancia: usuarios por estado, cursos publicados y trabajos en cola o en dead-letter queue."
        criterio="Identidad, autorizacion y seguridad / Calidad operativa"
      />
    </div>
  );
}

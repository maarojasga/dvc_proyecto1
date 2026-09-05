import { PageHeader } from "@/components/ui/page-header";
import { EstadoPendiente } from "@/components/ui/estado-pendiente";

export const metadata = { title: "Auditoria" };

export default function PaginaAdminAuditoria() {
  return (
    <div className="space-y-6">
      <PageHeader
        titulo="Auditoria"
        descripcion="Registro inmutable de acciones sensibles sobre cuentas, contenido e insignias."
      />
      <EstadoPendiente
        detalle="Consulta paginada por actor, accion y rango de fechas, con exportacion y correlacion con trazas."
        criterio="Identidad, autorizacion y seguridad"
      />
    </div>
  );
}

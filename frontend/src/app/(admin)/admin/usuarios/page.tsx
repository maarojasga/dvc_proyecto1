import { PageHeader } from "@/components/ui/page-header";
import { EstadoPendiente } from "@/components/ui/estado-pendiente";

export const metadata = { title: "Usuarios y roles" };

export default function PaginaAdminUsuarios() {
  return (
    <div className="space-y-6">
      <PageHeader
        titulo="Usuarios y roles"
        descripcion="Altas de profesores, cambios de rol, suspension y revocacion de sesiones."
      />
      <EstadoPendiente
        detalle="Listado con filtros, invitacion de profesores, cambio de estado auditado y proteccion del ultimo administrador activo."
        criterio="Identidad, autorizacion y seguridad"
      />
    </div>
  );
}

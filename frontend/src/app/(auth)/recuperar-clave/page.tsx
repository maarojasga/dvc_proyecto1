import { PageHeader } from "@/components/ui/page-header";
import { EstadoPendiente } from "@/components/ui/estado-pendiente";

export const metadata = { title: "Recuperar contraseña" };

export default function PaginaRecuperarClave() {
  return (
    <div className="space-y-6">
      <PageHeader
        titulo="Recuperar contraseña"
        descripcion="Solicitud y restablecimiento de la contraseña con token de un solo uso."
      />
      <EstadoPendiente
        detalle="Solicitud del enlace, canje del token y revocacion de las sesiones activas."
        criterio="Identidad, autorizacion y seguridad"
      />
    </div>
  );
}

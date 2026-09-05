import { PageHeader } from "@/components/ui/page-header";
import { EstadoPendiente } from "@/components/ui/estado-pendiente";

export const metadata = { title: "Verificar correo" };

export default function PaginaVerificarCorreo() {
  return (
    <div className="space-y-6">
      <PageHeader
        titulo="Verificar correo"
        descripcion="Confirmacion del correo mediante el enlace enviado tras el registro."
      />
      <EstadoPendiente
        detalle="Canje del token de verificacion y activacion de la cuenta."
        criterio="Identidad, autorizacion y seguridad"
      />
    </div>
  );
}

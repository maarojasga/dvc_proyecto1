import { PageHeader } from "@/components/ui/page-header";
import { EstadoPendiente } from "@/components/ui/estado-pendiente";

export const metadata = { title: "Iniciar sesion" };

export default function PaginaLogin() {
  return (
    <div className="space-y-6">
      <PageHeader
        titulo="Iniciar sesion"
        descripcion="Acceso con correo y contraseña; la sesion se emite y revoca desde la API."
      />
      <EstadoPendiente
        detalle="Formulario de credenciales, proteccion CSRF, limite de intentos y manejo de sesion revocable."
        criterio="Identidad, autorizacion y seguridad"
      />
    </div>
  );
}

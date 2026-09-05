import { PageHeader } from "@/components/ui/page-header";
import { FormularioRegistro } from "@/components/auth/formulario-registro";

export const metadata = { title: "Crear cuenta" };

export default function PaginaRegistro() {
  return (
    <div className="space-y-6">
      <PageHeader
        titulo="Crear cuenta"
        descripcion="El registro publico crea cuentas de estudiante. Los profesores se dan de alta por administracion."
      />
      <FormularioRegistro />
    </div>
  );
}

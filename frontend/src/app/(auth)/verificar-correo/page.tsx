import { PageHeader } from "@/components/ui/page-header";
import { FormularioVerificacion } from "@/components/auth/formulario-verificacion";

export const metadata = { title: "Verificar correo" };

export default async function PaginaVerificarCorreo({
  searchParams,
}: {
  searchParams: Promise<{ token?: string }>;
}) {
  const { token } = await searchParams;
  return (
    <div className="space-y-6">
      <PageHeader
        titulo="Verificar correo"
        descripcion="La cuenta se activa al abrir el enlace que enviamos por correo."
      />
      <FormularioVerificacion token={token} />
    </div>
  );
}

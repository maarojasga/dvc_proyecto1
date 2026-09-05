import { PageHeader } from "@/components/ui/page-header";
import { FormularioRecuperacion } from "@/components/auth/formulario-recuperacion";

export const metadata = { title: "Recuperar clave" };

export default async function PaginaRecuperarClave({
  searchParams,
}: {
  searchParams: Promise<{ token?: string }>;
}) {
  const { token } = await searchParams;
  return (
    <div className="space-y-6">
      <PageHeader
        titulo="Recuperar clave"
        descripcion={
          token
            ? "Elige una clave nueva para tu cuenta."
            : "Te enviaremos un enlace para restablecerla."
        }
      />
      <FormularioRecuperacion token={token} />
    </div>
  );
}

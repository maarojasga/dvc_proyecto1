import Link from "next/link";
import { PageHeader } from "@/components/ui/page-header";
import { FormularioLogin } from "@/components/auth/formulario-login";

export const metadata = { title: "Iniciar sesion" };

export default function PaginaLogin() {
  return (
    <div className="space-y-6">
      <PageHeader titulo="Iniciar sesion" />
      <FormularioLogin />
      <p className="text-sm text-texto-suave">
        ¿No tienes cuenta?{" "}
        <Link href="/registro" className="underline">
          Crear una
        </Link>
      </p>
    </div>
  );
}

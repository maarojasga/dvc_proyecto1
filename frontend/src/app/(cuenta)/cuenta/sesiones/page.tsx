import { PageHeader } from "@/components/ui/page-header";
import { ListaSesiones } from "@/components/auth/lista-sesiones";

export const metadata = { title: "Sesiones activas" };

export default function PaginaSesiones() {
  return (
    <div className="space-y-6">
      <PageHeader
        titulo="Sesiones activas"
        descripcion="Cada dispositivo con la sesion abierta. Revocar una la invalida de inmediato."
      />
      <ListaSesiones />
    </div>
  );
}

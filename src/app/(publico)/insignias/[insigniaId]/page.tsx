import { PageHeader } from "@/components/ui/page-header";
import { EstadoPendiente } from "@/components/ui/estado-pendiente";
import { Etiqueta } from "@/components/ui/etiqueta";

export const metadata = { title: "Verificacion de insignia" };

/**
 * Verificacion publica de una insignia. No requiere sesion y no debe exponer
 * el correo del estudiante.
 */
export default async function PaginaVerificarInsignia({
  params,
}: {
  params: Promise<{ insigniaId: string }>;
}) {
  const { insigniaId } = await params;

  return (
    <div className="space-y-6">
      <PageHeader
        titulo="Verificacion de insignia"
        descripcion="Consulta publica de autenticidad, sin exponer datos personales del estudiante."
        acciones={<Etiqueta>{insigniaId}</Etiqueta>}
      />
      <EstadoPendiente
        detalle="Consulta del emisor, curso, fecha de emision y estado de revocacion a partir del identificador publico."
        criterio="Progreso e insignias"
      />
    </div>
  );
}

import { PageHeader } from "@/components/ui/page-header";
import { EstadoPendiente } from "@/components/ui/estado-pendiente";

export const metadata = { title: "Quiz" };

/**
 * Intento de quiz. La clave correcta nunca llega al cliente y la calificacion
 * se calcula en el servidor.
 */
export default async function PaginaQuiz({
  params,
}: {
  params: Promise<{ cursoId: string; quizId: string }>;
}) {
  await params;

  return (
    <div className="flex-1 space-y-6">
      <PageHeader
        titulo="Intento de quiz"
        descripcion="Snapshot del cuestionario, guardado parcial y envio definitivo idempotente."
      />
      <EstadoPendiente
        detalle="Presentacion del snapshot sin claves correctas, guardados parciales, control de expiracion y envio con Idempotency-Key."
        criterio="Evaluacion academica"
      />
    </div>
  );
}

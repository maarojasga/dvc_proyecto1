import { PageHeader } from "@/components/ui/page-header";
import { EstadoPendiente } from "@/components/ui/estado-pendiente";

export const metadata = { title: "Operacion" };

/** Visibilidad sobre los workers asincronos: reintentos, DLQ y reencolado. */
export default function PaginaAdminOperacion() {
  return (
    <div className="space-y-6">
      <PageHeader
        titulo="Operacion"
        descripcion="Estado de los trabajos asincronos de procesamiento multimedia y sus fallos."
      />
      <EstadoPendiente
        detalle="Cola asynq, trabajos en proceso, reintentos con backoff, dead-letter queue, alertas y reencolado con la misma clave de idempotencia."
        criterio="Arquitectura y despliegue / Calidad operativa"
      />
    </div>
  );
}

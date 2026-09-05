import { Card, CardText, CardTitle } from "@/components/ui/card";

/**
 * Marcador de pantalla aun no implementada.
 *
 * Toda ruta de la base lo usa para que la navegacion sea recorrible sin
 * simular funcionalidad que no existe.
 */
export function EstadoPendiente({
  titulo = "Pendiente de implementacion",
  detalle,
  criterio,
}: {
  titulo?: string;
  detalle: string;
  /** Criterio de evaluacion del proyecto al que aporta esta pantalla. */
  criterio?: string;
}) {
  return (
    <Card>
      <CardTitle>{titulo}</CardTitle>
      <CardText>{detalle}</CardText>
      {criterio ? (
        <p className="mt-3 text-xs text-texto-suave">
          <span className="font-medium">Criterio:</span> {criterio}
        </p>
      ) : null}
    </Card>
  );
}

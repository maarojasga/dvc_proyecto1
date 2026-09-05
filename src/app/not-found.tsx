import Link from "next/link";
import { Container } from "@/components/ui/container";

export default function NoEncontrado() {
  return (
    <Container className="space-y-4">
      <h1 className="text-2xl font-semibold">Pagina no encontrada</h1>
      <p className="text-texto-suave">
        La ruta solicitada no existe en la plataforma.
      </p>
      <Link href="/" className="text-acento underline">
        Volver al inicio
      </Link>
    </Container>
  );
}

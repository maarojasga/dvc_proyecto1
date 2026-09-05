import Link from "next/link";
import { Container } from "@/components/ui/container";
import { Card, CardText, CardTitle } from "@/components/ui/card";

const modulos = [
  {
    href: "/catalogo",
    titulo: "Catalogo",
    texto: "Busqueda, filtros e inscripcion a cursos publicados.",
  },
  {
    href: "/mis-cursos",
    titulo: "Aprendizaje",
    texto: "Consumo de contenido, progreso validado en servidor e insignias.",
  },
  {
    href: "/autoria",
    titulo: "Autoria",
    texto: "Curso, modulo, unidad y recurso con versiones publicadas inmutables.",
  },
  {
    href: "/admin",
    titulo: "Administracion",
    texto: "Usuarios, roles, sesiones, auditoria y operacion de la plataforma.",
  },
];

export default function PaginaInicio() {
  return (
    <Container className="space-y-10">
      <section className="max-w-2xl space-y-4">
        <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
          Plataforma de cursos masivos abiertos en linea
        </h1>
        <p className="text-texto-suave">
          Base del frontend en Next.js. Las rutas estan creadas y navegables, sin
          logica funcional todavia: el consumo de la API en Go bajo{" "}
          <code className="font-mono text-sm">/api/v1</code> se implementa por
          modulo en las siguientes entregas.
        </p>
      </section>

      <section aria-labelledby="modulos" className="space-y-4">
        <h2 id="modulos" className="text-lg font-semibold">
          Modulos
        </h2>
        <div className="grid gap-4 sm:grid-cols-2">
          {modulos.map((modulo) => (
            <Link key={modulo.href} href={modulo.href} className="block">
              <Card className="h-full transition-colors hover:border-acento">
                <CardTitle>{modulo.titulo}</CardTitle>
                <CardText>{modulo.texto}</CardText>
              </Card>
            </Link>
          ))}
        </div>
      </section>
    </Container>
  );
}

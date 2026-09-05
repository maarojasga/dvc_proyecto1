import type { Rol } from "@/types";

export interface EnlaceNav {
  href: string;
  etiqueta: string;
  /** Roles que ven el enlace. Vacio = publico. */
  roles?: Rol[];
}

export const navPrincipal: EnlaceNav[] = [
  { href: "/catalogo", etiqueta: "Catalogo" },
  { href: "/mis-cursos", etiqueta: "Mis cursos", roles: ["estudiante"] },
  { href: "/autoria", etiqueta: "Autoria", roles: ["profesor"] },
  { href: "/admin", etiqueta: "Administracion", roles: ["administrador"] },
];

export const navAdmin: EnlaceNav[] = [
  { href: "/admin", etiqueta: "Resumen" },
  { href: "/admin/usuarios", etiqueta: "Usuarios y roles" },
  { href: "/admin/auditoria", etiqueta: "Auditoria" },
  { href: "/admin/operacion", etiqueta: "Operacion" },
];

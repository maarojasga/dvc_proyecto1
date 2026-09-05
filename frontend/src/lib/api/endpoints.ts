/**
 * Rutas de la API agrupadas por modulo del monolito en Go.
 *
 * Centralizarlas evita cadenas dispersas y deja explicito el contrato que el
 * frontend espera de OpenAPI.
 */
export const endpoints = {
  auth: {
    registro: "/auth/registro",
    login: "/auth/login",
    logout: "/auth/logout",
    sesionActual: "/auth/sesion",
    verificarCorreo: "/auth/verificar-correo",
    reenviarVerificacion: "/auth/reenviar-verificacion",
    recuperarClave: "/auth/recuperar-clave",
    confirmarRecuperacion: "/auth/recuperar-clave/confirmar",
    sesiones: "/auth/sesiones",
    sesion: (sesionId: string) => `/auth/sesiones/${sesionId}`,
  },
  catalogo: {
    listar: "/catalogo/cursos",
    detalle: (slug: string) => `/catalogo/cursos/${slug}`,
  },
  autoria: {
    cursos: "/autoria/cursos",
    curso: (cursoId: string) => `/autoria/cursos/${cursoId}`,
    estructura: (cursoId: string) => `/autoria/cursos/${cursoId}/estructura`,
    versiones: (cursoId: string) => `/autoria/cursos/${cursoId}/versiones`,
    publicar: (cursoId: string) => `/autoria/cursos/${cursoId}/publicar`,
  },
  archivos: {
    /** Devuelve URLs prefirmadas para la carga multipart directa al object storage. */
    iniciarCarga: "/archivos/cargas",
    completarCarga: (cargaId: string) => `/archivos/cargas/${cargaId}/completar`,
  },
  inscripciones: {
    listar: "/inscripciones",
    crear: "/inscripciones",
    retirar: (inscripcionId: string) => `/inscripciones/${inscripcionId}/retiro`,
  },
  progreso: {
    resumen: (cursoId: string) => `/cursos/${cursoId}/progreso`,
    heartbeat: (cursoId: string) => `/cursos/${cursoId}/progreso/heartbeat`,
  },
  quizzes: {
    iniciarIntento: (quizId: string) => `/quizzes/${quizId}/intentos`,
    guardarParcial: (intentoId: string) => `/intentos/${intentoId}/parcial`,
    enviar: (intentoId: string) => `/intentos/${intentoId}/envio`,
  },
  insignias: {
    propias: "/insignias",
    verificar: (insigniaId: string) => `/insignias/${insigniaId}/verificacion`,
  },
  admin: {
    usuarios: "/admin/usuarios",
    usuario: (usuarioId: string) => `/admin/usuarios/${usuarioId}`,
    auditoria: "/admin/auditoria",
  },
} as const;

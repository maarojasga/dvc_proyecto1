/**
 * Modelo de dominio de la plataforma MOOC.
 *
 * Refleja la jerarquia academica Curso -> Modulo -> Unidad -> Recurso descrita en
 * la especificacion. Son tipos de solo lectura para la capa de presentacion: la
 * fuente de verdad es PostgreSQL, expuesta por la API en Go bajo /api/v1.
 */

export type Rol = "administrador" | "profesor" | "estudiante";

export type EstadoUsuario = "pendiente" | "activo" | "suspendido";

export interface Usuario {
  id: string;
  nombre: string;
  correo: string;
  roles: Rol[];
  estado: EstadoUsuario;
  creadoEn: string;
}

/** Una version publicada es inmutable; toda edicion ocurre sobre un borrador. */
export type EstadoCurso = "borrador" | "publicado" | "despublicado" | "archivado";

export interface Curso {
  id: string;
  slug: string;
  titulo: string;
  resumen: string;
  descripcion: string;
  estado: EstadoCurso;
  versionVigente: number | null;
  portadaUrl: string | null;
  etiquetas: string[];
  actualizadoEn: string;
}

export interface VersionCurso {
  id: string;
  cursoId: string;
  numero: number;
  publicada: boolean;
  publicadaEn: string | null;
  modulos: Modulo[];
}

export interface Modulo {
  /** Identificador estable: sobrevive a nuevas versiones y preserva el progreso. */
  stableId: string;
  titulo: string;
  posicion: number;
  unidades: Unidad[];
}

export interface Unidad {
  stableId: string;
  titulo: string;
  posicion: number;
  recursos: Recurso[];
}

export type TipoRecurso =
  | "texto"
  | "imagen"
  | "video"
  | "audio"
  | "pdf"
  | "presentacion"
  | "descargable"
  | "iframe"
  | "enlace"
  | "quiz";

/** Estado del pipeline asincrono (transcodificacion HLS, conversion a PDF). */
export type EstadoProcesamiento =
  | "no_aplica"
  | "pendiente"
  | "procesando"
  | "listo"
  | "fallido";

export interface Recurso {
  stableId: string;
  tipo: TipoRecurso;
  titulo: string;
  posicion: number;
  visible: boolean;
  obligatorio: boolean;
  descargable: boolean;
  estadoProcesamiento: EstadoProcesamiento;
}

export interface Quiz {
  id: string;
  recursoStableId: string;
  titulo: string;
  intentosPermitidos: number;
  notaAprobatoria: number;
  duracionMinutos: number | null;
}

/**
 * Pregunta tal como viaja al cliente: nunca incluye la clave correcta.
 * La calificacion se calcula exclusivamente en el servidor.
 */
export interface PreguntaQuiz {
  id: string;
  enunciado: string;
  opciones: OpcionQuiz[];
  seleccionMultiple: boolean;
}

export interface OpcionQuiz {
  id: string;
  texto: string;
}

export type EstadoInscripcion = "activa" | "retirada" | "completada" | "aprobada";

export interface Inscripcion {
  id: string;
  cursoId: string;
  versionNumero: number;
  estado: EstadoInscripcion;
  inscritoEn: string;
}

/** El avance lo determina el servidor a partir de heartbeats y eventos de apertura. */
export interface Progreso {
  cursoId: string;
  obligatoriosTotales: number;
  obligatoriosCompletados: number;
  porcentaje: number;
  ultimoRecursoStableId: string | null;
  ultimaPosicionSegundos: number | null;
}

export interface Insignia {
  id: string;
  cursoId: string;
  cursoTitulo: string;
  emitidaEn: string;
  revocadaEn: string | null;
  /** URL publica de verificacion; no expone el correo del estudiante. */
  urlVerificacion: string;
  imagenUrl: string;
}

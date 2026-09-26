const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export class ApiError extends Error {
  status: number;
  code: string;
  details?: string[];

  constructor(status: number, code: string, message: string, details?: string[]) {
    super(message);
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

const CSRF_COOKIE = "mooc_csrf";
const CSRF_HEADER = "X-CSRF-Token";
const SAFE_METHODS = new Set(["GET", "HEAD", "OPTIONS"]);

/**
 * Lee el token anti-CSRF que la API emite junto con la sesión.
 *
 * La cookie de sesión es httpOnly y no se puede leer; esta sí, a propósito:
 * repetirla en la cabecera es lo que un sitio atacante no puede hacer, porque
 * no tiene acceso a las cookies de este origen.
 */
function csrfToken(): string | undefined {
  if (typeof document === "undefined") return undefined;
  const entry = document.cookie.split("; ").find((c) => c.startsWith(`${CSRF_COOKIE}=`));
  return entry ? decodeURIComponent(entry.slice(CSRF_COOKIE.length + 1)) : undefined;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const method = (init?.method ?? "GET").toUpperCase();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...((init?.headers as Record<string, string>) ?? {}),
  };
  if (!SAFE_METHODS.has(method)) {
    const token = csrfToken();
    if (token) headers[CSRF_HEADER] = token;
  }

  const res = await fetch(`${API_URL}${path}`, {
    ...init,
    credentials: "include",
    headers,
  });

  if (res.status === 204) {
    return undefined as T;
  }

  const body = await res.json().catch(() => null);

  if (!res.ok) {
    const err = body?.error;
    throw new ApiError(res.status, err?.code ?? "unknown", err?.message ?? "Error inesperado", err?.details);
  }
  return body as T;
}

export interface User {
  id: string;
  email: string;
  full_name: string;
  role: "admin" | "teacher" | "student";
  status: "pending_verification" | "active" | "suspended";
}

export interface Session {
  id: string;
  created_at: string;
  expires_at: string;
  ip_address?: string;
  user_agent?: string;
  current: boolean;
}

import type { PistaDeSubtitulos } from "@/components/ReproductorHLS";

export type { PistaDeSubtitulos };

export interface ResourceContent {
  type: string;
  // Identifica el recurso a través de las versiones: lo usa el foro para atar
  // una conversación a la lección y no a la fila.
  stable_id: string;
  title: string;
  downloadable: boolean;
  url?: string;
  cdn?: boolean;
  expires_in?: number;
  markdown?: string;
  external_url?: string;
  position_seconds?: number;
  // El archivo tal como lo subio el profesor, cuando el recurso se presenta
  // convertido (una presentacion) y ademas es descargable.
  original_url?: string;
  // Atributos de seguridad de un iframe. Los decide el servidor a partir de la
  // lista blanca; el cliente los aplica tal cual.
  sandbox?: string;
  allow?: string;
  referrer_policy?: string;
}

// Destino autorizado para incrustar.
export interface IframeDestino {
  id: string;
  host: string;
  include_subdomains: boolean;
  permissions?: string;
  description?: string;
  example_url?: string;
}

export interface AuditEntry {
  id: string;
  actor_id?: string;
  actor_email?: string;
  action: string;
  entity_type: string;
  entity_id?: string;
  metadata: Record<string, unknown>;
  ip_address?: string;
  created_at: string;
}

export interface QuizOption {
  stable_id: string;
  text_md: string;
}

export interface QuizQuestion {
  stable_id: string;
  prompt_md: string;
  type: "single" | "multiple";
  points: number;
  options: QuizOption[];
}

export interface QuizAttempt {
  attempt_id: string;
  quiz_id: string;
  title: string;
  attempt_number: number;
  status: "in_progress" | "submitted" | "expired";
  started_at: string;
  expires_at?: string;
  questions: QuizQuestion[];
  // Clave: stable_id de la pregunta. Valor: stable_id de las opciones marcadas.
  answers: Record<string, string[]>;
  // Solo llegan si la politica de retroalimentacion del quiz los permite en
  // este momento (por ejemplo, tras enviar el intento).
  score?: number;
  passed?: boolean;
}

export interface QuizQuestionDraft {
  prompt_md: string;
  type: "single" | "multiple";
  points: number;
  options: { text_md: string; is_correct: boolean }[];
}

export interface CourseProgress {
  enrollment_id: string;
  course_id: string;
  status: "active" | "withdrawn" | "completed" | "approved";
  required_percent: number;
  required_total: number;
  required_completed: number;
  quizzes_pending: number;
  completed_at?: string;
  approved_at?: string;
  badge_code?: string;
}

// Vista propia de una insignia (autenticado): el struct de dominio no declara
// tags JSON, asi que Go serializa los nombres de campo tal cual.
// La insignia tal como la ve su dueno. La API resuelve la URL de la imagen,
// porque es ella la que sabe si los objetos salen del CDN o van firmados.
export interface Badge {
  code: string;
  course_id: string;
  valid: boolean;
  issued_at: string;
  revoked_at?: string | null;
  image_url?: string;
  verify_path: string;
}

// Verificacion publica de una insignia: no exige sesion ni expone datos
// personales, por eso es un DTO aparte con sus propios tags JSON.
export interface BadgeVerification {
  code: string;
  course_id: string;
  valid: boolean;
  issued_at: string;
  revoked_at?: string | null;
  image_url?: string;
}

// Clasificación de los cambios de un borrador de actualización.
//
// El alcance resume el impacto sobre el progreso ya registrado: "menor" no
// cambia lo que hay que completar, "mayor" sí.
export interface Cambio {
  tipo: "agregado" | "eliminado" | "modificado" | "movido";
  elemento: "modulo" | "unidad" | "recurso";
  stable_id: string;
  titulo: string;
  detalle?: string;
  afecta_progreso: boolean;
}

export interface Clasificacion {
  alcance: "ninguno" | "menor" | "mayor";
  cambios: Cambio[];
  obligatorios_agregados?: string[];
  obligatorios_eliminados?: string[];
}

// Métricas del panel administrativo. El desglose es un mapa abierto para que
// añadir un estado en la base no obligue a tocar el cliente.
export interface Conteo {
  total: number;
  desglose: Record<string, number>;
}

export interface Metricas {
  usuarios: Conteo;
  cursos: Conteo;
  inscripciones: Conteo;
  insignias: Conteo;
  multimedia: Conteo;
  evaluaciones: Conteo;
}

export interface ResumenDeOpcion {
  stable_id: string;
  text_md: string;
  es_correcta: boolean;
  elegida: number;
  porcentaje: number;
}

export interface ResumenDePregunta {
  stable_id: string;
  prompt_md: string;
  respondida: number;
  en_blanco: number;
  aciertos: number;
  tasa_acierto: number;
  opciones: ResumenDeOpcion[];
}

export interface ResultadosDeQuiz {
  quiz_id: string;
  title: string;
  resultados: {
    intentos: number;
    estudiantes: number;
    nota_media: number;
    nota_mediana: number;
    tasa_aprobado: number;
    preguntas: ResumenDePregunta[];
  };
}

// Una revisión guardada del contenido de un recurso. La lista no trae el
// contenido: se pide por revisión al mirar una en concreto.
export interface Revision {
  id: string;
  resource_id: string;
  revision_number: number;
  content_md?: string;
  author_email?: string;
  created_at: string;
}

// Profesor con acceso de edición a un curso ajeno.
export interface Colaborador {
  user_id: string;
  email: string;
  full_name: string;
  role: string;
  added_at: string;
}

// Hilo del foro de un curso.
export interface Hilo {
  id: string;
  course_id: string;
  resource_stable_id?: string;
  author_name?: string;
  title: string;
  body_md: string;
  locked: boolean;
  replies: number;
  created_at: string;
  last_activity_at: string;
}

export interface RespuestaDeForo {
  id: string;
  thread_id: string;
  author_name?: string;
  body_md: string;
  created_at: string;
  deleted: boolean;
}

// Una parte ya recibida por el almacen. El ETag es lo que el cliente reenvia
// al completar, y lo que delata una parte que no llego intacta.
export interface ParteCargada {
  part_number: number;
  etag: string;
  size_bytes?: number;
}

// Lo que la API responde tras verificar un objeto cargado: el tipo real
// deducido de los bytes, el tamano y el checksum que calculo el servidor.
export interface CargaVerificada {
  status: "ready" | "queued";
  mime_type?: string;
  size_bytes?: number;
  checksum_sha256?: string;
  media_asset_id?: string;
}

export const api = {
  me: () => request<User>("/api/v1/auth/me"),
  // Responde igual exista o no el correo, así que no devuelve el usuario.
  register: (data: { email: string; password: string; full_name: string }) =>
    request<{ message: string }>("/api/v1/auth/register", { method: "POST", body: JSON.stringify(data) }),
  resendVerification: (email: string) =>
    request<{ message: string }>("/api/v1/auth/resend-verification", {
      method: "POST",
      body: JSON.stringify({ email }),
    }),
  listSessions: () => request<{ items: Session[] }>("/api/v1/auth/sessions"),
  revokeSession: (id: string) =>
    request<void>(`/api/v1/auth/sessions/${id}`, { method: "DELETE" }),
  revokeOtherSessions: () =>
    request<{ revoked: number }>("/api/v1/auth/sessions", { method: "DELETE" }),
  verifyEmail: (token: string) =>
    request<{ status: string }>("/api/v1/auth/verify-email", { method: "POST", body: JSON.stringify({ token }) }),
  // La respuesta no trae el token de sesión: vive solo en la cookie httpOnly.
  login: (email: string, password: string) =>
    request<{ user: User; expires_at: string }>("/api/v1/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    }),
  logout: () => request<{ status: string }>("/api/v1/auth/logout", { method: "POST" }),
  resourceContent: (resourceId: string) =>
    request<ResourceContent>(`/api/v1/resources/${resourceId}/content`),
  saveResourcePosition: (resourceId: string, position_seconds: number) =>
    request<void>(`/api/v1/resources/${resourceId}/position`, {
      method: "PUT",
      body: JSON.stringify({ position_seconds }),
    }),
  listAudit: (params: { action?: string; limit?: number } = {}) => {
    const q = new URLSearchParams();
    if (params.action) q.set("action", params.action);
    if (params.limit) q.set("limit", String(params.limit));
    const cadena = q.toString();
    return request<{ items: AuditEntry[] }>(`/api/v1/admin/audit${cadena ? `?${cadena}` : ""}`);
  },
  listUserSessions: (userId: string) =>
    request<{ items: Session[] }>(`/api/v1/admin/users/${userId}/sessions`),
  revokeUserSessions: (userId: string) =>
    request<void>(`/api/v1/admin/users/${userId}/sessions`, { method: "DELETE" }),
  requestPasswordReset: (email: string) =>
    request<{ status: string }>("/api/v1/auth/password/reset-request", {
      method: "POST",
      body: JSON.stringify({ email }),
    }),
  confirmPasswordReset: (token: string, new_password: string) =>
    request<{ status: string }>("/api/v1/auth/password/reset-confirm", {
      method: "POST",
      body: JSON.stringify({ token, new_password }),
    }),

  listUsers: (params: Record<string, string> = {}) =>
    request<{ items: User[] }>(`/api/v1/admin/users?${new URLSearchParams(params)}`),
  updateUserRole: (id: string, role: string) =>
    request<{ status: string }>(`/api/v1/admin/users/${id}/role`, {
      method: "PATCH",
      body: JSON.stringify({ role }),
    }),
  updateUserStatus: (id: string, status: string) =>
    request<{ status: string }>(`/api/v1/admin/users/${id}/status`, {
      method: "PATCH",
      body: JSON.stringify({ status }),
    }),
  createTeacher: (data: { email: string; full_name: string }) =>
    request<User>("/api/v1/admin/teachers", { method: "POST", body: JSON.stringify(data) }),

  createCourse: (data: { slug: string; title: string }) =>
    request<{ course_id: string; version_id: string; version_number: number }>("/api/v1/courses", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  listMyCourses: () => request<{ items: Course[] }>("/api/v1/courses/mine"),
  createUpdateDraft: (courseId: string) =>
    request<Version>(`/api/v1/courses/${courseId}/update-draft`, { method: "POST" }),
  unpublishCourse: (courseId: string) =>
    request<{ status: string }>(`/api/v1/courses/${courseId}/unpublish`, { method: "POST" }),
  getVersion: (versionId: string) => request<Version>(`/api/v1/courses/versions/${versionId}`),
  updateVersionMetadata: (versionId: string, data: Partial<Version>) =>
    request<{ status: string }>(`/api/v1/courses/versions/${versionId}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    }),
  // Clasificación de lo que un borrador cambia respecto a lo publicado.
  versionChanges: (versionId: string) =>
    request<Clasificacion>(`/api/v1/courses/versions/${versionId}/changes`),
  publishVersion: (versionId: string) =>
    request<{ status: string }>(`/api/v1/courses/versions/${versionId}/publish`, { method: "POST" }),

  addModule: (versionId: string, data: { title: string; position: number }) =>
    request<Module>(`/api/v1/courses/versions/${versionId}/modules`, {
      method: "POST",
      body: JSON.stringify(data),
    }),
  deleteModule: (versionId: string, moduleId: string) =>
    request<void>(`/api/v1/courses/versions/${versionId}/modules/${moduleId}`, { method: "DELETE" }),
  addUnit: (versionId: string, moduleId: string, data: { title: string; position: number }) =>
    request<Unit>(`/api/v1/courses/versions/${versionId}/modules/${moduleId}/units`, {
      method: "POST",
      body: JSON.stringify(data),
    }),
  deleteUnit: (versionId: string, unitId: string) =>
    request<void>(`/api/v1/courses/versions/${versionId}/units/${unitId}`, { method: "DELETE" }),
  addResource: (versionId: string, unitId: string, data: NuevoRecurso) =>
    request<Resource>(`/api/v1/courses/versions/${versionId}/units/${unitId}/resources`, {
      method: "POST",
      // El cuerpo se arma campo a campo en vez de reenviar el objeto tal cual.
      // La respuesta del recurso viene en PascalCase (es la serialización por
      // defecto del dominio en Go) pero la petición se declara en snake_case,
      // así que mandar de vuelta la misma forma que se recibe produce un 400 y
      // el profesor no puede añadir ningún recurso. Traducir aquí, en el
      // adaptador al cable, es lo que impide que la asimetría se filtre a las
      // pantallas.
      body: JSON.stringify({
        type: data.type,
        title: data.title,
        position: data.position,
        visible: data.visible,
        required: data.required,
        downloadable: data.downloadable ?? false,
        text_content_md: data.text_content_md ?? "",
        external_url: data.external_url ?? "",
      }),
    }),
  deleteResource: (versionId: string, resourceId: string) =>
    request<void>(`/api/v1/courses/versions/${versionId}/resources/${resourceId}`, { method: "DELETE" }),
  requestUploadUrl: (versionId: string, resourceId: string, mimeType: string) =>
    request<{ upload_url: string; object_key: string; method: string }>(
      `/api/v1/courses/versions/${versionId}/resources/${resourceId}/upload-url`,
      { method: "POST", body: JSON.stringify({ mime_type: mimeType }) },
    ),
  confirmUpload: (versionId: string, resourceId: string, checksumSha256?: string) =>
    request<CargaVerificada>(
      `/api/v1/courses/versions/${versionId}/resources/${resourceId}/confirm-upload`,
      { method: "POST", body: JSON.stringify(checksumSha256 ? { checksum_sha256: checksumSha256 } : {}) },
    ),

  initiateMultipart: (versionId: string, resourceId: string, contentType: string) =>
    request<{ upload_id: string; object_key: string }>(
      `/api/v1/courses/versions/${versionId}/resources/${resourceId}/multipart/initiate`,
      { method: "POST", body: JSON.stringify({ content_type: contentType }) },
    ),
  getMultipartPartUrl: (versionId: string, resourceId: string, uploadId: string, partNumber: number) =>
    request<{ upload_url: string; part_number: number }>(
      `/api/v1/courses/versions/${versionId}/resources/${resourceId}/multipart/part-url`,
      { method: "POST", body: JSON.stringify({ upload_id: uploadId, part_number: partNumber }) },
    ),
  // Devuelve las partes que el almacen ya recibio. Es lo que permite reanudar
  // una subida interrumpida sin reenviar lo que llego.
  listMultipartParts: (versionId: string, resourceId: string, uploadId: string) =>
    request<{ upload_id: string; parts: ParteCargada[] }>(
      `/api/v1/courses/versions/${versionId}/resources/${resourceId}/multipart/parts?upload_id=${encodeURIComponent(uploadId)}`,
    ),
  completeMultipart: (
    versionId: string,
    resourceId: string,
    uploadId: string,
    parts: ParteCargada[],
    checksumSha256?: string,
  ) =>
    request<CargaVerificada>(
      `/api/v1/courses/versions/${versionId}/resources/${resourceId}/multipart/complete`,
      {
        method: "POST",
        body: JSON.stringify({
          upload_id: uploadId,
          parts: parts.map((p) => ({ part_number: p.part_number, etag: p.etag })),
          ...(checksumSha256 ? { checksum_sha256: checksumSha256 } : {}),
        }),
      },
    ),

  // La lista blanca de iframes. El profesor la consulta para saber qué
  // destinos puede incrustar; solo la administración la modifica.
  listIframeAllowlist: () =>
    request<{ items: IframeDestino[]; sandbox: string; referrer_policy: string }>("/api/v1/iframe-allowlist"),
  addIframeDestino: (data: {
    host: string;
    include_subdomains: boolean;
    permissions: string;
    description: string;
  }) =>
    request<IframeDestino>("/api/v1/admin/iframe-allowlist", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  removeIframeDestino: (id: string) =>
    request<void>(`/api/v1/admin/iframe-allowlist/${id}`, { method: "DELETE" }),

  platformMetrics: () => request<Metricas>("/api/v1/admin/metrics"),
  // Resultados agregados de una evaluación: los ve su autor o la administración.
  quizResults: (versionId: string, resourceId: string) =>
    request<ResultadosDeQuiz>(
      `/api/v1/courses/versions/${versionId}/resources/${resourceId}/results`,
    ),

  // Historial de revisiones de un recurso de texto.
  listRevisions: (versionId: string, resourceId: string) =>
    request<{ items: Revision[] }>(
      `/api/v1/courses/versions/${versionId}/resources/${resourceId}/revisions`,
    ),
  saveRevision: (versionId: string, resourceId: string, content_md: string) =>
    request<Revision | { status: string }>(
      `/api/v1/courses/versions/${versionId}/resources/${resourceId}/revisions`,
      { method: "POST", body: JSON.stringify({ content_md }) },
    ),
  getRevision: (versionId: string, resourceId: string, numero: number) =>
    request<Revision>(
      `/api/v1/courses/versions/${versionId}/resources/${resourceId}/revisions/${numero}`,
    ),
  restoreRevision: (versionId: string, resourceId: string, numero: number) =>
    request<{ status: string; restored_from?: number }>(
      `/api/v1/courses/versions/${versionId}/resources/${resourceId}/revisions/${numero}/restore`,
      { method: "POST" },
    ),

  // Coautoría de un curso. Va bajo /collaborators y no bajo
  // /courses/{id}/collaborators por un conflicto de patrones en el router del
  // servidor, que la API documenta.
  listCollaborators: (courseId: string) =>
    request<{ items: Colaborador[] }>(`/api/v1/collaborators/${courseId}`),
  addCollaborator: (courseId: string, email: string) =>
    request<{ user_id: string; email: string }>(`/api/v1/collaborators/${courseId}`, {
      method: "POST",
      body: JSON.stringify({ email }),
    }),
  removeCollaborator: (courseId: string, userId: string) =>
    request<void>(`/api/v1/collaborators/${courseId}/${userId}`, { method: "DELETE" }),

  // Subtítulos y transcripción de un recurso reproducible.
  listCaptions: (resourceId: string) =>
    request<{ items: PistaDeSubtitulos[] }>(`/api/v1/resources/${resourceId}/captions`),
  getTranscript: (resourceId: string, idioma: string) =>
    request<{ resource_title: string; language: string; transcript: string }>(
      `/api/v1/resources/${resourceId}/transcript/${idioma}`,
    ),
  saveCaptions: (
    versionId: string,
    resourceId: string,
    idioma: string,
    data: { label: string; kind: "subtitles" | "captions"; vtt: string },
  ) =>
    request<{ cues: number; duration_ms: number }>(
      `/api/v1/courses/versions/${versionId}/resources/${resourceId}/captions/${idioma}`,
      { method: "PUT", body: JSON.stringify(data) },
    ),

  // Foro asíncrono. Cuelga de /forum y no de /courses/{id}/forum por un
  // conflicto de patrones en el router del servidor, que la API documenta.
  listThreads: (courseId: string, resourceStableId?: string) =>
    request<{ items: Hilo[] }>(
      `/api/v1/forum/${courseId}${resourceStableId ? `?resource_stable_id=${resourceStableId}` : ""}`,
    ),
  createThread: (courseId: string, data: { title: string; body_md: string; resource_stable_id?: string }) =>
    request<Hilo>(`/api/v1/forum/${courseId}`, { method: "POST", body: JSON.stringify(data) }),
  getThread: (threadId: string) =>
    request<{ thread: Hilo; replies: RespuestaDeForo[] }>(`/api/v1/forum/threads/${threadId}`),
  replyToThread: (threadId: string, body_md: string) =>
    request<RespuestaDeForo>(`/api/v1/forum/threads/${threadId}/replies`, {
      method: "POST",
      body: JSON.stringify({ body_md }),
    }),
  deleteForumPost: (postId: string) =>
    request<void>(`/api/v1/forum/posts/${postId}`, { method: "DELETE" }),
  lockThread: (threadId: string, locked: boolean) =>
    request<{ locked: boolean }>(`/api/v1/forum/threads/${threadId}/lock`, {
      method: "POST",
      body: JSON.stringify({ locked }),
    }),

  // Credencial Open Badges 3.0 de una insignia. Publica: el sentido de una
  // credencial verificable es que cualquiera la compruebe sin cuenta aquí.
  openBadgeURL: (code: string) => `${API_URL}/api/v1/badges/${code}/openbadge`,
  openBadgeJWTURL: (code: string) => `${API_URL}/api/v1/badges/${code}/openbadge.jwt`,

  listCatalog: (params: Record<string, string> = {}) =>
    request<{ items: Version[] }>(`/api/v1/catalog?${new URLSearchParams(params)}`),
  getPublishedCourse: (courseId: string) => request<Version>(`/api/v1/catalog/${courseId}`),

  enroll: (courseId: string) =>
    request<Enrollment>("/api/v1/enrollments", { method: "POST", body: JSON.stringify({ course_id: courseId }) }),
  listMyEnrollments: () => request<{ items: Enrollment[] }>("/api/v1/enrollments/mine"),
  withdraw: (courseId: string) =>
    request<{ status: string }>(`/api/v1/enrollments/${courseId}/withdraw`, { method: "POST" }),

  // Autoria del quiz. Solo el profesor dueno y sobre un borrador; la
  // respuesta nunca trae la clave correcta, ni siquiera para quien lo define.
  defineQuiz: (
    versionId: string,
    resourceId: string,
    data: {
      title: string;
      time_limit_seconds?: number | null;
      max_attempts?: number | null;
      pass_score: number;
      feedback_policy: "immediate" | "after_submit" | "after_close" | "none";
      shuffle_questions: boolean;
      questions: QuizQuestionDraft[];
    },
  ) =>
    request<{ quiz_id: string; questions: number }>(
      `/api/v1/courses/versions/${versionId}/resources/${resourceId}/quiz`,
      { method: "PUT", body: JSON.stringify(data) },
    ),

  // Abre un intento, o devuelve el que ya estuviera en curso: recargar la
  // pagina no debe gastar otro intento.
  startQuizAttempt: (resourceId: string) =>
    request<QuizAttempt>(`/api/v1/resources/${resourceId}/quiz/attempts`, { method: "POST" }),
  getQuizAttempt: (attemptId: string) => request<QuizAttempt>(`/api/v1/quiz/attempts/${attemptId}`),
  // Guardado parcial de una pregunta; se llama en cada cambio de seleccion.
  saveQuizAnswer: (attemptId: string, questionStableId: string, selectedOptionStableIds: string[]) =>
    request<void>(`/api/v1/quiz/attempts/${attemptId}/answers`, {
      method: "PUT",
      body: JSON.stringify({
        question_stable_id: questionStableId,
        selected_option_stable_ids: selectedOptionStableIds,
      }),
    }),
  // La Idempotency-Key hace que reintentar tras un corte de red devuelva la
  // misma nota en vez de calificar otra vez.
  submitQuizAttempt: (attemptId: string, idempotencyKey: string) =>
    request<QuizAttempt>(`/api/v1/quiz/attempts/${attemptId}/submit`, {
      method: "POST",
      headers: { "Idempotency-Key": idempotencyKey },
    }),

  // El cliente solo reporta que abrio, que sigue ahi o que cerro; el servidor
  // decide cuanto tiempo acredita. Un porcentaje enviado desde aqui no serviria
  // de nada: la API lo rechazaria. `keepalive` deja que la peticion de cierre
  // sobreviva a que la pagina se este descargando (cambio de pestana o cierre).
  recordProgress: (
    resourceId: string,
    type: "open" | "heartbeat" | "close",
    complete = false,
    opts: { keepalive?: boolean } = {},
  ) =>
    request<CourseProgress>(`/api/v1/resources/${resourceId}/progress`, {
      method: "POST",
      body: JSON.stringify({ type, complete }),
      keepalive: opts.keepalive,
    }),
  getCourseProgress: (courseId: string) =>
    request<CourseProgress>(`/api/v1/enrollments/${courseId}/progress`),

  listMyBadges: () => request<{ items: Badge[] }>("/api/v1/badges/mine"),
  // Publica y sin sesion: ese es el sentido de una insignia verificable.
  verifyBadge: (code: string) => request<BadgeVerification>(`/api/v1/badges/${code}`),
  revokeBadge: (code: string, reason: string) =>
    request<{ status: string }>(`/api/v1/badges/${code}/revoke`, {
      method: "POST",
      body: JSON.stringify({ reason }),
    }),
};

export interface Course {
  ID: string;
  TeacherID: string;
  Slug: string;
  CurrentPublishedVersionID?: string | null;
  // El borrador editable más reciente, si existe. Un curso recién creado lo
  // tiene desde el principio; uno recién despublicado no tiene ninguno hasta
  // crear un borrador de actualización.
  LatestDraftVersionID?: string | null;
  CreatedAt: string;
  UpdatedAt: string;
}

export interface Version {
  ID: string;
  CourseID: string;
  VersionNumber: number;
  Status: "draft" | "published" | "unpublished" | "archived";
  Title: string;
  Summary: string;
  DescriptionMD: string;
  Category: string;
  Level: string;
  Language: string;
  ApprovalMinScore: number;
  ApprovalRequiredResourcesPct: number;
  PublishedAt?: string | null;
  Modules?: Module[];
}

export interface Module {
  ID: string;
  CourseVersionID: string;
  StableID: string;
  Title: string;
  Position: number;
  Units?: Unit[];
}

export interface Unit {
  ID: string;
  ModuleID: string;
  StableID: string;
  Title: string;
  Position: number;
  Resources?: Resource[];
}

/**
 * NuevoRecurso es el cuerpo que acepta la API al crear un recurso.
 *
 * Va declarado aparte de `Resource` a propósito: la respuesta llega en
 * PascalCase y la petición se declara en snake_case, y un único tipo para las
 * dos formas invita a reenviar lo recibido, que es exactamente el error que
 * dejaba al profesor sin poder añadir recursos.
 */
export interface NuevoRecurso {
  type: string;
  title: string;
  position: number;
  visible: boolean;
  required: boolean;
  downloadable?: boolean;
  text_content_md?: string;
  external_url?: string;
}

export interface Resource {
  ID: string;
  UnitID: string;
  StableID: string;
  Type: string;
  Title: string;
  Position: number;
  Visible: boolean;
  Required: boolean;
  Downloadable: boolean;
  ProcessingStatus: string;
  TextContentMD?: string;
  ExternalURL?: string;
  ObjectKey?: string;
}

export interface Enrollment {
  ID: string;
  StudentID: string;
  CourseID: string;
  CourseVersionID: string;
  Status: "active" | "withdrawn" | "completed" | "approved";
  EnrolledAt: string;
}

export { API_URL };

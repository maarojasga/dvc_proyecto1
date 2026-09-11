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

export interface ResourceContent {
  type: string;
  title: string;
  downloadable: boolean;
  url?: string;
  cdn?: boolean;
  expires_in?: number;
  markdown?: string;
  external_url?: string;
  position_seconds?: number;
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
  addResource: (versionId: string, unitId: string, data: Partial<Resource>) =>
    request<Resource>(`/api/v1/courses/versions/${versionId}/units/${unitId}/resources`, {
      method: "POST",
      body: JSON.stringify(data),
    }),
  deleteResource: (versionId: string, resourceId: string) =>
    request<void>(`/api/v1/courses/versions/${versionId}/resources/${resourceId}`, { method: "DELETE" }),
  requestUploadUrl: (versionId: string, resourceId: string, mimeType: string) =>
    request<{ upload_url: string; object_key: string; method: string }>(
      `/api/v1/courses/versions/${versionId}/resources/${resourceId}/upload-url`,
      { method: "POST", body: JSON.stringify({ mime_type: mimeType }) },
    ),
  confirmUpload: (versionId: string, resourceId: string, checksumSha256: string) =>
    request<{ status: string; mime_type?: string; size_bytes?: number; checksum_sha256?: string }>(
      `/api/v1/courses/versions/${versionId}/resources/${resourceId}/confirm-upload`,
      { method: "POST", body: JSON.stringify({ checksum_sha256: checksumSha256 }) },
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
  completeMultipart: (
    versionId: string,
    resourceId: string,
    uploadId: string,
    parts: { part_number: number; etag: string }[],
    checksumSha256: string,
  ) =>
    request<{ status: string; media_asset_id?: string; size_bytes?: number; checksum_sha256?: string }>(
      `/api/v1/courses/versions/${versionId}/resources/${resourceId}/multipart/complete`,
      {
        method: "POST",
        body: JSON.stringify({ upload_id: uploadId, parts, checksum_sha256: checksumSha256 }),
      },
    ),

  listMultipartParts: (versionId: string, resourceId: string, uploadId: string) =>
    request<{ upload_id: string; parts: { part_number: number; size_bytes: number; etag: string }[] }>(
      `/api/v1/courses/versions/${versionId}/resources/${resourceId}/multipart/parts?upload_id=${encodeURIComponent(uploadId)}`,
    ),

  listCatalog: (params: Record<string, string> = {}) =>
    request<{ items: Version[] }>(`/api/v1/catalog?${new URLSearchParams(params)}`),
  getPublishedCourse: (courseId: string) => request<Version>(`/api/v1/catalog/${courseId}`),

  enroll: (courseId: string) =>
    request<Enrollment>("/api/v1/enrollments", { method: "POST", body: JSON.stringify({ course_id: courseId }) }),
  listMyEnrollments: () => request<{ items: Enrollment[] }>("/api/v1/enrollments/mine"),
  withdraw: (courseId: string) =>
    request<{ status: string }>(`/api/v1/enrollments/${courseId}/withdraw`, { method: "POST" }),
};

export interface Course {
  ID: string;
  TeacherID: string;
  Slug: string;
  CurrentPublishedVersionID?: string | null;
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

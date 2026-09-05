import type { ProblemDetails } from "@/types";

/** Error de API con el cuerpo problem details asociado, cuando la API lo entrega. */
export class ApiError extends Error {
  readonly status: number;
  readonly problem?: ProblemDetails;

  constructor(status: number, mensaje: string, problem?: ProblemDetails) {
    super(mensaje);
    this.name = "ApiError";
    this.status = status;
    this.problem = problem;
  }

  /** Errores por campo, utiles para pintar el detalle de una validacion. */
  get erroresPorCampo(): Record<string, string[]> {
    return this.problem?.errors ?? {};
  }
}

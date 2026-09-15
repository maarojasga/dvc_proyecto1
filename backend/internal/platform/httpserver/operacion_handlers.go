package httpserver

import (
	"net/http"
	"time"

	"github.com/hibiken/asynq"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
)

// maxTareasMuertasListadas acota la respuesta: la dead-letter queue se
// consulta para diagnosticar, no para volcarla entera.
const maxTareasMuertasListadas = 50

func (h *handlers) registerOperacion(mux *http.ServeMux) {
	admin := RequireRole(user.RoleAdmin)

	mux.Handle("GET /api/v1/admin/queues", h.auth()(admin(http.HandlerFunc(h.listQueues))))
	mux.Handle("POST /api/v1/admin/queues/{queue}/dead/{taskId}/retry",
		h.auth()(admin(http.HandlerFunc(h.retryDeadTask))))
}

type queueInfoResponse struct {
	Queue     string `json:"queue"`
	Pending   int    `json:"pending"`
	Active    int    `json:"active"`
	Retry     int    `json:"retry"`
	Archived  int    `json:"archived"`
	Completed int    `json:"completed"`
	Failed    int    `json:"failed"`
}

type deadTaskResponse struct {
	ID           string `json:"id"`
	Queue        string `json:"queue"`
	Type         string `json:"type"`
	LastError    string `json:"last_error,omitempty"`
	LastFailedAt string `json:"last_failed_at,omitempty"`
	Retried      int    `json:"retried"`
}

// listQueues expone el estado de las colas y las tareas archivadas.
//
// Es la evidencia de que el backoff y la dead-letter queue operan: "archived"
// cuenta los trabajos que agotaron sus reintentos, los mismos que dispararon
// la alerta del worker.
func (h *handlers) listQueues(w http.ResponseWriter, r *http.Request) {
	if h.deps.Inspector == nil {
		writeError(w, ErrColaNoDisponible)
		return
	}

	colas, err := h.deps.Inspector.Queues()
	if err != nil {
		writeError(w, err)
		return
	}

	infos := make([]queueInfoResponse, 0, len(colas))
	muertas := make([]deadTaskResponse, 0)
	for _, cola := range colas {
		info, err := h.deps.Inspector.GetQueueInfo(cola)
		if err != nil {
			writeError(w, err)
			return
		}
		infos = append(infos, queueInfoResponse{
			Queue: info.Queue, Pending: info.Pending, Active: info.Active,
			Retry: info.Retry, Archived: info.Archived,
			Completed: info.Completed, Failed: info.Failed,
		})

		if len(muertas) >= maxTareasMuertasListadas {
			continue
		}
		tareas, err := h.deps.Inspector.ListArchivedTasks(cola, asynq.PageSize(maxTareasMuertasListadas))
		if err != nil {
			writeError(w, err)
			return
		}
		for _, t := range tareas {
			d := deadTaskResponse{ID: t.ID, Queue: t.Queue, Type: t.Type, LastError: t.LastErr, Retried: t.Retried}
			if !t.LastFailedAt.IsZero() {
				d.LastFailedAt = t.LastFailedAt.UTC().Format(time.RFC3339)
			}
			muertas = append(muertas, d)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"queues": infos, "dead_letter": muertas})
}

// retryDeadTask reencola una tarea archivada conservando su identificador.
//
// Conservarlo importa: es la clave de idempotencia del trabajo, así que el
// reintento no puede producir una segunda salida de algo que ya se completó.
func (h *handlers) retryDeadTask(w http.ResponseWriter, r *http.Request) {
	if h.deps.Inspector == nil {
		writeError(w, ErrColaNoDisponible)
		return
	}
	cola := r.PathValue("queue")
	id := r.PathValue("taskId")
	if cola == "" || id == "" {
		writeError(w, ErrBadRequest)
		return
	}
	if err := h.deps.Inspector.RunTask(cola, id); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "requeued", "task_id": id, "queue": cola})
}

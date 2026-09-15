package httpserver

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/app/quizzes"
	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/domain/user"
)

func (h *handlers) registerQuizzes(mux *http.ServeMux) {
	student := RequireRole(user.RoleStudent)
	teacher := RequireRole(user.RoleTeacher, user.RoleAdmin)

	mux.Handle("PUT /api/v1/courses/versions/{versionId}/resources/{resourceId}/quiz",
		h.auth()(teacher(http.HandlerFunc(h.defineQuiz))))

	mux.Handle("POST /api/v1/resources/{resourceId}/quiz/attempts",
		h.auth()(student(http.HandlerFunc(h.startQuizAttempt))))
	mux.Handle("GET /api/v1/quiz/attempts/{attemptId}",
		h.auth()(student(http.HandlerFunc(h.getQuizAttempt))))
	mux.Handle("PUT /api/v1/quiz/attempts/{attemptId}/answers",
		h.auth()(student(http.HandlerFunc(h.saveQuizAnswer))))
	mux.Handle("POST /api/v1/quiz/attempts/{attemptId}/submit",
		h.auth()(student(http.HandlerFunc(h.submitQuizAttempt))))
}

type defineQuizRequest struct {
	Title            string  `json:"title"`
	TimeLimitSeconds *int    `json:"time_limit_seconds"`
	MaxAttempts      *int    `json:"max_attempts"`
	PassScore        float64 `json:"pass_score"`
	FeedbackPolicy   string  `json:"feedback_policy"`
	ShuffleQuestions bool    `json:"shuffle_questions"`
	Questions        []struct {
		PromptMD string  `json:"prompt_md"`
		Type     string  `json:"type"`
		Points   float64 `json:"points"`
		Options  []struct {
			TextMD    string `json:"text_md"`
			IsCorrect bool   `json:"is_correct"`
		} `json:"options"`
	} `json:"questions"`
}

func (h *handlers) defineQuiz(w http.ResponseWriter, r *http.Request) {
	versionID, err := uuid.Parse(r.PathValue("versionId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	resourceID, err := uuid.Parse(r.PathValue("resourceId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	var req defineQuizRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	def := quizzes.DefinicionQuiz{
		Title: req.Title, TimeLimitSeconds: req.TimeLimitSeconds,
		MaxAttempts: req.MaxAttempts, PassScore: req.PassScore,
		FeedbackPolicy: req.FeedbackPolicy, ShuffleQuestions: req.ShuffleQuestions,
	}
	for _, p := range req.Questions {
		question := quizzes.DefinicionPregunta{PromptMD: p.PromptMD, Type: p.Type, Points: p.Points}
		for _, o := range p.Options {
			question.Options = append(question.Options, quizzes.DefinicionOpcion{
				TextMD: o.TextMD, IsCorrect: o.IsCorrect,
			})
		}
		def.Questions = append(def.Questions, question)
	}

	actor, _ := UserFromContext(r.Context())
	q, err := h.deps.Quizzes.Definir(r.Context(), actor, versionID, resourceID, def)
	if err != nil {
		writeError(w, err)
		return
	}
	// Se responde solo con el identificador y el recuento: devolver el quiz
	// completo incluiria la clave correcta, que no debe salir del servidor.
	writeJSON(w, http.StatusOK, map[string]any{
		"quiz_id": q.ID, "questions": len(q.Questions),
	})
}

func (h *handlers) startQuizAttempt(w http.ResponseWriter, r *http.Request) {
	resourceID, err := uuid.Parse(r.PathValue("resourceId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	actor, _ := UserFromContext(r.Context())
	v, err := h.deps.Quizzes.Iniciar(r.Context(), actor, resourceID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *handlers) getQuizAttempt(w http.ResponseWriter, r *http.Request) {
	attemptID, err := uuid.Parse(r.PathValue("attemptId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	actor, _ := UserFromContext(r.Context())
	v, err := h.deps.Quizzes.Intento(r.Context(), actor, attemptID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

type saveAnswerRequest struct {
	QuestionStableID string   `json:"question_stable_id"`
	SelectedOptions  []string `json:"selected_option_stable_ids"`
}

func (h *handlers) saveQuizAnswer(w http.ResponseWriter, r *http.Request) {
	attemptID, err := uuid.Parse(r.PathValue("attemptId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	var req saveAnswerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	questionID, err := uuid.Parse(req.QuestionStableID)
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	selection := make([]uuid.UUID, 0, len(req.SelectedOptions))
	for _, s := range req.SelectedOptions {
		id, err := uuid.Parse(s)
		if err != nil {
			writeError(w, ErrBadRequest)
			return
		}
		selection = append(selection, id)
	}

	actor, _ := UserFromContext(r.Context())
	if err := h.deps.Quizzes.Guardar(r.Context(), actor, attemptID, questionID, selection); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (h *handlers) submitQuizAttempt(w http.ResponseWriter, r *http.Request) {
	attemptID, err := uuid.Parse(r.PathValue("attemptId"))
	if err != nil {
		writeError(w, ErrBadRequest)
		return
	}
	actor, _ := UserFromContext(r.Context())
	// La cabecera estandar del proyecto hace repetible el envio definitivo:
	// reintentar tras un corte de red devuelve la misma nota.
	key := r.Header.Get("Idempotency-Key")
	v, err := h.deps.Quizzes.Enviar(r.Context(), actor, attemptID, key)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

package httpserver

import (
	"encoding/json"

	"github.com/hibiken/asynq"
)

// queueTask serializa el payload a JSON y construye la tarea asynq
// correspondiente al tipo de trabajo indicado.
func queueTask(taskType string, payload any) (*asynq.Task, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(taskType, b), nil
}

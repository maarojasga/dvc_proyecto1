package queue

import "github.com/google/uuid"

// MediaProcessPayload es el cuerpo del trabajo de transcodificación. TaskID
// es la clave de idempotencia: el worker debe verificar si ya existe una
// salida para este TaskID antes de procesar, para que una doble entrega no
// genere salidas repetidas.
type MediaProcessPayload struct {
	TaskID         uuid.UUID `json:"task_id"`
	MediaAssetID   uuid.UUID `json:"media_asset_id"`
	ResourceID     uuid.UUID `json:"resource_id"`
	SourceObjectKey string   `json:"source_object_key"`
	Kind           string    `json:"kind"` // "video" | "audio"
}

// BadgeIssuePayload dispara la emisión idempotente de una insignia cuando
// una inscripción alcanza el estado approved.
type BadgeIssuePayload struct {
	EnrollmentID uuid.UUID `json:"enrollment_id"`
}

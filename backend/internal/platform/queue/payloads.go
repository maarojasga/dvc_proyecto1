package queue

import "github.com/google/uuid"

// MediaProcessPayload es el cuerpo del trabajo de transcodificación. TaskID
// es la clave de idempotencia: el worker debe verificar si ya existe una
// salida para este TaskID antes de procesar, para que una doble entrega no
// genere salidas repetidas.
type MediaProcessPayload struct {
	TaskID          uuid.UUID `json:"task_id"`
	MediaAssetID    uuid.UUID `json:"media_asset_id"`
	ResourceID      uuid.UUID `json:"resource_id"`
	SourceObjectKey string    `json:"source_object_key"`
	Kind            string    `json:"kind"` // "video" | "audio"
}

// DocumentConvertPayload es el cuerpo del trabajo de conversion de una
// presentacion a PDF. Formato viaja en el payload porque lo dedujo la API al
// verificar la carga, leyendo los bytes; repetir esa deteccion en el worker
// seria trabajo doble y una ocasion de que las dos discrepen.
type DocumentConvertPayload struct {
	TaskID          uuid.UUID `json:"task_id"`
	MediaAssetID    uuid.UUID `json:"media_asset_id"`
	ResourceID      uuid.UUID `json:"resource_id"`
	SourceObjectKey string    `json:"source_object_key"`
	Formato         string    `json:"formato"` // "pptx" | "odp"
}

// BadgeIssuePayload dispara la emisión idempotente de una insignia cuando
// una inscripción alcanza el estado approved.
type BadgeIssuePayload struct {
	EnrollmentID uuid.UUID `json:"enrollment_id"`
}

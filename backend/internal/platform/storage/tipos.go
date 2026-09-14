package storage

// Los tipos de este archivo son el vocabulario con el que el resto de la
// aplicación habla del almacenamiento de objetos.
//
// Están aquí, y no en la firma de los métodos del SDK, porque la restricción
// técnica del proyecto pide que el dominio quede desacoplado del proveedor
// cloud: antes `minio.CompletePart` y `minio.ListObjectPartsResult` viajaban
// hasta los manejadores HTTP, de modo que cambiar de proveedor —o sustituir el
// almacén por un doble en una prueba— obligaba a tocar la capa de entrega.

// ParteCargada es una parte de una carga multipart ya recibida por el almacén.
//
// El ETag lo devuelve el almacén al subir cada parte y el cliente lo reenvía
// al completar: es lo que permite ensamblar el objeto y, de paso, detectar una
// parte corrupta.
type ParteCargada struct {
	Numero int    `json:"part_number"`
	ETag   string `json:"etag"`
	Tamano int64  `json:"size_bytes,omitempty"`
}

// ObjetoInfo son los metadatos de un objeto almacenado.
//
// ContentType es el que declaró quien subió el archivo, así que no acredita
// nada por sí solo: el tipo real se deduce de los primeros bytes al verificar
// la carga.
type ObjetoInfo struct {
	Tamano      int64
	ContentType string
}

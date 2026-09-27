# Cloud Storage: tres buckets regionales en la región de las VM.
#
# - objetos: originales, PDFs de presentaciones, subtítulos e insignias.
#   Privado, con prevención de acceso público forzada: todo se lee con URL
#   firmada por la API después de autorizar.
# - hls: solo los derivados hls/*. Legible por cualquiera, porque el
#   reproductor pide variantes y segmentos con rutas relativas que no
#   heredan la firma del manifiesto. Ver README, "HLS sin firma".
# - respaldos: exports de Cloud SQL. Sobrevive a la eliminación de la base.

locals {
  origenes_cors = concat([local.url_publica], var.origenes_cors_extra)
}

resource "google_storage_bucket" "objetos" {
  name                        = local.bucket_objetos
  location                    = var.region
  storage_class               = "STANDARD"
  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"
  force_destroy               = var.permitir_borrar_buckets

  # Sin eliminación temporal: por defecto GCS retiene 7 días lo borrado y lo
  # cobra, y aquí cada carga rechazada o reemplazada se borra a propósito.
  soft_delete_policy {
    retention_duration_seconds = 0
  }

  # Carga directa desde el navegador: PUT simple y partes de la multipart.
  # Sin ETag en response_header el navegador no puede leerlo y la multipart
  # no se puede completar (CompleteMultipartUpload lleva el ETag de cada
  # parte).
  cors {
    origin          = local.origenes_cors
    method          = ["GET", "HEAD", "PUT"]
    response_header = ["ETag", "Content-Type", "Content-Length", "Content-Range"]
    max_age_seconds = 3600
  }

  # Una multipart abandonada (pestaña cerrada) deja partes que se cobran como
  # almacenamiento y no aparecen al listar objetos.
  lifecycle_rule {
    action {
      type = "AbortIncompleteMultipartUpload"
    }
    condition {
      age = 1
    }
  }
}

resource "google_storage_bucket" "hls" {
  name                        = local.bucket_hls
  location                    = var.region
  storage_class               = "STANDARD"
  uniform_bucket_level_access = true
  # "inherited" y no "enforced": este es el único bucket que se abre. Si una
  # política de organización fuerza la prevención de acceso público
  # (constraints/storage.publicAccessPrevention), el binding de allUsers de
  # abajo falla al aplicar; ver README.
  public_access_prevention = "inherited"
  force_destroy            = var.permitir_borrar_buckets

  soft_delete_policy {
    retention_duration_seconds = 0
  }

  cors {
    origin          = local.origenes_cors
    method          = ["GET", "HEAD"]
    response_header = ["Content-Type", "Content-Length", "Content-Range", "ETag"]
    max_age_seconds = 3600
  }
}

# legacyObjectReader y no objectViewer: da storage.objects.get y nada más.
# objectViewer incluye storage.objects.list, y con allUsers cualquiera podría
# enumerar los identificadores de todos los recursos publicados.
resource "google_storage_bucket_iam_member" "hls_publico" {
  bucket = google_storage_bucket.hls.name
  role   = "roles/storage.legacyObjectReader"
  member = "allUsers"
}

resource "google_storage_bucket" "respaldos" {
  name                        = local.bucket_respaldos
  location                    = var.region
  storage_class               = "STANDARD"
  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"
  force_destroy               = var.permitir_borrar_buckets

  lifecycle_rule {
    action {
      type = "Delete"
    }
    condition {
      age = 90
    }
  }
}

# --- Permisos por componente ---------------------------------------------------

# API: lee y escribe en objetos. objectUser da storage.objects.* (salvo
# cambiar su IAM) y storage.multipartUploads.* (iniciar, listar partes,
# abortar), que la carga multipart por la API XML necesita. objectAdmin
# añadiría administrar el acceso a los objetos, que la API no hace. Borra
# porque la verificación posterior a la carga elimina lo rechazado.
resource "google_storage_bucket_iam_member" "web_objetos" {
  bucket = google_storage_bucket.objetos.name
  role   = "roles/storage.objectUser"
  member = data.google_service_account.web.member
}

# La API firma la URL de la lista maestra HLS; con la firma la petición llega
# autenticada como su cuenta de servicio. allUsers ya la cubre, pero se deja
# explícito para que no dependa de que el bucket siga siendo público.
resource "google_storage_bucket_iam_member" "web_hls" {
  bucket = google_storage_bucket.hls.name
  role   = "roles/storage.objectViewer"
  member = data.google_service_account.web.member
}

# Worker: lee cualquier objeto (los originales que procesa)...
resource "google_storage_bucket_iam_member" "worker_objetos_lectura" {
  bucket = google_storage_bucket.objetos.name
  role   = "roles/storage.objectViewer"
  member = data.google_service_account.worker.member
}

# ...pero solo escribe bajo presentaciones/, el PDF que genera LibreOffice.
# objectUser y no objectCreator: un reintento de asynq reescribe la misma
# clave, y sobrescribir exige storage.objects.delete. No puede tocar
# resources/ (los originales) ni badges/.
resource "google_storage_bucket_iam_member" "worker_objetos_escritura" {
  bucket = google_storage_bucket.objetos.name
  role   = "roles/storage.objectUser"
  member = data.google_service_account.worker.member

  condition {
    title       = "solo-presentaciones"
    description = "El worker solo escribe los PDFs de vista previa."
    expression  = "resource.name.startsWith(\"projects/_/buckets/${google_storage_bucket.objetos.name}/objects/presentaciones/\")"
  }
}

# Worker: dueño de los derivados HLS.
resource "google_storage_bucket_iam_member" "worker_hls" {
  bucket = google_storage_bucket.hls.name
  role   = "roles/storage.objectUser"
  member = data.google_service_account.worker.member
}

# El agente de servicio de Cloud SQL escribe los exports y lee lo que se
# importa. Sin esto `gcloud sql export sql` falla con 403.
# Crear (export) y leer (import) bastan; objectAdmin le daría además borrar
# respaldos y cambiar su acceso, que no necesita.
resource "google_storage_bucket_iam_member" "sql_respaldos" {
  for_each = var.crear_bd ? toset(["roles/storage.objectCreator", "roles/storage.objectViewer"]) : toset([])
  bucket   = google_storage_bucket.respaldos.name
  role     = each.value
  member   = "serviceAccount:${google_sql_database_instance.bd[0].service_account_email_address}"
}

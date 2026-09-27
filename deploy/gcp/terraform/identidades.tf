# Identidades, secretos y registro de imágenes.
#
# Una cuenta de servicio por componente: la VM corre con ella, las reglas de
# firewall la usan como origen/destino y su clave HMAC es con la que la API o
# el worker firman contra Cloud Storage. Así los permisos sobre los buckets
# son distintos por componente, como pide el enunciado.

# Las dos cuentas las crea deploy/gcp/bootstrap-ci.sh, no Terraform. Crear
# cuentas de servicio exige iam.serviceAccountAdmin de proyecto, que también
# da setIamPolicy sobre TODAS las cuentas del proyecto, incluida la de CI: con
# él, la cuenta de despliegue podría darse tokenCreator sobre cualquiera, o
# dárselo a un tercero y tener acceso fuera de GitHub. Creadas aparte, la
# cuenta de CI solo necesita actAs sobre estas dos (serviceAccountUser a
# nivel de cuenta, también del bootstrap).
data "google_service_account" "web" {
  account_id = "${var.prefijo}-web"
}

data "google_service_account" "worker" {
  account_id = "${var.prefijo}-worker"
}

# Estados anteriores a este cambio tienen las cuentas como recursos. Se
# olvidan sin borrarlas: siguen existiendo y ahora se leen como data.
removed {
  from = google_service_account.web
  lifecycle {
    destroy = false
  }
}

removed {
  from = google_service_account.worker
  lifecycle {
    destroy = false
  }
}

locals {
  cuentas = {
    web    = data.google_service_account.web
    worker = data.google_service_account.worker
  }
}

# Escribir registros y métricas (Ops Agent). Nada más a nivel de proyecto.
resource "google_project_iam_member" "registros" {
  for_each = local.cuentas
  project  = var.project_id
  role     = "roles/logging.logWriter"
  member   = each.value.member
}

resource "google_project_iam_member" "metricas" {
  for_each = local.cuentas
  project  = var.project_id
  role     = "roles/monitoring.metricWriter"
  member   = each.value.member
}

# --- Claves HMAC -------------------------------------------------------------
#
# La API XML de Cloud Storage acepta firmas SigV4 con claves HMAC, que es lo
# que habla minio-go sin tocar el código de firma, las URLs prefirmadas ni el
# multipart. El access_id no es secreto (identifica la clave, como un
# usuario); el secret va a Secret Manager y a ningún archivo.

resource "google_storage_hmac_key" "web" {
  service_account_email = data.google_service_account.web.email
}

resource "google_storage_hmac_key" "worker" {
  service_account_email = data.google_service_account.worker.email
}

# --- Secret Manager ------------------------------------------------------------

resource "random_password" "bd" {
  length = 32
  # Sin especiales: va dentro de DATABASE_URL y un @, / o : la rompería.
  special = false
}

resource "random_password" "admin" {
  length  = 24
  special = false
}

locals {
  # Secretos con valor generado aquí y qué VM puede leer cada uno. La clave
  # de insignias va aparte (abajo).
  #
  # Los valores van en un mapa distinto porque son sensibles, y un for_each no
  # puede recorrer nada derivado de un valor sensible.
  secretos = {
    bd-password        = ["web", "worker"]
    hmac-web-secret    = ["web"]
    hmac-worker-secret = ["worker"]
    admin-password     = ["web"]
  }
  valores_secretos = {
    bd-password        = random_password.bd.result
    hmac-web-secret    = google_storage_hmac_key.web.secret
    hmac-worker-secret = google_storage_hmac_key.worker.secret
    admin-password     = random_password.admin.result
  }
  lectores_de_secretos = merge(
    [for nombre, lectores in local.secretos : { for l in lectores : "${nombre}/${l}" => { secreto = nombre, lector = l } }]...
  )
}

resource "google_secret_manager_secret" "s" {
  for_each  = local.secretos
  secret_id = "${var.prefijo}-${each.key}"
  replication {
    user_managed {
      replicas {
        location = var.region
      }
    }
  }
}

resource "google_secret_manager_secret_version" "s" {
  for_each    = local.secretos
  secret      = google_secret_manager_secret.s[each.key].id
  secret_data = local.valores_secretos[each.key]
}

# Cada VM lee solo sus secretos: el Worker no puede leer la clave HMAC de la
# API ni la contraseña del administrador.
resource "google_secret_manager_secret_iam_member" "lector" {
  for_each  = local.lectores_de_secretos
  secret_id = google_secret_manager_secret.s[each.value.secreto].id
  role      = "roles/secretmanager.secretAccessor"
  member    = local.cuentas[each.value.lector].member
}

# La clave de firma de insignias vive fuera de Terraform (la crean
# bootstrap-ci.sh o desplegar-infra.sh), para que sobreviva a un destroy: con
# otra clave, las credenciales Open Badges ya emitidas dejarían de
# verificarse. Aquí solo se da acceso a la API. El ID se compone en lugar de
# leerse con un data source para que el plan no dependa de que ya exista: en
# CI el plan no crea nada, y la clave se asegura justo antes del apply.
locals {
  secreto_badge = "${var.prefijo}-badge-signing-key"
}

resource "google_secret_manager_secret_iam_member" "badge_web" {
  secret_id = "projects/${var.project_id}/secrets/${local.secreto_badge}"
  role      = "roles/secretmanager.secretAccessor"
  member    = data.google_service_account.web.member
}

# --- Artifact Registry ---------------------------------------------------------
#
# Las imágenes se construyen fuera de las e2-small (publicar.sh) y las VM
# solo hacen pull con su cuenta de servicio. El Worker llega por Private
# Google Access, sin NAT.

resource "google_artifact_registry_repository" "imagenes" {
  location      = var.region
  repository_id = var.prefijo
  format        = "DOCKER"
  description   = "Imágenes de la plataforma MOOC (api, migrate, worker, frontend y réplica de redis)."

  # Cada publicación deja ~1 GB (el worker lleva LibreOffice y FFmpeg). Se
  # conservan las 5 más recientes por imagen y el resto se borra a las dos
  # semanas: por encima de 0,5 GB el registro se cobra por GB-mes.
  cleanup_policy_dry_run = false
  cleanup_policies {
    id     = "conservar-recientes"
    action = "KEEP"
    most_recent_versions {
      keep_count = 5
    }
  }
  cleanup_policies {
    id     = "borrar-antiguas"
    action = "DELETE"
    condition {
      older_than = "1209600s"
    }
  }
}

resource "google_artifact_registry_repository_iam_member" "lector" {
  for_each   = local.cuentas
  location   = google_artifact_registry_repository.imagenes.location
  repository = google_artifact_registry_repository.imagenes.name
  role       = "roles/artifactregistry.reader"
  member     = each.value.member
}

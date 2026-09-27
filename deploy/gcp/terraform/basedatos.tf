# Cloud SQL for PostgreSQL 16, solo por IP privada.

# --- Private Service Access ------------------------------------------------------
#
# Cloud SQL con IP privada vive en una VPC de Google enlazada a la nuestra por
# peering. Este rango es el que Google usa para asignarle la IP.

resource "google_compute_global_address" "psa" {
  name          = "${var.prefijo}-psa"
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  address       = var.psa_direccion
  prefix_length = var.psa_prefijo
  network       = google_compute_network.vpc.id
}

resource "google_service_networking_connection" "psa" {
  network                 = google_compute_network.vpc.id
  service                 = "servicenetworking.googleapis.com"
  reserved_peering_ranges = [google_compute_global_address.psa.name]
  # Al borrar la instancia, Google tarda en liberar el peering y el destroy
  # fallaría con "producer services still using this connection". Se
  # abandona y desaparece con la VPC.
  deletion_policy = "ABANDON"
}

# El nombre de una instancia de Cloud SQL queda reservado hasta una semana
# después de borrarla. El sufijo cambia en cada creación (se destruye con
# crear_bd=false) para que recrear no choque con el nombre anterior.
resource "random_id" "bd" {
  count       = var.crear_bd ? 1 : 0
  byte_length = 2
}

resource "google_sql_database_instance" "bd" {
  count            = var.crear_bd ? 1 : 0
  name             = "${var.prefijo}-bd-${random_id.bd[0].hex}"
  region           = var.region
  database_version = "POSTGRES_16"

  # Protección de Terraform: un plan que borre la instancia falla. No es la
  # de la API (settings.deletion_protection_enabled), que frena también la
  # consola y gcloud. bd.sh eliminar baja esta antes de borrar.
  deletion_protection = var.bd_proteccion_terraform

  settings {
    # ENTERPRISE explícito: para PostgreSQL 16 la API puede elegir Enterprise
    # Plus, que no admite tiers pequeños ni custom de 1 vCPU y cuesta más.
    edition = "ENTERPRISE"
    tier    = var.bd_tier
    # Una zona, sin réplica en espera ni de lectura: lo que pide el enunciado.
    availability_type = "ZONAL"

    disk_type             = "PD_SSD"
    disk_size             = var.bd_disco_gb
    disk_autoresize       = true
    disk_autoresize_limit = var.bd_disco_max_gb

    deletion_protection_enabled = var.bd_proteccion_api

    location_preference {
      zone = var.zona
    }

    ip_configuration {
      ipv4_enabled    = false
      private_network = google_compute_network.vpc.id
      # Rechaza conexiones sin TLS. No exige certificado de cliente: la API y
      # el worker conectan con sslmode=require.
      ssl_mode = "ENCRYPTED_ONLY"
    }

    backup_configuration {
      enabled    = true
      start_time = "08:00" # UTC, 03:00 en Colombia
      # Sin PITR: guarda WAL aparte y se cobra; para esta entrega basta el
      # backup diario y el export de bd.sh.
      point_in_time_recovery_enabled = false
      backup_retention_settings {
        retained_backups = var.bd_retencion_backups
        retention_unit   = "COUNT"
      }
    }

    maintenance_window {
      day          = 7
      hour         = 8
      update_track = "stable"
    }

    # Query Insights: latencia y carga por consulta sin instalar nada. Es lo
    # que el análisis de capacidad necesita para decir qué consulta satura.
    insights_config {
      query_insights_enabled  = true
      query_string_length     = 1024
      record_application_tags = false
      record_client_address   = false
    }

    user_labels = {
      proyecto = var.prefijo
      entrega  = "2"
    }
  }

  depends_on = [google_service_networking_connection.psa]
}

# ABANDON en la base y el usuario: se van con la instancia. Borrarlos antes
# por separado falla en PostgreSQL (el rol es dueño de las tablas de las
# migraciones, y la base tiene conexiones abiertas de la API y el worker).
resource "google_sql_database" "mooc" {
  count           = var.crear_bd ? 1 : 0
  name            = "mooc"
  instance        = google_sql_database_instance.bd[0].name
  deletion_policy = "ABANDON"
}

resource "google_sql_user" "mooc" {
  count           = var.crear_bd ? 1 : 0
  name            = "mooc"
  instance        = google_sql_database_instance.bd[0].name
  password        = random_password.bd.result
  deletion_policy = "ABANDON"
}

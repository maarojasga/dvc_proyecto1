# Infraestructura de la Entrega 2 en GCP: dos VM (Web y Worker) en una VPC
# propia, Cloud SQL for PostgreSQL por IP privada, Cloud Storage por su API
# XML compatible con S3 y Artifact Registry para las imágenes.
#
# Este archivo tiene lo común y la red; el resto está repartido por tema:
# computo.tf, basedatos.tf, almacenamiento.tf, identidades.tf, costos.tf.

data "google_project" "actual" {}

locals {
  host_web    = var.dominio_web != "" ? var.dominio_web : "${google_compute_address.web.address}.sslip.io"
  url_publica = "https://${local.host_web}"

  # Nombres de bucket sin puntos: minio-go firma en estilo virtual-host
  # (<bucket>.storage.googleapis.com) y el certificado comodín de Google no
  # cubre un nombre con puntos.
  bucket_objetos   = "${var.project_id}-${var.prefijo}-objetos"
  bucket_hls       = "${var.project_id}-${var.prefijo}-hls"
  bucket_respaldos = "${var.project_id}-${var.prefijo}-respaldos"

  registro = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.imagenes.repository_id}"

  rango_psa = "${var.psa_direccion}/${var.psa_prefijo}"
}

# --- VPC ---------------------------------------------------------------------
#
# Modo custom: la red default trae subredes en todas las regiones y reglas
# que abren SSH, RDP e ICMP a Internet. Aquí solo existe lo que se declara.

resource "google_compute_network" "vpc" {
  name                    = "${var.prefijo}-vpc"
  auto_create_subnetworks = false
  routing_mode            = "REGIONAL"
}

# La ruta por defecto a Internet (0.0.0.0/0 -> default-internet-gateway) que
# crea GCP se conserva a propósito: la usan la IP pública del Web, Cloud NAT
# y también Private Google Access, que con el dominio por defecto
# (storage.googleapis.com, el único que minio-go reconoce como GCS) sale por
# esa ruta aunque el tráfico no deje la red de Google.

resource "google_compute_subnetwork" "web" {
  name          = "${var.prefijo}-web"
  region        = var.region
  network       = google_compute_network.vpc.id
  ip_cidr_range = var.cidr_web
  # El Web tiene IP externa, así que Private Google Access no le aplica; se
  # deja apagado para que la subred diga lo que hace.
  private_ip_google_access = false
}

resource "google_compute_subnetwork" "worker" {
  name          = "${var.prefijo}-worker"
  region        = var.region
  network       = google_compute_network.vpc.id
  ip_cidr_range = var.cidr_worker
  # El Worker no tiene IP externa. Con esto alcanza Cloud Storage, Artifact
  # Registry, Secret Manager y Logging/Monitoring sin NAT y sin salir a
  # Internet: es el tráfico pesado (originales, segmentos HLS, imágenes).
  private_ip_google_access = true
}

# --- Cloud NAT (solo subred del Worker) ----------------------------------------
#
# El Worker necesita salir a Internet para lo que no es de Google: el
# repositorio de Docker y el del Ops Agent en el primer arranque, y los
# paquetes de Debian. Con habilitar_nat=false se elimina: las imágenes
# (incluido Redis, que publicar.sh replica) salen de Artifact Registry por
# Private Google Access.

resource "google_compute_router" "router" {
  count   = var.habilitar_nat ? 1 : 0
  name    = "${var.prefijo}-router"
  region  = var.region
  network = google_compute_network.vpc.id
}

resource "google_compute_router_nat" "nat" {
  count                              = var.habilitar_nat ? 1 : 0
  name                               = "${var.prefijo}-nat"
  router                             = google_compute_router.router[0].name
  region                             = var.region
  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "LIST_OF_SUBNETWORKS"

  subnetwork {
    name                    = google_compute_subnetwork.worker.id
    source_ip_ranges_to_nat = ["ALL_IP_RANGES"]
  }

  log_config {
    enable = true
    filter = "ERRORS_ONLY"
  }
}

# --- Direcciones ---------------------------------------------------------------

# IP externa estática del Web: el nombre sslip.io, el certificado, el CORS de
# los buckets y NEXT_PUBLIC_API_URL dependen de ella, y una efímera cambia al
# detener la VM.
resource "google_compute_address" "web" {
  name         = "${var.prefijo}-web-ip"
  region       = var.region
  address_type = "EXTERNAL"
  network_tier = "PREMIUM"
}

resource "google_compute_address" "worker_interna" {
  name         = "${var.prefijo}-worker-ip"
  region       = var.region
  address_type = "INTERNAL"
  subnetwork   = google_compute_subnetwork.worker.id
  address      = var.ip_worker
}

# --- Firewall ----------------------------------------------------------------
#
# Por cuenta de servicio y no por etiqueta: cambiar las etiquetas de una VM
# solo exige compute.instances.setTags, mientras que cambiar su cuenta de
# servicio exige detenerla y el permiso iam.serviceAccountUser sobre la
# nueva. La regla sigue a la identidad de la máquina, no a una etiqueta.
#
# Todo lo que no está aquí queda cerrado por la regla implícita de denegar la
# entrada. La salida está abierta salvo hacia Cloud SQL (abajo).

resource "google_compute_firewall" "web_publico" {
  name                    = "${var.prefijo}-permitir-web-https"
  network                 = google_compute_network.vpc.id
  direction               = "INGRESS"
  priority                = 1000
  source_ranges           = ["0.0.0.0/0"]
  target_service_accounts = [data.google_service_account.web.email]

  # 80 solo para el reto HTTP-01 de Let's Encrypt y la redirección a 443.
  allow {
    protocol = "tcp"
    ports    = ["80", "443"]
  }
}

resource "google_compute_firewall" "redis" {
  name                    = "${var.prefijo}-permitir-redis-desde-web"
  network                 = google_compute_network.vpc.id
  direction               = "INGRESS"
  priority                = 1000
  source_service_accounts = [data.google_service_account.web.email]
  target_service_accounts = [data.google_service_account.worker.email]

  # Redis no tiene contraseña (la API y el worker no la soportan): lo
  # protegen esta regla y que el contenedor solo escuche en la IP interna.
  allow {
    protocol = "tcp"
    ports    = ["6379"]
  }
}

resource "google_compute_firewall" "ssh_iap" {
  name      = "${var.prefijo}-permitir-ssh-iap"
  network   = google_compute_network.vpc.id
  direction = "INGRESS"
  priority  = 1000
  # Rango fijo de IAP TCP forwarding. La administración es
  # `gcloud compute ssh --tunnel-through-iap`: el puerto 22 no está abierto a
  # Internet y quién entra lo decide IAM (roles/iap.tunnelResourceAccessor y
  # OS Login), no una llave repartida.
  source_ranges           = ["35.235.240.0/20"]
  target_service_accounts = [data.google_service_account.web.email, data.google_service_account.worker.email]

  allow {
    protocol = "tcp"
    ports    = ["22"]
  }
}

# Cloud SQL vive en la red del productor (peering de servicenetworking) y los
# firewalls de esta VPC no filtran su entrada. Lo que sí se controla es la
# salida desde aquí: solo las cuentas de servicio del Web y del Worker llegan
# al rango de Private Service Access, y solo al 5432. Cualquier otra VM que
# se cree en la VPC queda fuera.
resource "google_compute_firewall" "sql_permitir" {
  name                    = "${var.prefijo}-permitir-egress-sql"
  network                 = google_compute_network.vpc.id
  direction               = "EGRESS"
  priority                = 900
  destination_ranges      = [local.rango_psa]
  target_service_accounts = [data.google_service_account.web.email, data.google_service_account.worker.email]

  allow {
    protocol = "tcp"
    ports    = ["5432"]
  }
}

resource "google_compute_firewall" "sql_denegar" {
  name               = "${var.prefijo}-denegar-egress-sql"
  network            = google_compute_network.vpc.id
  direction          = "EGRESS"
  priority           = 1000
  destination_ranges = [local.rango_psa]

  deny {
    protocol = "all"
  }
}

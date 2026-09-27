variable "project_id" {
  description = "ID del proyecto de GCP (no el nombre ni el número)."
  type        = string
}

variable "region" {
  description = "Región de todo el despliegue. us-central1 es de las más baratas y tiene todos los servicios usados."
  type        = string
  default     = "us-central1"
}

variable "zona" {
  description = "Zona de las VM y de Cloud SQL. La misma para todo: el tráfico entre zonas se cobra y añade latencia."
  type        = string
  default     = "us-central1-a"
}

variable "prefijo" {
  description = "Prefijo de nombres de recursos. Solo minúsculas, dígitos y guiones (va en nombres de buckets)."
  type        = string
  default     = "mooc"

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{1,14}$", var.prefijo))
    error_message = "prefijo: 2-15 caracteres, minúsculas, dígitos o guiones, empezando por letra."
  }
}

# --- Cómputo -----------------------------------------------------------------

variable "tipo_maquina" {
  description = "Tipo de las dos VM. e2-small = 2 vCPU compartidas (0,5 vCPU sostenida con ráfaga) y 2 GB. e2-custom-2-2048 da 2 vCPU dedicadas con los mismos 2 GB (ver README)."
  type        = string
  default     = "e2-small"
}

variable "disco_gb" {
  description = "Disco de arranque de cada VM (pd-balanced)."
  type        = number
  default     = 30
}

variable "imagen_vm" {
  description = "Imagen de las VM. Debian 12: trae google-cloud-cli y admite Docker Engine con el plugin compose desde el repositorio de Docker."
  type        = string
  default     = "debian-cloud/debian-12"
}

variable "instalar_ops_agent" {
  description = "Instala el Ops Agent en las VM. Sin él Cloud Monitoring no ve memoria ni disco, que el análisis de capacidad exige."
  type        = bool
  default     = true
}

# --- Red ---------------------------------------------------------------------

variable "cidr_web" {
  description = "Subred del Web Server."
  type        = string
  default     = "10.10.1.0/24"
}

variable "cidr_worker" {
  description = "Subred privada del Worker Server."
  type        = string
  default     = "10.10.2.0/24"
}

variable "ip_worker" {
  description = "IP interna fija del Worker (Redis). Fija para que REDIS_ADDR del Web no cambie al recrear la VM."
  type        = string
  default     = "10.10.2.10"
}

variable "psa_direccion" {
  description = "Inicio del rango reservado para Private Service Access (Cloud SQL). Explícito para poder escribir las reglas de egress contra él."
  type        = string
  default     = "10.30.0.0"
}

variable "psa_prefijo" {
  description = "Longitud del rango de Private Service Access. /20 es lo que recomienda Google para Cloud SQL."
  type        = number
  default     = 20
}

variable "habilitar_nat" {
  description = "Cloud NAT para la salida del Worker. Hace falta en el primer arranque (Docker, Ops Agent); después el Worker solo habla con APIs de Google por Private Google Access y se puede apagar."
  type        = bool
  default     = true
}

variable "dominio_web" {
  description = "Nombre del Web Server para TLS. Vacío: <IP estática>.sslip.io."
  type        = string
  default     = ""
}

# --- Cloud SQL ---------------------------------------------------------------

variable "crear_bd" {
  description = "Crea la instancia de Cloud SQL. false la elimina (antes: deploy/gcp/bd.sh exportar)."
  type        = bool
  default     = true
}

variable "bd_tier" {
  description = "Tier de Cloud SQL. db-custom-1-3840: 1 vCPU dedicada, 3,75 GB, max_connections 100 por defecto. db-g1-small (vCPU compartida, 50 conexiones) es más barato pero añade ruido a las mediciones."
  type        = string
  default     = "db-custom-1-3840"
}

variable "bd_disco_gb" {
  description = "Almacenamiento SSD inicial de Cloud SQL (crece solo hasta bd_disco_max_gb)."
  type        = number
  default     = 10
}

variable "bd_disco_max_gb" {
  description = "Tope del crecimiento automático del disco de Cloud SQL."
  type        = number
  default     = 30
}

variable "bd_retencion_backups" {
  description = "Backups automáticos diarios que se conservan. Se borran con la instancia: el respaldo que sobrevive es el export de bd.sh."
  type        = number
  default     = 3
}

variable "bd_proteccion_terraform" {
  description = "deletion_protection de Terraform: impide que terraform destroy/apply borre la instancia. bd.sh y destruir.sh lo bajan explícitamente."
  type        = bool
  default     = true
}

variable "bd_proteccion_api" {
  description = "settings.deletion_protection_enabled: protección en la propia API de Cloud SQL (también frena la consola y gcloud)."
  type        = bool
  default     = false
}

# --- Almacenamiento ----------------------------------------------------------

variable "permitir_borrar_buckets" {
  description = "force_destroy de los buckets. false: destroy falla si tienen objetos. destruir.sh lo sube tras respaldar."
  type        = bool
  default     = false
}

variable "origenes_cors_extra" {
  description = "Orígenes adicionales permitidos por CORS en los buckets (p. ej. http://localhost:3000 para probar el frontend en local contra GCS)."
  type        = list(string)
  default     = []
}

# --- Costos ------------------------------------------------------------------

variable "cuenta_facturacion" {
  description = "ID de la cuenta de facturación (XXXXXX-XXXXXX-XXXXXX) para el presupuesto. Vacío: no se crea."
  type        = string
  default     = ""
}

variable "presupuesto_monto" {
  description = "Monto mensual del presupuesto, en la moneda de la cuenta de facturación."
  type        = number
  default     = 50
}

variable "presupuesto_moneda" {
  description = "Moneda del presupuesto. Tiene que coincidir con la de la cuenta de facturación (USD, COP...)."
  type        = string
  default     = "USD"
}

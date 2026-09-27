# Salidas que leen los scripts de deploy/gcp (lib.sh: salida <nombre>).
# Ninguna es secreta: los secretos solo se nombran.

output "proyecto" {
  value = var.project_id
}

output "region" {
  value = var.region
}

output "zona" {
  value = var.zona
}

output "prefijo" {
  value = var.prefijo
}

output "vm_web" {
  value = google_compute_instance.web.name
}

output "vm_worker" {
  value = google_compute_instance.worker.name
}

output "ip_web" {
  value = google_compute_address.web.address
}

output "host_web" {
  value = local.host_web
}

output "url_publica" {
  value = local.url_publica
}

output "ip_worker" {
  description = "IP interna del Worker: REDIS_ADDR del Web y REDIS_BIND_IP del Worker."
  value       = google_compute_address.worker_interna.address
}

output "bd_instancia" {
  value = var.crear_bd ? google_sql_database_instance.bd[0].name : ""
}

output "bd_ip_privada" {
  value = var.crear_bd ? google_sql_database_instance.bd[0].private_ip_address : ""
}

output "bd_nombre" {
  value = "mooc"
}

output "bd_usuario" {
  value = "mooc"
}

output "bucket_objetos" {
  value = google_storage_bucket.objetos.name
}

output "bucket_hls" {
  value = google_storage_bucket.hls.name
}

output "bucket_respaldos" {
  value = google_storage_bucket.respaldos.name
}

output "registro" {
  description = "Prefijo de las imágenes en Artifact Registry."
  value       = local.registro
}

output "hmac_web_access_id" {
  description = "Identificador de la clave HMAC de la API (S3_ACCESS_KEY). No es secreto."
  value       = google_storage_hmac_key.web.access_id
}

output "hmac_worker_access_id" {
  value = google_storage_hmac_key.worker.access_id
}

output "secreto_bd_password" {
  value = google_secret_manager_secret.s["bd-password"].secret_id
}

output "secreto_hmac_web" {
  value = google_secret_manager_secret.s["hmac-web-secret"].secret_id
}

output "secreto_hmac_worker" {
  value = google_secret_manager_secret.s["hmac-worker-secret"].secret_id
}

output "secreto_admin_password" {
  value = google_secret_manager_secret.s["admin-password"].secret_id
}

output "secreto_badge" {
  value = local.secreto_badge
}

output "sa_web" {
  value = data.google_service_account.web.email
}

output "sa_worker" {
  value = data.google_service_account.worker.email
}

output "nat_activa" {
  value = var.habilitar_nat
}

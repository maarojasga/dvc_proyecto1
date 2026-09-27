mock_provider "google" {
  mock_data "google_project" { defaults = { number = "123456789" } }
  mock_data "google_service_account" { defaults = { email = "sa@p.iam.gserviceaccount.com", member = "serviceAccount:sa@p.iam.gserviceaccount.com" } }
  mock_resource "google_compute_network" { defaults = { id = "projects/mi-proyecto/global/networks/mooc-vpc" } }
  mock_resource "google_compute_subnetwork" { defaults = { id = "projects/mi-proyecto/regions/us-central1/subnetworks/s" } }
  mock_resource "google_compute_address" { defaults = { address = "203.0.113.10" } }
  mock_resource "google_sql_database_instance" { defaults = { service_account_email_address = "p123@gcp-sa-cloud-sql.iam.gserviceaccount.com", private_ip_address = "10.30.0.3" } }
}
mock_provider "random" {}

variables {
  project_id = "mi-proyecto"
}

run "completo" {
  command = apply
  assert {
    condition     = output.url_publica == "https://203.0.113.10.sslip.io"
    error_message = "url_publica"
  }
  assert {
    condition     = output.bucket_hls == "mi-proyecto-mooc-hls"
    error_message = "bucket_hls"
  }
  assert {
    condition     = length(google_secret_manager_secret_iam_member.lector) == 5
    error_message = "lectores de secretos: ${length(google_secret_manager_secret_iam_member.lector)}"
  }
  assert {
    condition     = length(google_compute_router_nat.nat) == 1 && length(google_sql_database_instance.bd) == 1
    error_message = "nat/bd"
  }
  assert {
    condition     = google_sql_database_instance.bd[0].settings[0].edition == "ENTERPRISE" && google_sql_database_instance.bd[0].settings[0].ip_configuration[0].ipv4_enabled == false
    error_message = "bd"
  }
  assert {
    condition     = contains(google_storage_bucket.objetos.cors[0].response_header, "ETag")
    error_message = "cors etag"
  }
  # La clave de insignias no se lee con un data source: el plan de CI no
  # debe depender de que ya exista.
  assert {
    condition     = google_secret_manager_secret_iam_member.badge_web.secret_id == "projects/mi-proyecto/secrets/mooc-badge-signing-key" && output.secreto_badge == "mooc-badge-signing-key"
    error_message = "secreto de insignias"
  }
  # Las cuentas de las VM las crea bootstrap-ci.sh; aquí solo se leen.
  assert {
    condition     = google_compute_instance.web.service_account[0].email == data.google_service_account.web.email
    error_message = "cuenta de servicio del Web"
  }
}

run "sin_bd_ni_nat_con_presupuesto" {
  command = apply
  variables {
    crear_bd           = false
    habilitar_nat      = false
    cuenta_facturacion = "000000-000000-000000"
    dominio_web        = "mooc.ejemplo.co"
  }
  assert {
    condition     = output.bd_instancia == "" && length(google_compute_router_nat.nat) == 0 && length(google_storage_bucket_iam_member.sql_respaldos) == 0
    error_message = "toggles"
  }
  assert {
    condition     = output.url_publica == "https://mooc.ejemplo.co" && length(google_billing_budget.mensual) == 1
    error_message = "dominio/presupuesto"
  }
  assert {
    condition     = google_billing_budget.mensual[0].budget_filter[0].credit_types_treatment == "EXCLUDE_ALL_CREDITS"
    error_message = "créditos"
  }
}

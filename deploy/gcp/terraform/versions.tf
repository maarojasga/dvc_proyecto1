terraform {
  required_version = ">= 1.6"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 7.0, < 9.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }

  # Estado local por defecto (terraform.tfstate, ignorado por git). Contiene
  # en claro la contraseña de la base y los secretos HMAC, porque Terraform
  # los genera. Para compartirlo en el equipo, un bucket propio con acceso
  # restringido a sus integrantes:
  #
  #   terraform init -backend-config="bucket=<proyecto>-tfstate" -backend-config="prefix=mooc"
  #
  # tras descomentar:
  # backend "gcs" {}
}

provider "google" {
  project = var.project_id
  region  = var.region
  zone    = var.zona

  # El presupuesto (billingbudgets.googleapis.com) exige un proyecto de cuota
  # explícito cuando se usan credenciales de usuario (gcloud auth
  # application-default login); sin esto la API responde 403 aunque el
  # usuario administre la cuenta de facturación.
  user_project_override = true
  billing_project       = var.project_id

  default_labels = {
    proyecto = var.prefijo
    entrega  = "2"
  }
}

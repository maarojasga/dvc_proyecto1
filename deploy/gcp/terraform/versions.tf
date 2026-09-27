terraform {
  # 1.7: bloques removed (identidades.tf) y mock_provider de las pruebas.
  required_version = ">= 1.7"

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

  # Estado remoto en Cloud Storage, compartido por los portátiles del equipo
  # y GitHub Actions. Configuración parcial: el bucket no se escribe aquí
  # porque depende del proyecto; lo pasa deploy/gcp/lib.sh (tf_init):
  #
  #   terraform init -backend-config="bucket=<proyecto>-tfstate" -backend-config="prefix=mooc"
  #
  # El bucket lo crea deploy/gcp/bootstrap-ci.sh (versionado, acceso uniforme,
  # prevención de acceso público). El estado contiene EN CLARO la contraseña
  # de la base, la del administrador y los secretos HMAC, porque Terraform
  # los genera: quien lee el bucket lee esos secretos.
  #
  # Validar o probar sin bucket: terraform init -backend=false.
  backend "gcs" {}
}

provider "google" {
  project = var.project_id
  region  = var.region
  zone    = var.zona

  # El presupuesto (billingbudgets.googleapis.com) exige un proyecto de cuota
  # explícito cuando se usan credenciales de usuario (gcloud auth
  # application-default login); sin esto la API responde 403 aunque el
  # usuario administre la cuenta de facturación. El proyecto de cuota va en
  # todas las peticiones, así que quien aplica necesita
  # serviceusage.services.use en él (la cuenta de CI lo tiene por
  # roles/serviceusage.serviceUsageConsumer, ver bootstrap-ci.sh).
  user_project_override = true
  billing_project       = var.project_id

  default_labels = {
    proyecto = var.prefijo
    entrega  = "2"
  }
}

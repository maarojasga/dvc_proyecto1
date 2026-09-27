# Las dos VM. Misma imagen y mismo arranque; cambian la subred, la cuenta de
# servicio y que el Worker no tiene IP externa.

locals {
  arranque = templatefile("${path.module}/arranque.sh.tftpl", {
    registro_host      = "${var.region}-docker.pkg.dev"
    instalar_ops_agent = var.instalar_ops_agent
  })

  metadatos_comunes = {
    # Acceso SSH por identidad de Google (roles/compute.osAdminLogin) y no por
    # llaves en los metadatos del proyecto, que valdrían para cualquier VM.
    enable-oslogin         = "TRUE"
    block-project-ssh-keys = "TRUE"
    # Solo instala. El script de arranque es legible por cualquiera con
    # compute.instances.get: la configuración y los secretos llegan después
    # (deploy/gcp/desplegar-vm.sh y Secret Manager).
    startup-script = local.arranque
  }
}

resource "google_compute_instance" "web" {
  name         = "${var.prefijo}-web"
  machine_type = var.tipo_maquina
  zone         = var.zona

  allow_stopping_for_update = true

  boot_disk {
    initialize_params {
      image = var.imagen_vm
      size  = var.disco_gb
      type  = "pd-balanced"
    }
  }

  network_interface {
    subnetwork = google_compute_subnetwork.web.id
    access_config {
      nat_ip       = google_compute_address.web.address
      network_tier = "PREMIUM"
    }
  }

  service_account {
    email = data.google_service_account.web.email
    # cloud-platform porque Secret Manager no tiene un alcance propio: los
    # alcances son un mecanismo heredado y Google recomienda este con los
    # permisos acotados por IAM. Lo que la VM puede hacer es lo que tiene
    # su cuenta de servicio (identidades.tf, almacenamiento.tf), nada más.
    scopes = ["cloud-platform"]
  }

  shielded_instance_config {
    enable_secure_boot          = true
    enable_vtpm                 = true
    enable_integrity_monitoring = true
  }

  metadata = merge(local.metadatos_comunes, { mooc-rol = "web" })

  labels = {
    rol = "web"
  }
}

resource "google_compute_instance" "worker" {
  name         = "${var.prefijo}-worker"
  machine_type = var.tipo_maquina
  zone         = var.zona

  allow_stopping_for_update = true

  boot_disk {
    initialize_params {
      image = var.imagen_vm
      size  = var.disco_gb
      type  = "pd-balanced"
    }
  }

  # Sin access_config: sin IP externa. Sale a Google por Private Google
  # Access y a lo demás por Cloud NAT.
  network_interface {
    subnetwork = google_compute_subnetwork.worker.id
    network_ip = google_compute_address.worker_interna.address
  }

  service_account {
    email  = data.google_service_account.worker.email
    scopes = ["cloud-platform"]
  }

  shielded_instance_config {
    enable_secure_boot          = true
    enable_vtpm                 = true
    enable_integrity_monitoring = true
  }

  metadata = merge(local.metadatos_comunes, { mooc-rol = "worker" })

  labels = {
    rol = "worker"
  }

  # El arranque instala Docker desde Internet: sin NAT fallaría la primera
  # vez y la marca de arranque no se escribiría.
  depends_on = [google_compute_router_nat.nat]
}

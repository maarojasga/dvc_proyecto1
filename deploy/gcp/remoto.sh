#!/usr/bin/env bash
# Abre una sesión o ejecuta un comando en el Web o el Worker Server por el
# túnel de IAP.
#
#   deploy/gcp/remoto.sh worker                                  # sesión interactiva
#   deploy/gcp/remoto.sh web 'sudo /opt/mooc/actual/deploy/gcp/en-vm.sh estado'
#   deploy/gcp/remoto.sh web 'sudo /opt/mooc/actual/deploy/web/certificado.sh letsencrypt'
#   deploy/gcp/remoto.sh web -- -L 8025:127.0.0.1:8025 -N        # Mailpit en localhost:8025
#
# Lo que sigue a `--` va tal cual a ssh (túneles de puertos).
source "$(dirname "$0")/lib.sh"
requiere terraform
comprobar_gcloud
requiere_infra

rol="${1:?uso: remoto.sh <web|worker> [comando | -- opciones de ssh]}"
shift
vm="$(vm_de "$rol")"
if [[ "${1:-}" == "--" ]]; then
  gc compute ssh "$vm" --zone "$(salida zona)" --tunnel-through-iap "$@"
elif [[ $# -gt 0 ]]; then
  ssh_vm "$rol" "$*"
else
  gc compute ssh "$vm" --zone "$(salida zona)" --tunnel-through-iap
fi

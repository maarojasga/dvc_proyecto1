# shellcheck shell=bash
# Funciones comunes de los scripts de deploy/gcp. Se carga con `source`; no se
# ejecuta. La fuente de verdad de la infraestructura es el estado de
# Terraform: los scripts leen de sus salidas el proyecto, la zona, los
# nombres de las VM, los buckets y los secretos.

set -euo pipefail

RAIZ="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DIR_GCP="$RAIZ/deploy/gcp"
DIR_TF="$DIR_GCP/terraform"
# Estado local entre scripts (último tag publicado, último export de la
# base). Ignorado por git.
# shellcheck disable=SC2034
ESTADO="$DIR_GCP/.estado"

morir() { echo "error: $*" >&2; exit 1; }
aviso() { echo ">> $*" >&2; }

requiere() {
  local c
  for c in "$@"; do
    command -v "$c" >/dev/null 2>&1 || morir "falta el comando '$c'"
  done
}

tf() { terraform -chdir="$DIR_TF" "$@"; }

# Las salidas se leen una vez por ejecución: cada `terraform output` tarda
# casi un segundo y generar-env.sh pide una veintena.
SALIDAS=""
salida() {
  if [[ -z "$SALIDAS" ]]; then
    SALIDAS="$(tf output -no-color 2>/dev/null || true)"
    [[ -n "$SALIDAS" ]] || SALIDAS="(sin salidas)"
  fi
  # Formato de `terraform output`: nombre = "valor" (o sin comillas en
  # números y booleanos). Ningún valor de estas salidas lleva comillas.
  printf '%s\n' "$SALIDAS" | sed -n "s/^$1 = \"\{0,1\}\([^\"]*\)\"\{0,1\}\$/\1/p"
}

# Olvida las salidas leídas: tras un `terraform apply` dentro del mismo script.
releer_salidas() { SALIDAS=""; }

requiere_infra() {
  [[ -n "$(salida proyecto)" ]] || morir "no hay estado de Terraform con salidas: ejecuta deploy/gcp/desplegar-infra.sh"
}

comprobar_gcloud() {
  requiere gcloud
  gcloud auth print-access-token >/dev/null 2>&1 \
    || morir "gcloud no tiene sesión: gcloud auth login (y gcloud auth application-default login para Terraform)"
}

gc() { gcloud --project "$(salida proyecto)" "$@"; }

vm_de() {
  case "$1" in
    web) salida vm_web ;;
    worker) salida vm_worker ;;
    *) morir "rol desconocido '$1' (web|worker)" ;;
  esac
}

# ssh_vm <rol> <comando>: ejecuta en la VM por el túnel de IAP. No hay puerto
# 22 abierto a Internet: la regla de firewall solo admite 35.235.240.0/20, y
# entrar exige roles/iap.tunnelResourceAccessor y OS Login en IAM.
ssh_vm() {
  local rol="$1"
  shift
  gc compute ssh "$(vm_de "$rol")" --zone "$(salida zona)" --tunnel-through-iap --quiet --command "$*"
}

# scp_vm <rol> <local> <remoto>
scp_vm() {
  gc compute scp --zone "$(salida zona)" --tunnel-through-iap --quiet "$2" "$(vm_de "$1"):$3"
}

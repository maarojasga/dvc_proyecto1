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

# valor_tfvars <variable>: valor de texto de terraform.tfvars, vacío si no
# está (en CI no hay tfvars hasta que lo escribe escribir-tfvars.sh).
valor_tfvars() {
  [[ -f "$DIR_TF/terraform.tfvars" ]] || return 0
  sed -n "s/^[[:space:]]*$1[[:space:]]*=[[:space:]]*\"\([^\"]*\)\".*/\1/p" "$DIR_TF/terraform.tfvars" | head -1
}

# --- Estado remoto -------------------------------------------------------------
#
# El estado vive en un bucket (backend "gcs" en versions.tf, configuración
# parcial) para que el portátil de un integrante y GitHub Actions trabajen
# sobre el mismo. El bucket lo crea bootstrap-ci.sh como <proyecto>-tfstate;
# TF_STATE_BUCKET lo cambia (en CI llega de las variables del repositorio).
#
# Quien quiera estado local, sin bucket, crea terraform/backend_override.tf
# (ignorado por git) con:
#
#   terraform {
#     backend "local" {}
#   }
#
# No mezclarlo con despliegues desde GitHub: serían dos estados de la misma
# infraestructura.
PREFIJO_ESTADO="${TF_STATE_PREFIX:-mooc}"

bucket_estado() {
  if [[ -n "${TF_STATE_BUCKET:-}" ]]; then
    echo "$TF_STATE_BUCKET"
    return
  fi
  local proyecto
  proyecto="$(valor_tfvars project_id)"
  [[ -n "$proyecto" ]] || morir "sin bucket de estado: define TF_STATE_BUCKET o project_id en $DIR_TF/terraform.tfvars"
  echo "$proyecto-tfstate"
}

hay_override_backend() { [[ -f "$DIR_TF/backend_override.tf" ]]; }

# tf_init: init contra el bucket. Lo que imprime va a stderr para
# no ensuciar la salida de quien llama (salida() la captura).
tf_init() {
  if hay_override_backend; then
    tf init -input=false >&2
    return
  fi
  # Un terraform.tfstate local con recursos es un despliegue anterior al
  # backend remoto: inicializar contra el bucket vacío y aplicar intentaría
  # crearlo todo otra vez. Se migra una vez, explícitamente.
  [[ ! -s "$DIR_TF/terraform.tfstate" ]] \
    || morir "hay un estado local ($DIR_TF/terraform.tfstate): súbelo al bucket con deploy/gcp/desplegar-infra.sh --migrar-estado"
  local bucket
  bucket="$(bucket_estado)"
  gcloud storage buckets describe "gs://$bucket" --format 'value(name)' >/dev/null 2>&1 \
    || morir "no existe el bucket de estado gs://$bucket o no hay acceso a él: deploy/gcp/bootstrap-ci.sh lo crea"
  tf init -input=false -backend-config="bucket=$bucket" -backend-config="prefix=$PREFIJO_ESTADO" >&2
}

# init descarga los providers y tarda; solo se repite si el directorio no
# está ya inicializado contra este bucket (primer uso, CI, cambio de bucket).
asegurar_init() {
  if hay_override_backend; then
    [[ -d "$DIR_TF/.terraform" ]] || tf_init
    return
  fi
  local bucket
  bucket="$(bucket_estado)"
  grep -qs "\"bucket\": \"$bucket\"" "$DIR_TF/.terraform/terraform.tfstate" || tf_init
}

# Las salidas se leen una vez por ejecución: cada `terraform output` tarda
# casi un segundo y generar-env.sh pide una veintena. Se cargan en el shell
# del script (requiere_infra) y no dentro de $(salida ...), que es un
# subshell: ahí la caché se perdería al volver.
SALIDAS=""
cargar_salidas() {
  [[ -z "$SALIDAS" ]] || return 0
  asegurar_init
  SALIDAS="$(tf output -no-color 2>/dev/null || true)"
  [[ -n "$SALIDAS" ]] || SALIDAS="(sin salidas)"
}

salida() {
  cargar_salidas
  # Formato de `terraform output`: nombre = "valor" (o sin comillas en
  # números y booleanos). Ningún valor de estas salidas lleva comillas.
  printf '%s\n' "$SALIDAS" | sed -n "s/^$1 = \"\{0,1\}\([^\"]*\)\"\{0,1\}\$/\1/p"
}

# Olvida las salidas leídas: tras un `terraform apply` dentro del mismo script.
releer_salidas() {
  SALIDAS=""
  cargar_salidas
}

requiere_infra() {
  cargar_salidas
  [[ -n "$(salida proyecto)" ]] || morir "no hay estado de Terraform con salidas: ejecuta deploy/gcp/desplegar-infra.sh"
}

comprobar_gcloud() {
  requiere gcloud
  gcloud auth print-access-token >/dev/null 2>&1 \
    || morir "gcloud no tiene sesión: gcloud auth login (y gcloud auth application-default login para Terraform)"
}

# Terraform no usa la sesión de gcloud sino las credenciales de aplicación.
# En GitHub Actions, google-github-actions/auth deja un archivo de
# credenciales federadas (Workload Identity) en GOOGLE_APPLICATION_CREDENTIALS
# y no hay nada que comprobar con gcloud.
comprobar_adc() {
  [[ -n "${GOOGLE_APPLICATION_CREDENTIALS:-}" && -f "${GOOGLE_APPLICATION_CREDENTIALS:-}" ]] && return 0
  gcloud auth application-default print-access-token >/dev/null 2>&1 \
    || morir "Terraform usa las credenciales de aplicación: gcloud auth application-default login"
}

gc() { gcloud --project "$(salida proyecto)" "$@"; }

# APIs que usa la plantilla. bootstrap-ci.sh las habilita todas porque la
# cuenta de CI no puede habilitar APIs (no tiene serviceusage.services.enable,
# a propósito). billingbudgets solo hace falta con presupuesto, pero
# habilitarla no cuesta nada.
# shellcheck disable=SC2034
APIS=(
  compute.googleapis.com
  sqladmin.googleapis.com
  servicenetworking.googleapis.com
  secretmanager.googleapis.com
  artifactregistry.googleapis.com
  iap.googleapis.com
  oslogin.googleapis.com
  storage.googleapis.com
  iam.googleapis.com
  cloudresourcemanager.googleapis.com
  serviceusage.googleapis.com
  logging.googleapis.com
  monitoring.googleapis.com
  billingbudgets.googleapis.com
)

# habilitar_apis <proyecto> <api>...: habilita solo las que faltan. Listar
# pide un permiso de lectura; habilitar, uno de administración que en CI no
# se tiene: si todo está habilitado, CI no lo necesita.
habilitar_apis() {
  local proyecto="$1" api habilitadas
  shift
  local faltan=()
  habilitadas="$(gcloud services list --enabled --project "$proyecto" --format 'value(config.name)')"
  for api in "$@"; do
    grep -qx "$api" <<<"$habilitadas" || faltan+=("$api")
  done
  [[ ${#faltan[@]} -gt 0 ]] || return 0
  aviso "habilitando APIs en $proyecto: ${faltan[*]}"
  gcloud services enable "${faltan[@]}" --project "$proyecto" \
    || morir "no se pudieron habilitar ${faltan[*]}; si esto es CI, el dueño del proyecto tiene que volver a correr deploy/gcp/bootstrap-ci.sh"
}

# asegurar_clave_insignias <proyecto> <prefijo> <región>
#
# La clave de firma de insignias vive fuera de Terraform a propósito: un
# `terraform destroy` borraría el secreto, y las credenciales Open Badges ya
# emitidas dejarían de verificarse con la clave nueva del entorno recreado.
# Es una clave Ed25519 en bruto (semilla + pública, 64 bytes, en base64), el
# formato que espera BADGE_SIGNING_KEY, y Terraform no la sabe producir.
asegurar_clave_insignias() {
  local proyecto="$1" secreto="$2-badge-signing-key" region="$3"
  if ! gcloud secrets describe "$secreto" --project "$proyecto" >/dev/null 2>&1; then
    gcloud secrets create "$secreto" --project "$proyecto" \
      --replication-policy user-managed --locations "$region" \
      --labels proyecto="$2" >/dev/null
  fi
  if [[ -n "$(gcloud secrets versions list "$secreto" --project "$proyecto" \
               --filter 'state:ENABLED' --format 'value(name)' --limit 1)" ]]; then
    return
  fi
  requiere openssl
  aviso "generando la clave de firma de insignias en Secret Manager ($secreto)"
  local tmp
  tmp="$(mktemp -d)"
  chmod 700 "$tmp"
  # shellcheck disable=SC2064
  trap "rm -rf '${tmp:?}'" EXIT
  openssl genpkey -algorithm ed25519 -outform DER -out "$tmp/privada.der"
  openssl pkey -inform DER -in "$tmp/privada.der" -pubout -outform DER -out "$tmp/publica.der"
  # En DER, los últimos 32 bytes son la semilla (PKCS#8) y la clave pública
  # (SPKI); ed25519.PrivateKey de Go es la concatenación de ambas.
  { tail -c 32 "$tmp/privada.der"; tail -c 32 "$tmp/publica.der"; } | base64 | tr -d '\n' > "$tmp/clave"
  # --data-file y no el valor en la línea de órdenes, donde lo vería `ps`.
  gcloud secrets versions add "$secreto" --project "$proyecto" --data-file "$tmp/clave" >/dev/null
  rm -rf "${tmp:?}"
  trap - EXIT
}

vm_de() {
  case "$1" in
    web) salida vm_web ;;
    worker) salida vm_worker ;;
    *) morir "rol desconocido '$1' (web|worker)" ;;
  esac
}

# Caducidad de la llave SSH en OS Login. gcloud genera la llave la primera
# vez (~/.ssh/google_compute_engine, sin frase: --quiet) y registra la
# pública en el perfil de OS Login de quien entra. En un portátil conviene
# que dure; en CI la privada muere con el runner, pero la pública quedaría
# registrada para siempre en el perfil de la cuenta de servicio. Con
# --ssh-key-expire-after, que gcloud también aplica con OS Login, el perfil se
# limpia solo aunque el trabajo muera a medias.
#
# Se expande como ${OPCIONES_SSH[@]+...}: con set -u, el bash 3.2 de macOS
# trata un arreglo vacío como variable sin definir.
OPCIONES_SSH=()
if [[ -n "${MOOC_SSH_CADUCIDAD:-}" ]]; then
  OPCIONES_SSH=(--ssh-key-expire-after "$MOOC_SSH_CADUCIDAD")
elif [[ "${GITHUB_ACTIONS:-}" == true ]]; then
  OPCIONES_SSH=(--ssh-key-expire-after 1h)
fi

# ssh_vm <rol> <comando>: ejecuta en la VM por el túnel de IAP. No hay puerto
# 22 abierto a Internet: la regla de firewall solo admite 35.235.240.0/20, y
# entrar exige roles/iap.tunnelResourceAccessor y OS Login en IAM.
ssh_vm() {
  local rol="$1"
  shift
  gc compute ssh "$(vm_de "$rol")" --zone "$(salida zona)" --tunnel-through-iap --quiet \
    ${OPCIONES_SSH[@]+"${OPCIONES_SSH[@]}"} --command "$*"
}

# scp_vm <rol> <local> <remoto>
scp_vm() {
  gc compute scp --zone "$(salida zona)" --tunnel-through-iap --quiet ${OPCIONES_SSH[@]+"${OPCIONES_SSH[@]}"} \
    "$2" "$(vm_de "$1"):$3"
}

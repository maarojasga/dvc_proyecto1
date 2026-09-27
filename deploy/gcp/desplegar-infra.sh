#!/usr/bin/env bash
# Crea o actualiza la infraestructura con Terraform (deploy/gcp/terraform).
#
#   deploy/gcp/desplegar-infra.sh                     # con terraform.tfvars
#   deploy/gcp/desplegar-infra.sh -var habilitar_nat=false
#
# Los argumentos van tal cual a `terraform apply`. Antes habilita las APIs
# del proyecto (Terraform las necesita para planificar) y después asegura la
# clave de firma de insignias en Secret Manager.
source "$(dirname "$0")/lib.sh"
requiere terraform openssl
comprobar_gcloud

TFVARS="$DIR_TF/terraform.tfvars"
[[ -f "$TFVARS" ]] || morir "falta $TFVARS (cópialo de terraform.tfvars.example)"
PROYECTO="$(sed -n 's/^[[:space:]]*project_id[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' "$TFVARS")"
[[ -n "$PROYECTO" ]] || morir "project_id vacío en $TFVARS"
PREFIJO="$(sed -n 's/^[[:space:]]*prefijo[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' "$TFVARS")"
PREFIJO="${PREFIJO:-mooc}"
REGION="$(sed -n 's/^[[:space:]]*region[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' "$TFVARS")"
REGION="${REGION:-us-central1}"

gcloud auth application-default print-access-token >/dev/null 2>&1 \
  || morir "Terraform usa las credenciales de aplicación: gcloud auth application-default login"

# Todas las APIs que usa la plantilla. billingbudgets solo hace falta con
# presupuesto, pero habilitarla no cuesta nada.
APIS=(
  compute.googleapis.com
  sqladmin.googleapis.com
  servicenetworking.googleapis.com
  secretmanager.googleapis.com
  artifactregistry.googleapis.com
  iap.googleapis.com
  storage.googleapis.com
  iam.googleapis.com
  cloudresourcemanager.googleapis.com
  serviceusage.googleapis.com
  logging.googleapis.com
  monitoring.googleapis.com
  billingbudgets.googleapis.com
)
aviso "habilitando APIs en $PROYECTO"
gcloud services enable "${APIS[@]}" --project "$PROYECTO"

# La clave de firma de insignias vive fuera de Terraform a propósito: un
# `terraform destroy` borraría el secreto, y las credenciales Open Badges ya
# emitidas dejarían de verificarse con la clave nueva del entorno recreado.
# Es una clave Ed25519 en bruto (semilla + pública, 64 bytes, en base64), el
# formato que espera BADGE_SIGNING_KEY, y Terraform no la sabe producir.
SECRETO_BADGE="$PREFIJO-badge-signing-key"
asegurar_clave_insignias() {
  if ! gcloud secrets describe "$SECRETO_BADGE" --project "$PROYECTO" >/dev/null 2>&1; then
    gcloud secrets create "$SECRETO_BADGE" --project "$PROYECTO" \
      --replication-policy user-managed --locations "$REGION" \
      --labels proyecto="$PREFIJO" >/dev/null
  fi
  if [[ -n "$(gcloud secrets versions list "$SECRETO_BADGE" --project "$PROYECTO" \
               --filter 'state:ENABLED' --format 'value(name)' --limit 1)" ]]; then
    return
  fi
  aviso "generando la clave de firma de insignias en Secret Manager ($SECRETO_BADGE)"
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
  gcloud secrets versions add "$SECRETO_BADGE" --project "$PROYECTO" --data-file "$tmp/clave" >/dev/null
  rm -rf "${tmp:?}"
  trap - EXIT
}
asegurar_clave_insignias

tf init -input=false
tf apply -input=false "$@"
releer_salidas

aviso "infraestructura lista. Siguiente: deploy/gcp/publicar.sh y deploy/gcp/generar-env.sh"
tf output

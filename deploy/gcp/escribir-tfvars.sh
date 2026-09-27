#!/usr/bin/env bash
# Escribe deploy/gcp/terraform/terraform.tfvars a partir de variables de
# entorno. Lo usa GitHub Actions, donde los valores son variables del
# repositorio (Settings > Secrets and variables > Actions > Variables); en un
# portátil se copia terraform.tfvars.example a mano.
#
#   GCP_PROJECT_ID=mi-proyecto GCP_REGION=us-central1 deploy/gcp/escribir-tfvars.sh [-f]
#
# Variable de entorno       -> variable de Terraform
#   GCP_PROJECT_ID (obligatoria) project_id
#   GCP_REGION, GCP_ZONE         region, zona
#   TIPO_MAQUINA, BD_TIER        tipo_maquina, bd_tier
#   CREAR_BD (obligatoria)       crear_bd (true/false)
#   HABILITAR_NAT                habilitar_nat (true/false)
#   DOMINIO_WEB                  dominio_web
#   CUENTA_FACTURACION           cuenta_facturacion
#   PRESUPUESTO_MONTO, PRESUPUESTO_MONEDA
#
# Las vacías no se escriben y Terraform usa su valor por defecto
# (variables.tf). CREAR_BD no tiene valor por defecto a propósito: en un
# portátil, bd.sh guarda crear_bd en bd.auto.tfvars, que CI no ve; si CI
# asumiera true, recrearía vacía una base eliminada. Tiene que decirse.
#
# Ninguna es secreta, pero el registro de un repositorio público lo lee
# cualquiera: el ID de la cuenta de facturación se enmascara y no se imprime.
source "$(dirname "$0")/lib.sh"

DESTINO="$DIR_TF/terraform.tfvars"
[[ ! -f "$DESTINO" || "${1:-}" == "-f" ]] || morir "$DESTINO ya existe (usar -f para sobrescribirlo)"
[[ -n "${GCP_PROJECT_ID:-}" ]] || morir "falta GCP_PROJECT_ID"
[[ "${CREAR_BD:-}" == true || "${CREAR_BD:-}" == false ]] \
  || morir "CREAR_BD tiene que ser true o false (es '${CREAR_BD:-}'): definir la variable del repositorio; false si la base se eliminó con bd.sh eliminar"
if [[ "${GITHUB_ACTIONS:-}" == true && -n "${CUENTA_FACTURACION:-}" ]]; then
  echo "::add-mask::$CUENTA_FACTURACION"
fi

lineas=()
# agregar <tipo> <variable de Terraform> <valor>. Se valida el formato: un
# valor con comillas o saltos de línea rompería el HCL o inyectaría otras
# variables.
agregar() {
  local tipo="$1" nombre="$2" valor="$3"
  [[ -n "$valor" ]] || return 0
  case "$tipo" in
    texto)
      [[ "$valor" =~ ^[A-Za-z0-9._@:/-]+$ ]] || morir "valor no admitido para $nombre: '$valor'"
      lineas+=("$nombre = \"$valor\"") ;;
    bool)
      [[ "$valor" == true || "$valor" == false ]] || morir "$nombre tiene que ser true o false (es '$valor')"
      lineas+=("$nombre = $valor") ;;
    numero)
      [[ "$valor" =~ ^[0-9]+$ ]] || morir "$nombre tiene que ser un entero (es '$valor')"
      lineas+=("$nombre = $valor") ;;
  esac
}

agregar texto project_id "$GCP_PROJECT_ID"
agregar texto region "${GCP_REGION:-}"
agregar texto zona "${GCP_ZONE:-}"
agregar texto tipo_maquina "${TIPO_MAQUINA:-}"
agregar texto bd_tier "${BD_TIER:-}"
agregar bool habilitar_nat "${HABILITAR_NAT:-}"
agregar bool crear_bd "$CREAR_BD"
agregar texto dominio_web "${DOMINIO_WEB:-}"
agregar texto cuenta_facturacion "${CUENTA_FACTURACION:-}"
agregar numero presupuesto_monto "${PRESUPUESTO_MONTO:-}"
agregar texto presupuesto_moneda "${PRESUPUESTO_MONEDA:-}"

{
  echo "# Generado por deploy/gcp/escribir-tfvars.sh el $(date -u '+%Y-%m-%d %H:%M') UTC."
  printf '%s\n' "${lineas[@]}"
} > "$DESTINO"
sed 's/^cuenta_facturacion = .*/cuenta_facturacion = "(oculta)"/' "$DESTINO"

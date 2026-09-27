#!/usr/bin/env bash
# Crea o actualiza la infraestructura con Terraform (deploy/gcp/terraform).
#
#   deploy/gcp/desplegar-infra.sh                     # con terraform.tfvars
#   deploy/gcp/desplegar-infra.sh -var habilitar_nat=false
#   deploy/gcp/desplegar-infra.sh --plan <archivo> [opciones de plan]
#   deploy/gcp/desplegar-infra.sh --aplicar <archivo>
#   deploy/gcp/desplegar-infra.sh --migrar-estado
#
# Sin opción, los argumentos van tal cual a `terraform apply`, que muestra el
# plan y pide confirmación. Antes habilita las APIs que falten y asegura la
# clave de firma de insignias en Secret Manager.
#
# --plan y --aplicar separan las dos mitades para GitHub Actions: se revisa
# un plan guardado y se aplica exactamente ese. --plan sale como
# `terraform plan -detailed-exitcode`: 0 sin cambios, 2 con cambios, 1 error,
# y no crea nada (tampoco la clave de insignias: solo avisa si falta, y
# --aplicar la crea antes del apply).
#
# --migrar-estado sube al bucket un terraform.tfstate local de antes del
# backend remoto (una sola vez, ver README).
source "$(dirname "$0")/lib.sh"
requiere terraform
comprobar_gcloud
comprobar_adc

modo=aplicar-interactivo
archivo=""
case "${1:-}" in
  --plan|--aplicar)
    modo="${1#--}"
    archivo="${2:?falta el archivo del plan}"
    shift 2
    # Absoluto: tf() corre con -chdir y una ruta relativa acabaría dentro
    # de deploy/gcp/terraform.
    [[ "$archivo" == /* ]] || archivo="$PWD/$archivo"
    ;;
  --migrar-estado)
    modo=migrar
    shift
    ;;
esac

if [[ "$modo" == migrar ]]; then
  hay_override_backend && morir "hay un backend_override.tf: el estado ya es local por decisión propia"
  [[ -s "$DIR_TF/terraform.tfstate" ]] || morir "no hay $DIR_TF/terraform.tfstate que migrar"
  BUCKET="$(bucket_estado)"
  gcloud storage buckets describe "gs://$BUCKET" --format 'value(name)' >/dev/null \
    || morir "no existe gs://$BUCKET: deploy/gcp/bootstrap-ci.sh lo crea"
  # -force-copy responde que sí a todo, también a sobrescribir un estado
  # remoto que ya exista (p. ej. de un despliegue desde GitHub). Eso sería
  # perder la infraestructura que describe; se comprueba antes.
  if gcloud storage objects describe "gs://$BUCKET/$PREFIJO_ESTADO/default.tfstate" >/dev/null 2>&1; then
    morir "gs://$BUCKET/$PREFIJO_ESTADO/default.tfstate ya existe: hay dos estados de la misma infraestructura; decidir a mano cuál vale"
  fi
  aviso "subiendo el estado local a gs://$BUCKET/$PREFIJO_ESTADO/"
  tf init -input=false -migrate-state -force-copy \
    -backend-config="bucket=$BUCKET" -backend-config="prefix=$PREFIJO_ESTADO"
  # Terraform deja el archivo local tal cual. Se aparta con otro nombre
  # (sigue ignorado por git) para que tf_init no lo tome por un despliegue
  # sin migrar; se puede borrar cuando el remoto esté comprobado.
  apartado="$DIR_TF/terraform.tfstate.migrado-$(date -u +%Y%m%d-%H%M%S)"
  mv "$DIR_TF/terraform.tfstate" "$apartado"
  aviso "estado migrado. Copia local apartada en $apartado (contiene secretos: borrarla tras comprobar con 'terraform output')"
  exit 0
fi

if [[ "$modo" == aplicar ]]; then
  [[ -f "$archivo" ]] || morir "no existe el plan $archivo"
  tf_init
  # Proyecto, prefijo y región del propio plan (en CI no hay tfvars en este
  # trabajo). Solo esas variables: el JSON del plan lleva los secretos en
  # claro y no se imprime ni se guarda.
  requiere jq
  vars_plan="$(tf show -json "$archivo" | jq -r '[.variables.project_id.value, (.variables.prefijo.value // "mooc"), (.variables.region.value // "us-central1")] | @tsv')"
  IFS=$'\t' read -r PROYECTO PREFIJO REGION <<<"$vars_plan"
  asegurar_clave_insignias "$PROYECTO" "$PREFIJO" "$REGION"
  # Un plan guardado se aplica sin preguntar: la revisión fue antes. Si el
  # estado cambió desde que se generó, Terraform lo rechaza por obsoleto.
  tf apply -input=false "$archivo"
  releer_salidas
  tf output
  exit 0
fi

TFVARS="$DIR_TF/terraform.tfvars"
[[ -f "$TFVARS" ]] || morir "falta $TFVARS (cópialo de terraform.tfvars.example; en CI lo escribe escribir-tfvars.sh)"
PROYECTO="$(valor_tfvars project_id)"
[[ -n "$PROYECTO" ]] || morir "project_id vacío en $TFVARS"
PREFIJO="$(valor_tfvars prefijo)"
PREFIJO="${PREFIJO:-mooc}"
REGION="$(valor_tfvars region)"
REGION="${REGION:-us-central1}"

# Terraform necesita las APIs habilitadas para planificar.
habilitar_apis "$PROYECTO" "${APIS[@]}"

tf_init

if [[ "$modo" == plan ]]; then
  if ! gcloud secrets describe "$PREFIJO-badge-signing-key" --project "$PROYECTO" >/dev/null 2>&1; then
    aviso "AVISO: no existe el secreto $PREFIJO-badge-signing-key; se creará al aplicar (--aplicar)"
  fi
  rc=0
  tf plan -input=false -detailed-exitcode -out "$archivo" "$@" || rc=$?
  exit "$rc"
fi

asegurar_clave_insignias "$PROYECTO" "$PREFIJO" "$REGION"
tf apply -input=false "$@"
releer_salidas

aviso "infraestructura lista. Siguiente: deploy/gcp/publicar.sh y deploy/gcp/generar-env.sh"
tf output

#!/usr/bin/env bash
# Crea o actualiza la pila de CloudFormation (deploy/aws/infra.yaml).
#
#   deploy/aws/desplegar-infra.sh                        # con parametros.env
#   deploy/aws/desplegar-infra.sh HabilitarNatGateway=false
#
# Los argumentos Clave=Valor se suman a los de parametros.env y prevalecen.
# Antes de la pila asegura el parámetro SecureString con la contraseña de la
# base: si no existe, genera una aleatoria. Nunca se imprime ni se escribe en
# el repositorio; para verla: aws ssm get-parameter --with-decryption ...
source "$(dirname "$0")/lib.sh"
comprobar_credenciales

PARAM_PASSWORD="${PARAMETRO_PASSWORD_BD:-/${PROYECTO:-mooc}/db/master-password}"

asegurar_password_bd() {
  if aws_ ssm get-parameter --name "$PARAM_PASSWORD" --query Parameter.Name --output text >/dev/null 2>&1; then
    return
  fi
  requiere openssl
  aviso "creando la contraseña de la base en SSM ($PARAM_PASSWORD)"
  # Hexadecimal: va dentro de DATABASE_URL, y un carácter reservado de URL
  # (@, /, :) rompería la cadena de conexión. 48 hex = 192 bits.
  local tmp
  tmp="$(mktemp)"
  chmod 600 "$tmp"
  # shellcheck disable=SC2064
  trap "rm -f '$tmp'" EXIT
  openssl rand -hex 24 | tr -d '\n' > "$tmp"
  # file:// y no el valor en la línea de órdenes, donde lo vería `ps`.
  aws_ ssm put-parameter --name "$PARAM_PASSWORD" --type SecureString \
    --value "file://$tmp" --description "Contraseña maestra de RDS ($PILA)" >/dev/null
  rm -f "$tmp"
}

asegurar_password_bd

# Variable de parametros.env -> parámetro de la plantilla. Solo las no vacías.
declare -a sobre=()
agregar() { [[ -n "${2:-}" ]] && sobre+=("$1=$2"); return 0; }
agregar Proyecto "${PROYECTO:-}"
agregar CrearRolesPropios "${CREAR_ROLES_PROPIOS:-}"
agregar PerfilInstanciaExistente "${PERFIL_INSTANCIA:-}"
agregar NombreLlaveSsh "${NOMBRE_LLAVE_SSH:-}"
agregar CidrAdministracion "${CIDR_ADMINISTRACION:-}"
agregar TipoInstancia "${TIPO_INSTANCIA:-}"
agregar ClaseInstanciaBD "${CLASE_INSTANCIA_BD:-}"
agregar DominioWeb "${DOMINIO_WEB:-}"
agregar CorreoAlertasPresupuesto "${CORREO_ALERTAS_PRESUPUESTO:-}"
agregar PresupuestoMensualUSD "${PRESUPUESTO_MENSUAL_USD:-}"
agregar ParametroPasswordBD "$PARAM_PASSWORD"
for kv in "$@"; do
  [[ "$kv" == *=* ]] || morir "argumento '$kv' no es Clave=Valor"
  sobre+=("$kv")
done

aviso "desplegando la pila '$PILA' en $REGION"
# CAPABILITY_NAMED_IAM solo se usa con CrearRolesPropios=true, pero se pide
# siempre: sin ella el cambio de false a true fallaría a mitad.
aws_ cloudformation deploy \
  --stack-name "$PILA" \
  --template-file "$DIR_AWS/infra.yaml" \
  --capabilities CAPABILITY_NAMED_IAM \
  --no-fail-on-empty-changeset \
  --tags "Proyecto=${PROYECTO:-mooc}" "Entrega=2" \
  ${sobre[@]+--parameter-overrides "${sobre[@]}"}

"$DIR_AWS/salidas.sh"

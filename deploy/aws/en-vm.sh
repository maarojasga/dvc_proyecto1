#!/usr/bin/env bash
# Se ejecuta EN la máquina virtual (lo llama desplegar-vm.sh, o a mano en una
# sesión de SSM). Lee la configuración de /opt/mooc/config/<rol>.env, trae los
# secretos de SSM Parameter Store solo para esta invocación y levanta el
# compose del rol.
#
#   en-vm.sh web       # nginx + api + mailpit (y frontend si su perfil está)
#   en-vm.sh worker    # redis + worker
#   en-vm.sh migrar    # migraciones contra RDS (en el Web Server)
#   en-vm.sh estado    # contenedores y salud
#
# Los secretos acaban en el entorno de los contenedores (visible con
# `docker inspect` para quien ya es root en la máquina), pero no en disco, ni
# en el repositorio, ni en el UserData, ni en la imagen.
set -euo pipefail

accion="${1:?uso: en-vm.sh <web|worker|migrar|estado>}"
case "$accion" in
  web|migrar) rol=web ;;
  worker) rol=worker ;;
  estado)
    for r in web worker; do
      [[ -f "/opt/mooc/config/$r.env" ]] || continue
      rol=$r
    done
    ;;
  *) echo "acción desconocida '$accion'" >&2; exit 1 ;;
esac

DIR="$(cd "$(dirname "$0")/../.." && pwd)"
CONFIG="${MOOC_CONFIG:-/opt/mooc/config/$rol.env}"
[[ -f "$CONFIG" ]] || { echo "falta $CONFIG (deploy/aws/generar-env.sh + desplegar-vm.sh)" >&2; exit 1; }
set -a
# shellcheck disable=SC1090
source "$CONFIG"
set +a
export AWS_DEFAULT_REGION="${S3_REGION:?falta S3_REGION en $CONFIG}"

compose() { docker compose -f "$DIR/deploy/$rol/docker-compose.yml" --env-file "$CONFIG" "$@"; }

# parametro_ssm <nombre>: valor descifrado, vacío si el parámetro no existe.
parametro_ssm() {
  aws ssm get-parameter --with-decryption --name "$1" \
    --query Parameter.Value --output text 2>/dev/null || true
}

cargar_secretos() {
  # Si ya viene en el entorno (p. ej. sin SSM disponible), se respeta.
  if [[ -z "${DB_PASSWORD:-}" ]]; then
    DB_PASSWORD="$(parametro_ssm "${DB_PASSWORD_SSM:?falta DB_PASSWORD_SSM en $CONFIG}")"
  fi
  [[ -n "$DB_PASSWORD" ]] || { echo "no se pudo leer $DB_PASSWORD_SSM de SSM (¿rol sin ssm:GetParameter? ¿credenciales del laboratorio caducadas?)" >&2; exit 1; }
  export DB_PASSWORD
  if [[ "$rol" == web ]]; then
    ADMIN_PASSWORD="${ADMIN_PASSWORD:-$(parametro_ssm "${SSM_PREFIJO:-/mooc}/admin-password")}"
    BADGE_SIGNING_KEY="${BADGE_SIGNING_KEY:-$(parametro_ssm "${SSM_PREFIJO:-/mooc}/badge-signing-key")}"
    export ADMIN_PASSWORD BADGE_SIGNING_KEY
  fi
}

# Usa las imágenes del commit desplegado si están cargadas; si no, las
# etiquetas del .env, que compose construye si tampoco existen.
fijar_imagenes() {
  local tag
  tag="$(cat "$DIR/VERSION" 2>/dev/null || true)"
  [[ -n "$tag" ]] || return 0
  imagen() { docker image inspect "$1:$tag" >/dev/null 2>&1 && printf '%s:%s' "$1" "$tag"; }
  if [[ "$rol" == web ]]; then
    IMAGEN_API="$(imagen mooc-api || echo "${IMAGEN_API:-}")"
    IMAGEN_MIGRATE="$(imagen mooc-migrate || echo "${IMAGEN_MIGRATE:-}")"
    IMAGEN_FRONTEND="$(imagen mooc-frontend || echo "${IMAGEN_FRONTEND:-}")"
    export IMAGEN_API IMAGEN_MIGRATE IMAGEN_FRONTEND
  else
    IMAGEN_WORKER="$(imagen mooc-worker || echo "${IMAGEN_WORKER:-}")"
    export IMAGEN_WORKER
  fi
}

case "$accion" in
  web)
    cargar_secretos
    fijar_imagenes
    # nginx no arranca sin certificado: si no hay ninguno, uno autofirmado
    # provisional; certificado.sh letsencrypt lo sustituye.
    "$DIR/deploy/web/certificado.sh" asegurar
    compose up -d --remove-orphans
    compose ps
    ;;
  worker)
    cargar_secretos
    fijar_imagenes
    compose up -d --remove-orphans
    compose ps
    ;;
  migrar)
    cargar_secretos
    fijar_imagenes
    compose run --rm migrate
    ;;
  estado)
    compose ps
    df -h / | tail -1
    free -m
    ;;
esac

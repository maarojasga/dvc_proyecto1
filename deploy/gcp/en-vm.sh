#!/usr/bin/env bash
# Se ejecuta EN la VM, como root (lo llama desplegar-vm.sh, o a mano tras
# `gcloud compute ssh --tunnel-through-iap`). Lee /opt/mooc/config/<rol>.env,
# trae los secretos de Secret Manager solo para esta invocación y levanta el
# compose del rol con las imágenes de Artifact Registry del commit
# desplegado.
#
#   en-vm.sh web       # nginx + api + mailpit (y frontend si su perfil está)
#   en-vm.sh worker    # redis + worker
#   en-vm.sh migrar    # migraciones contra Cloud SQL (en el Web Server)
#   en-vm.sh sembrar   # datos sintéticos de las pruebas de carga (Web Server)
#   en-vm.sh estado    # contenedores, disco y memoria
#
# Los secretos acaban en el entorno de los contenedores (visible con
# `docker inspect` para quien ya es root en la VM), pero no en disco, ni en
# el repositorio, ni en los metadatos de la VM, ni en la imagen. Cada VM solo
# puede leer los suyos: los permisos están por secreto en Terraform.
set -euo pipefail

accion="${1:?uso: en-vm.sh <web|worker|migrar|sembrar|estado>}"
case "$accion" in
  web|migrar|sembrar) rol=web ;;
  worker) rol=worker ;;
  estado)
    rol=""
    for r in web worker; do
      [[ -f "/opt/mooc/config/$r.env" ]] && rol=$r
    done
    [[ -n "$rol" ]] || { echo "sin configuración en /opt/mooc/config" >&2; exit 1; }
    ;;
  *) echo "acción desconocida '$accion'" >&2; exit 1 ;;
esac

DIR="$(cd "$(dirname "$0")/../.." && pwd)"
CONFIG="${MOOC_CONFIG:-/opt/mooc/config/$rol.env}"
[[ -f "$CONFIG" ]] || { echo "falta $CONFIG (deploy/gcp/generar-env.sh + desplegar-vm.sh)" >&2; exit 1; }
# shellcheck source=deploy/gcp/env.sh
source "$DIR/deploy/gcp/env.sh"
cargar_env "$CONFIG"

compose() { docker compose -f "$DIR/deploy/$rol/docker-compose.yml" --env-file "$CONFIG" "$@"; }

# secreto <nombre>: última versión, vacío si no existe o no hay permiso.
secreto() {
  [[ -n "${1:-}" ]] || return 0
  gcloud secrets versions access latest --secret "$1" --project "${PROYECTO_GCP:?falta PROYECTO_GCP en $CONFIG}" 2>/dev/null || true
}

obligatorio() {
  [[ -n "${!1:-}" ]] || {
    echo "no se pudo leer $1 de Secret Manager (secreto '$2'): ¿existe con versión activa? ¿la cuenta de servicio de esta VM tiene secretAccessor sobre él?" >&2
    exit 1
  }
}

cargar_secretos() {
  # Si ya vienen en el entorno se respetan (pruebas a mano).
  DB_PASSWORD="${DB_PASSWORD:-$(secreto "${SECRETO_DB_PASSWORD:-}")}"
  obligatorio DB_PASSWORD "${SECRETO_DB_PASSWORD:-}"
  S3_SECRET_KEY="${S3_SECRET_KEY:-$(secreto "${SECRETO_S3_SECRET_KEY:-}")}"
  obligatorio S3_SECRET_KEY "${SECRETO_S3_SECRET_KEY:-}"
  export DB_PASSWORD S3_SECRET_KEY
  if [[ "$rol" == web ]]; then
    # La contraseña del administrador solo viaja mientras ADMIN_EMAIL esté
    # definido: el administrador se crea en el primer arranque y después
    # conviene retirar el correo del .env.
    ADMIN_PASSWORD="${ADMIN_PASSWORD:-}"
    if [[ -n "${ADMIN_EMAIL:-}" && -z "$ADMIN_PASSWORD" ]]; then
      ADMIN_PASSWORD="$(secreto "${SECRETO_ADMIN_PASSWORD:-}")"
    fi
    BADGE_SIGNING_KEY="${BADGE_SIGNING_KEY:-$(secreto "${SECRETO_BADGE_SIGNING_KEY:-}")}"
    export ADMIN_PASSWORD BADGE_SIGNING_KEY
  fi
}

# Imágenes del commit desplegado (VERSION lo escribe desplegar-vm.sh).
fijar_imagenes() {
  local tag
  tag="$(cat "$DIR/VERSION" 2>/dev/null || true)"
  [[ -n "$tag" ]] || { echo "falta $DIR/VERSION: despliega con deploy/gcp/desplegar-vm.sh" >&2; exit 1; }
  : "${REGISTRO:?falta REGISTRO en $CONFIG}"
  if [[ "$rol" == web ]]; then
    export IMAGEN_API="$REGISTRO/mooc-api:$tag"
    export IMAGEN_MIGRATE="$REGISTRO/mooc-migrate:$tag"
    export IMAGEN_SEED="$REGISTRO/mooc-seed:$tag"
    export IMAGEN_FRONTEND="$REGISTRO/mooc-frontend:$tag"
  else
    export IMAGEN_WORKER="$REGISTRO/mooc-worker:$tag"
    export IMAGEN_REDIS="$REGISTRO/redis:7.4-alpine"
  fi
}

case "$accion" in
  web)
    cargar_secretos
    fijar_imagenes
    compose pull --quiet
    # nginx no arranca sin certificado: si no hay ninguno, uno autofirmado
    # provisional; `certificado.sh letsencrypt` lo sustituye.
    MOOC_CONFIG="$CONFIG" "$DIR/deploy/web/certificado.sh" asegurar
    compose up -d --remove-orphans
    # nginx se recrea siempre. Monta su configuración a través del enlace
    # /opt/mooc/actual, y un bind mount fija el archivo al crear el
    # contenedor: sin recrearlo seguiría leyendo la versión anterior, que la
    # rotación de desplegar-vm.sh acaba borrando. Compose no lo detecta,
    # porque la ruta escrita en el compose no cambia entre versiones.
    compose up -d --no-deps --force-recreate nginx
    compose ps
    ;;
  worker)
    cargar_secretos
    fijar_imagenes
    compose pull --quiet
    compose up -d --remove-orphans
    compose ps
    ;;
  migrar)
    cargar_secretos
    fijar_imagenes
    compose run --rm migrate
    ;;
  sembrar)
    # Cloud SQL solo es alcanzable desde las dos VM, así que el sembrador
    # corre aquí. El escenario (ids y sesiones) queda en /opt/mooc/salida
    # para copiarlo a la máquina del generador de carga:
    #   deploy/gcp/remoto.sh web 'sudo cat /opt/mooc/salida/escenario.json' > load/salida/escenario.json
    cargar_secretos
    fijar_imagenes
    mkdir -p /opt/mooc/salida
    compose run --rm seed
    ;;
  estado)
    compose ps
    df -h / | tail -1
    free -m
    ;;
esac

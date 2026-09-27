#!/usr/bin/env bash
# Construye las imágenes FUERA de las e2-small y las publica en Artifact
# Registry con el commit como etiqueta.
#
#   deploy/gcp/publicar.sh              # api, migrate, seed, worker y réplica de redis
#   deploy/gcp/publicar.sh --frontend   # además el frontend existente
#
# Por qué no compilar en las VM: el builder de Go y la imagen del worker
# (LibreOffice + JRE + FFmpeg) pasan con facilidad de 1,5 GB al construirse,
# y en 2 GB eso es swap o el OOM killer; además consume la ráfaga de CPU de
# la e2-small antes de la primera prueba de carga.
source "$(dirname "$0")/lib.sh"
requiere git docker terraform
comprobar_gcloud
requiere_infra

frontend=false
for a in "$@"; do
  case "$a" in
    --frontend) frontend=true ;;
    *) morir "opción desconocida '$a'" ;;
  esac
done

cd "$RAIZ" || exit 1
# Solo commits: una imagen de un árbol con cambios sin confirmar no se puede
# reconstruir ni asociar al tag de la entrega. Los archivos sin seguimiento
# solo importan dentro de los contextos de construcción: fuera de ellos
# quedan los que generan los propios scripts y CI (.terraform.lock.hcl, las
# credenciales federadas gha-creds-*.json), que no llegan a la imagen.
[[ -z "$(git status --porcelain --untracked-files=no)$(git status --porcelain backend frontend)" ]] \
  || morir "hay cambios sin confirmar; haz commit antes de publicar"
# MOOC_TAG lo fija CI (los 12 primeros caracteres de GITHUB_SHA) para que el
# trabajo que despliega use exactamente la misma etiqueta.
TAG="${MOOC_TAG:-$(git rev-parse --short=12 HEAD)}"
REGISTRO="$(salida registro)"
[[ -n "$REGISTRO" ]] || morir "sin salida 'registro' en Terraform"

gcloud auth configure-docker "${REGISTRO%%/*}" --quiet >/dev/null

# Caché de capas en GitHub Actions (type=gha): sin ella cada ejecución
# recompila Go y reinstala LibreOffice y FFmpeg desde cero. Necesita las
# variables ACTIONS_* que expone crazy-max/ghaction-github-runtime y un
# builder docker-container (docker/setup-buildx-action); fuera de CI no hay
# caché remota y se usa la local de Docker.
CACHE=()
fijar_cache() {
  CACHE=()
  if [[ -n "${ACTIONS_RUNTIME_TOKEN:-}" ]]; then
    CACHE=(--cache-from "type=gha,scope=mooc-$1" --cache-to "type=gha,mode=max,scope=mooc-$1")
  fi
}

# linux/amd64 explícito: las e2 son x86 y un portátil con Apple Silicon
# construiría arm64 por defecto, que luego no arranca ("exec format error").
construir() {
  aviso "construyendo $1"
  fijar_cache "$1"
  docker buildx build --platform linux/amd64 --target "$1" ${CACHE[@]+"${CACHE[@]}"} \
    -t "$REGISTRO/mooc-$1:$TAG" --push backend
}
construir api
construir migrate
construir seed
construir worker

if [[ "$frontend" == true ]]; then
  # Next incrusta NEXT_PUBLIC_API_URL al compilar: la imagen queda atada al
  # origen del Web Server (si cambia la IP o el dominio, se vuelve a publicar).
  aviso "construyendo frontend para $(salida url_publica)"
  fijar_cache frontend
  docker buildx build --platform linux/amd64 ${CACHE[@]+"${CACHE[@]}"} \
    --build-arg "NEXT_PUBLIC_API_URL=$(salida url_publica)" \
    -t "$REGISTRO/mooc-frontend:$TAG" --push frontend
fi

# Redis se replica en el registro para que el Worker no dependa de Docker Hub
# ni de Cloud NAT: Artifact Registry le llega por Private Google Access.
# imagetools copia el manifiesto entre registros sin descargar la imagen aquí.
aviso "replicando redis:7.4-alpine en Artifact Registry"
docker buildx imagetools create -t "$REGISTRO/redis:7.4-alpine" docker.io/library/redis:7.4-alpine

mkdir -p "$ESTADO"
echo "$TAG" > "$ESTADO/ultimo-tag"
aviso "publicado $REGISTRO/*:$TAG"

#!/usr/bin/env bash
# Construye las imágenes FUERA de las e2-small y las publica en Artifact
# Registry con el commit como etiqueta.
#
#   deploy/gcp/publicar.sh              # api, migrate, worker y réplica de redis
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

cd "$RAIZ"
# Solo commits: una imagen de un árbol con cambios sin confirmar no se puede
# reconstruir ni asociar al tag de la entrega.
[[ -z "$(git status --porcelain)" ]] || morir "hay cambios sin confirmar; haz commit antes de publicar"
TAG="$(git rev-parse --short=12 HEAD)"
REGISTRO="$(salida registro)"
[[ -n "$REGISTRO" ]] || morir "sin salida 'registro' en Terraform"

gcloud auth configure-docker "${REGISTRO%%/*}" --quiet >/dev/null

# linux/amd64 explícito: las e2 son x86 y un portátil con Apple Silicon
# construiría arm64 por defecto, que luego no arranca ("exec format error").
construir() {
  aviso "construyendo $1"
  docker buildx build --platform linux/amd64 --target "$1" \
    -t "$REGISTRO/mooc-$1:$TAG" --push backend
}
construir api
construir migrate
construir worker

if [[ "$frontend" == true ]]; then
  # Next incrusta NEXT_PUBLIC_API_URL al compilar: la imagen queda atada al
  # origen del Web Server (si cambia la IP o el dominio, se vuelve a publicar).
  aviso "construyendo frontend para $(salida url_publica)"
  docker buildx build --platform linux/amd64 \
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

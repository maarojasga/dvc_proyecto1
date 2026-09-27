#!/usr/bin/env bash
# Construye las imágenes FUERA de las t3.small y las deja, junto con el código
# del commit, en s3://<bucket>/artefactos/<commit>/.
#
#   deploy/aws/publicar.sh                 # código + imágenes de web y worker
#   deploy/aws/publicar.sh --sin-imagenes  # solo el código (las VMs compilan)
#   deploy/aws/publicar.sh --frontend      # añade la imagen del frontend
#
# Por qué no compilar en las VMs: el builder de Go y la imagen del worker
# (LibreOffice + JRE + FFmpeg) superan con facilidad 1,5 GiB durante la
# construcción, y en 2 GiB eso es swap intenso o el OOM killer; además quema
# los créditos de CPU de la t3 antes de la primera prueba de carga.
#
# Por qué S3 y no ECR ni Docker Hub: el Worker está en una subred privada y
# llega a S3 por el endpoint de gateway, sin NAT; ECR exigiría NAT o endpoints
# de interfaz (~7 USD/mes cada uno). Los artefactos caducan a los 14 días
# (regla de ciclo de vida del bucket).
source "$(dirname "$0")/lib.sh"
comprobar_credenciales
requiere git docker gzip

imagenes=true
frontend=false
for a in "$@"; do
  case "$a" in
    --sin-imagenes) imagenes=false ;;
    --frontend) frontend=true ;;
    *) morir "opción desconocida '$a'" ;;
  esac
done

cd "$RAIZ"
# Solo commits: una imagen de un árbol con cambios sin confirmar no se puede
# volver a construir ni asociar a un tag de entrega.
[[ -z "$(git status --porcelain)" ]] || morir "hay cambios sin confirmar; haz commit antes de publicar"
TAG="$(git rev-parse --short=12 HEAD)"
BUCKET="$(salida NombreBucket)"
[[ -n "$BUCKET" ]] || morir "la pila no tiene bucket"
DESTINO="s3://$BUCKET/artefactos/$TAG"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

aviso "código del commit $TAG"
git archive --format=tar.gz -o "$TMP/codigo.tar.gz" HEAD
aws_ s3 cp --only-show-errors "$TMP/codigo.tar.gz" "$DESTINO/codigo.tar.gz"

if [[ "$imagenes" == true ]]; then
  # linux/amd64 explícito: las t3 son x86 y un portátil con Apple Silicon
  # construiría arm64 por defecto, que luego no arranca ("exec format error").
  construir() {
    docker buildx build --platform linux/amd64 --load --target "$1" -t "$2" backend
  }
  construir api "mooc-api:$TAG"
  construir migrate "mooc-migrate:$TAG"
  construir worker "mooc-worker:$TAG"
  # Redis viaja en el paquete del Worker para que su despliegue no dependa de
  # Docker Hub (ni de la NAT).
  docker pull --platform linux/amd64 redis:7.4-alpine

  web_imgs=("mooc-api:$TAG" "mooc-migrate:$TAG")
  if [[ "$frontend" == true ]]; then
    url="$(salida UrlPublica)"
    docker buildx build --platform linux/amd64 --load \
      --build-arg "NEXT_PUBLIC_API_URL=$url" -t "mooc-frontend:$TAG" frontend
    web_imgs+=("mooc-frontend:$TAG")
  fi

  aviso "empaquetando imágenes"
  docker save "${web_imgs[@]}" | gzip > "$TMP/imagenes-web.tar.gz"
  docker save "mooc-worker:$TAG" redis:7.4-alpine | gzip > "$TMP/imagenes-worker.tar.gz"
  aws_ s3 cp --only-show-errors "$TMP/imagenes-web.tar.gz" "$DESTINO/imagenes-web.tar.gz"
  aws_ s3 cp --only-show-errors "$TMP/imagenes-worker.tar.gz" "$DESTINO/imagenes-worker.tar.gz"
fi

mkdir -p "$ESTADO"
echo "$TAG" > "$ESTADO/ultimo-tag"
aviso "publicado en $DESTINO (tag $TAG)"

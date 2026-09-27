#!/usr/bin/env bash
# Despliega en el Web o el Worker Server un commit publicado con publicar.sh:
# copia por IAP los archivos de deploy/ de ese commit y el .env del rol, y
# arranca los contenedores con en-vm.sh, que hace pull de Artifact Registry.
#
#   deploy/gcp/desplegar-vm.sh worker         # primero el Worker (Redis)
#   deploy/gcp/desplegar-vm.sh web
#   deploy/gcp/desplegar-vm.sh web <tag>      # un commit publicado concreto
#
# Las versiones quedan en /opt/mooc/releases/<tag> y /opt/mooc/actual apunta
# a la activa: volver atrás es desplegar el tag anterior.
source "$(dirname "$0")/lib.sh"
requiere git terraform tar
comprobar_gcloud
requiere_infra

rol="${1:?uso: desplegar-vm.sh <web|worker> [tag]}"
[[ "$rol" == web || "$rol" == worker ]] || morir "rol desconocido '$rol'"
TAG="${2:-$(cat "$ESTADO/ultimo-tag" 2>/dev/null || true)}"
[[ -n "$TAG" ]] || morir "sin tag: ejecuta publicar.sh o pásalo como argumento"
git -C "$RAIZ" cat-file -e "$TAG^{commit}" 2>/dev/null || morir "el tag $TAG no es un commit de este repositorio"
ENV_LOCAL="$RAIZ/deploy/$rol/.env"
[[ -f "$ENV_LOCAL" ]] || morir "falta $ENV_LOCAL (deploy/gcp/generar-env.sh)"

# Que el commit exista en git no quiere decir que se haya publicado. Sin
# esta comprobación el fallo llega después, como un `compose pull` roto en
# la VM a medio desplegar.
case "$rol" in
  web) imagenes=(mooc-api mooc-migrate) ;;
  worker) imagenes=(mooc-worker redis) ;;
esac
for img in "${imagenes[@]}"; do
  etiqueta="$TAG"
  [[ "$img" == redis ]] && etiqueta=7.4-alpine
  gc artifacts docker images describe "$(salida registro)/$img:$etiqueta" >/dev/null 2>&1 \
    || morir "no está publicada $(salida registro)/$img:$etiqueta: ejecuta deploy/gcp/publicar.sh en ese commit"
done

TMP="$(mktemp -d)"
trap 'rm -rf "${TMP:?}"' EXIT
mkdir -p "$TMP/paquete"
# deploy/ del commit publicado, no del árbol de trabajo: los compose y los
# scripts tienen que ser los que corresponden a las imágenes.
git -C "$RAIZ" archive --format=tar "$TAG" deploy | tar -x -C "$TMP/paquete"
echo "$TAG" > "$TMP/paquete/VERSION"
cp "$ENV_LOCAL" "$TMP/paquete/$rol.env"
tar -czf "$TMP/paquete.tgz" -C "$TMP/paquete" .

aviso "copiando el paquete $TAG a $rol"
scp_vm "$rol" "$TMP/paquete.tgz" "/tmp/mooc-$TAG.tgz"

aviso "desplegando $TAG en $rol"
ssh_vm "$rol" "sudo bash -s -- '$TAG' '$rol'" <<'REMOTO'
set -euo pipefail
TAG="${1:?}" ROL="${2:?}"
if [[ ! -f /var/lib/mooc-arranque-ok ]]; then
  echo "el arranque de la VM no terminó: sudo tail -50 /var/log/mooc-arranque.log" >&2
  exit 1
fi
REL="/opt/mooc/releases/$TAG"
rm -rf "${REL:?}"
mkdir -p "$REL"
tar -xzf "/tmp/mooc-$TAG.tgz" -C "$REL"
# La configuración vive fuera de las versiones, con permisos de root.
install -m 600 "$REL/$ROL.env" "/opt/mooc/config/$ROL.env"
rm -f "${REL:?}/${ROL:?}.env" "/tmp/mooc-${TAG:?}.tgz"
ln -sfn "$REL" /opt/mooc/actual
/opt/mooc/actual/deploy/gcp/en-vm.sh "$ROL"
# Se conservan las 5 versiones más recientes para poder volver atrás.
ls -1dt /opt/mooc/releases/*/ | tail -n +6 | xargs -r rm -rf
REMOTO

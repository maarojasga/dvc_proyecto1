#!/usr/bin/env bash
# Despliega en el Web o el Worker Server lo publicado por publicar.sh:
# descarga el código y las imágenes del bucket, instala el .env generado por
# generar-env.sh y arranca los contenedores con en-vm.sh.
#
#   deploy/aws/desplegar-vm.sh worker         # primero el Worker (Redis)
#   deploy/aws/desplegar-vm.sh web
#   deploy/aws/desplegar-vm.sh web <tag>      # un commit publicado concreto
#
# El .env viaja dentro del comando de SSM; puede hacerlo porque no lleva
# secretos. Las versiones quedan en /opt/mooc/releases/<tag> y
# /opt/mooc/actual apunta a la activa: volver atrás es desplegar el tag
# anterior.
source "$(dirname "$0")/lib.sh"
comprobar_credenciales

rol="${1:?uso: desplegar-vm.sh <web|worker> [tag]}"
[[ "$rol" == web || "$rol" == worker ]] || morir "rol desconocido '$rol'"
TAG="${2:-$(cat "$ESTADO/ultimo-tag" 2>/dev/null || true)}"
[[ -n "$TAG" ]] || morir "sin tag: ejecuta publicar.sh o pásalo como argumento"
ENV_LOCAL="$RAIZ/deploy/$rol/.env"
[[ -f "$ENV_LOCAL" ]] || morir "falta $ENV_LOCAL (deploy/aws/generar-env.sh)"
BUCKET="$(salida NombreBucket)"

env_b64="$(base64 < "$ENV_LOCAL" | tr -d '\n')"

"$DIR_AWS/remoto.sh" "$rol" <<REMOTO
set -euo pipefail
export AWS_DEFAULT_REGION="$REGION"
test -f /var/lib/mooc-arranque-ok || { echo "el arranque de la máquina no terminó: ver /var/log/mooc-arranque.log"; exit 1; }
ORIGEN="s3://$BUCKET/artefactos/$TAG"
REL="/opt/mooc/releases/$TAG"
mkdir -p "\$REL" /opt/mooc/config
aws s3 cp --only-show-errors "\$ORIGEN/codigo.tar.gz" - | tar -xz -C "\$REL"
echo "$TAG" > "\$REL/VERSION"
echo "$env_b64" | base64 -d > /opt/mooc/config/$rol.env
chmod 600 /opt/mooc/config/$rol.env
if aws s3 ls "\$ORIGEN/imagenes-$rol.tar.gz" >/dev/null 2>&1; then
  aws s3 cp --only-show-errors "\$ORIGEN/imagenes-$rol.tar.gz" - | gunzip | docker load
else
  echo "sin imágenes publicadas para $TAG: compose las construirá en la máquina"
fi
ln -sfn "\$REL" /opt/mooc/actual
chown -R ec2-user:ec2-user /opt/mooc
/opt/mooc/actual/deploy/aws/en-vm.sh $rol
REMOTO

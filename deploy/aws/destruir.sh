#!/usr/bin/env bash
# Elimina el entorno completo. Pensado para después de entregar.
#
#   destruir.sh --confirmar [--respaldar-bucket DIR] [--vaciar-bucket]
#
# Qué se conserva y qué no:
# - RDS: snapshot manual antes de borrar (rds.sh snapshot), más el final de la
#   DeletionPolicy. Cuestan ~0,095 USD/GB-mes; con 20 GiB, < 2 USD/mes.
# - SSM: la contraseña de la base NO se borra: la necesita la base restaurada.
# - Bucket: CloudFormation no borra un bucket con objetos. Sin
#   --vaciar-bucket la pila falla al final y el bucket se queda (con su costo
#   de almacenamiento); con --respaldar-bucket se copia antes a un directorio.
#
# Para reconstruir: desplegar-infra.sh, rds.sh recrear <snapshot>, publicar.sh
# (o reutilizar un tag de menos de 14 días), generar-env.sh y desplegar-vm.sh.
source "$(dirname "$0")/lib.sh"
comprobar_credenciales

confirmar=false; vaciar=false; respaldo=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --confirmar) confirmar=true ;;
    --vaciar-bucket) vaciar=true ;;
    --respaldar-bucket) respaldo="${2:?falta el directorio}"; shift ;;
    *) morir "opción desconocida '$1'" ;;
  esac
  shift
done
[[ "$confirmar" == true ]] || morir "esto borra la pila '$PILA' en $REGION; repite con --confirmar"

BUCKET="$(salida NombreBucket)"

if [[ -n "$(salida BdIdentificador)" ]]; then
  "$DIR_AWS/rds.sh" snapshot
fi

if [[ -n "$respaldo" ]]; then
  aviso "copiando s3://$BUCKET a $respaldo"
  aws_ s3 sync --only-show-errors "s3://$BUCKET" "$respaldo"
fi
if [[ "$vaciar" == true ]]; then
  aviso "vaciando s3://$BUCKET"
  aws_ s3 rm --only-show-errors --recursive "s3://$BUCKET"
fi

aviso "eliminando la pila $PILA"
aws_ cloudformation delete-stack --stack-name "$PILA"
if ! aws_ cloudformation wait stack-delete-complete --stack-name "$PILA"; then
  morir "la pila no terminó de borrarse (¿bucket con objetos?). Ver eventos en la consola de CloudFormation"
fi
aviso "pila eliminada. Snapshots conservados:"
aws_ rds describe-db-snapshots --snapshot-type manual \
  --query "DBSnapshots[?starts_with(DBSnapshotIdentifier, '${PROYECTO:-mooc}')].DBSnapshotIdentifier" --output text

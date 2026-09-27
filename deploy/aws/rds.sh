#!/usr/bin/env bash
# Ciclo de vida de la base de datos, que es lo que el enunciado pide borrar al
# final y poder reconstruir para la sustentación.
#
#   rds.sh estado
#   rds.sh snapshot [id]        # snapshot manual y espera a que esté listo
#   rds.sh eliminar             # snapshot manual + CrearBaseDeDatos=false
#   rds.sh recrear <snapshot>   # nueva instancia restaurada desde el snapshot
#   rds.sh detener | iniciar
#
# Tras recrear, el endpoint es el mismo (depende del identificador, la cuenta
# y la región), así que los .env de las máquinas siguen valiendo; basta con
# reiniciar la API y el worker para que abran conexiones nuevas.
source "$(dirname "$0")/lib.sh"
comprobar_credenciales

accion="${1:?uso: rds.sh <estado|snapshot|eliminar|recrear|detener|iniciar>}"
BD="$(salida BdIdentificador)"

requiere_bd() { [[ -n "$BD" ]] || morir "la pila no tiene base de datos (CrearBaseDeDatos=false)"; }

snapshot() {
  requiere_bd
  local id="${1:-$BD-manual-$(date -u +%Y%m%d-%H%M)}"
  aviso "snapshot $id de $BD (tarda unos minutos)"
  aws_ rds create-db-snapshot --db-instance-identifier "$BD" --db-snapshot-identifier "$id" \
    --tags Key=Proyecto,Value="${PROYECTO:-mooc}" >/dev/null
  aws_ rds wait db-snapshot-available --db-snapshot-identifier "$id"
  mkdir -p "$ESTADO"
  echo "$id" > "$ESTADO/ultimo-snapshot"
  aviso "snapshot listo: $id (guardado en deploy/aws/.estado/ultimo-snapshot)"
}

case "$accion" in
  estado)
    if [[ -n "$BD" ]]; then
      aws_ rds describe-db-instances --db-instance-identifier "$BD" \
        --query 'DBInstances[0].[DBInstanceIdentifier,DBInstanceStatus,DBInstanceClass,EngineVersion,AllocatedStorage,Endpoint.Address]' \
        --output table
    else
      aviso "sin instancia de base de datos en la pila"
    fi
    aws_ rds describe-db-snapshots --snapshot-type manual \
      --query "DBSnapshots[?starts_with(DBSnapshotIdentifier, '${PROYECTO:-mooc}')].[DBSnapshotIdentifier,SnapshotCreateTime,AllocatedStorage,Status]" \
      --output table
    ;;
  snapshot)
    snapshot "${2:-}"
    ;;
  eliminar)
    # Snapshot con nombre conocido antes de borrar. La DeletionPolicy de la
    # plantilla toma otro al eliminar, pero con nombre generado; este es el
    # que se usa para recrear, y el otro puede borrarse para no pagar dos.
    snapshot "${2:-}"
    "$DIR_AWS/desplegar-infra.sh" CrearBaseDeDatos=false
    aviso "base eliminada. Para recrearla: deploy/aws/rds.sh recrear $(cat "$ESTADO/ultimo-snapshot")"
    ;;
  recrear)
    id="${2:?uso: rds.sh recrear <snapshot>}"
    # El mismo SnapshotParaRestaurar tiene que mantenerse en adelante: si
    # cambia o se vacía, CloudFormation reemplaza la instancia por otra
    # vacía. desplegar-infra.sh no lo toca salvo que se le pase.
    "$DIR_AWS/desplegar-infra.sh" CrearBaseDeDatos=true "SnapshotParaRestaurar=$id"
    aviso "recreada desde $id. Reinicia api y worker: deploy/aws/desplegar-vm.sh worker && deploy/aws/desplegar-vm.sh web"
    ;;
  detener)
    requiere_bd
    # AWS la vuelve a arrancar sola a los 7 días y se sigue cobrando el
    # almacenamiento y los snapshots mientras está detenida.
    aws_ rds stop-db-instance --db-instance-identifier "$BD" >/dev/null
    aviso "deteniendo $BD; se reiniciará sola en 7 días si no se vuelve a detener"
    ;;
  iniciar)
    requiere_bd
    aws_ rds start-db-instance --db-instance-identifier "$BD" >/dev/null
    aws_ rds wait db-instance-available --db-instance-identifier "$BD"
    aviso "$BD disponible"
    ;;
  *)
    morir "acción desconocida '$accion'"
    ;;
esac

#!/usr/bin/env bash
# Ciclo de vida de Cloud SQL, que es lo que el enunciado pide eliminar al
# final y poder reconstruir para la sustentación.
#
#   bd.sh estado
#   bd.sh exportar [--descargar]   # export SQL al bucket de respaldos (y copia local)
#   bd.sh eliminar                 # exportar + borrar la instancia (crear_bd=false)
#   bd.sh recrear <gs://...|archivo.sql.gz>   # instancia nueva + importar el export
#   bd.sh detener | iniciar
#
# Por qué exportar y no confiar en los backups: los backups automáticos de
# Cloud SQL se borran con la instancia. El export es un .sql.gz en un bucket
# (y, con --descargar, en deploy/gcp/respaldos/, ignorado por git) que
# sobrevive a la instancia y sirve para recrearla.
#
# crear_bd se guarda en terraform/bd.auto.tfvars (ignorado por git), que
# Terraform lee solo: así un desplegar-infra.sh posterior no vuelve a crear
# la base que se acaba de eliminar. Ese archivo solo existe en este
# portátil: si el equipo despliega desde GitHub Actions, la variable del
# repositorio CREAR_BD tiene que decir lo mismo, o el siguiente `infra` de CI
# recrearía una base vacía (o borraría la recreada).
source "$(dirname "$0")/lib.sh"
requiere terraform
comprobar_gcloud
requiere_infra

accion="${1:?uso: bd.sh <estado|exportar|eliminar|recrear|detener|iniciar>}"
shift
BD="$(salida bd_instancia)"
RESPALDOS="$(salida bucket_respaldos)"
AUTO_TFVARS="$DIR_TF/bd.auto.tfvars"

requiere_bd() { [[ -n "$BD" ]] || morir "no hay instancia de Cloud SQL (crear_bd=false)"; }

fijar_crear_bd() {
  printf '# Lo escribe deploy/gcp/bd.sh. Borrar para volver al valor de terraform.tfvars.\ncrear_bd = %s\n' "$1" > "$AUTO_TFVARS"
}

exportar() {
  requiere_bd
  local descargar="${1:-false}" uri
  uri="gs://$RESPALDOS/exports/$BD-$(date -u +%Y%m%d-%H%M%S).sql.gz"
  aviso "exportando la base mooc de $BD a $uri (tarda unos minutos)"
  # Lo escribe el agente de servicio de la instancia, no quien ejecuta esto:
  # por eso Terraform le da objectAdmin sobre el bucket de respaldos.
  gc sql export sql "$BD" "$uri" --database mooc --quiet
  mkdir -p "$ESTADO"
  echo "$uri" > "$ESTADO/ultimo-export"
  if [[ "$descargar" == true ]]; then
    mkdir -p "$DIR_GCP/respaldos"
    gc storage cp "$uri" "$DIR_GCP/respaldos/"
    aviso "copia local en deploy/gcp/respaldos/$(basename "$uri") (contiene datos y hashes de contraseñas: no subirla al repositorio)"
  fi
  aviso "export listo: $uri"
}

case "$accion" in
  estado)
    if [[ -n "$BD" ]]; then
      gc sql instances describe "$BD" \
        --format 'table(name,databaseVersion,settings.tier,settings.edition,state,settings.activationPolicy,settings.dataDiskSizeGb,ipAddresses[0].ipAddress)'
      gc sql backups list --instance "$BD" --limit 5
    else
      aviso "sin instancia de Cloud SQL"
    fi
    gc storage ls -l "gs://$RESPALDOS/exports/" 2>/dev/null || aviso "sin exports en gs://$RESPALDOS/exports/"
    ;;
  exportar)
    descargar=false
    [[ "${1:-}" == "--descargar" ]] && descargar=true
    exportar "$descargar"
    ;;
  eliminar)
    requiere_bd
    exportar true
    # Dos pasos: primero se baja la protección con la instancia aún en el
    # estado, porque Terraform comprueba deletion_protection con el valor
    # guardado, no con el del plan que la borra.
    aviso "bajando la protección contra borrado de $BD"
    tf apply -input=false -var crear_bd=true -var bd_proteccion_terraform=false -var bd_proteccion_api=false
    fijar_crear_bd false
    aviso "eliminando $BD"
    tf apply -input=false -var bd_proteccion_terraform=false -var bd_proteccion_api=false
    aviso "base eliminada. Para recrearla: deploy/gcp/bd.sh recrear $(cat "$ESTADO/ultimo-export")"
    aviso "si despliegan desde GitHub Actions: fijar la variable del repositorio CREAR_BD=false"
    ;;
  recrear)
    origen="${1:?uso: bd.sh recrear <gs://...sql.gz | archivo local>}"
    if [[ "$origen" != gs://* ]]; then
      [[ -f "$origen" ]] || morir "no existe $origen"
      aviso "subiendo $origen al bucket de respaldos"
      gc storage cp "$origen" "gs://$RESPALDOS/exports/"
      origen="gs://$RESPALDOS/exports/$(basename "$origen")"
    fi
    fijar_crear_bd true
    tf apply -input=false
    releer_salidas
    BD="$(salida bd_instancia)"
    requiere_bd
    # Antes de desplegar la API: si arrancara contra la base vacía, migraría
    # y la importación chocaría con las tablas ya creadas.
    aviso "importando $origen en $BD"
    gc sql import sql "$BD" "$origen" --database mooc --user mooc --quiet
    aviso "recreada ($BD, IP $(salida bd_ip_privada)). La IP privada cambió: regenerar los .env y redesplegar:"
    aviso "  deploy/gcp/generar-env.sh -f && deploy/gcp/desplegar-vm.sh worker && deploy/gcp/desplegar-vm.sh web"
    aviso "  (desde GitHub Actions: CREAR_BD=true o borrarla, y la acción 'desplegar')"
    ;;
  detener)
    requiere_bd
    # Cloud SQL detenida no se vuelve a encender sola; sigue
    # cobrando almacenamiento y backups.
    gc sql instances patch "$BD" --activation-policy NEVER --quiet
    aviso "$BD detenida"
    ;;
  iniciar)
    requiere_bd
    gc sql instances patch "$BD" --activation-policy ALWAYS --quiet
    aviso "$BD en marcha"
    ;;
  *)
    morir "acción desconocida '$accion'"
    ;;
esac

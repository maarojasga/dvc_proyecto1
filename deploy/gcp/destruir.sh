#!/usr/bin/env bash
# Elimina el entorno completo. Pensado para después de entregar.
#
#   destruir.sh --confirmar [--respaldar-buckets DIR]
#
# Qué se conserva:
# - La base: export SQL descargado en deploy/gcp/respaldos/ (bd.sh exportar
#   --descargar) antes de borrar nada. Los backups automáticos de Cloud SQL
#   se borran con la instancia; el bucket de respaldos, con la infraestructura.
# - Los objetos: con --respaldar-buckets se copian antes a DIR (originales,
#   PDFs, subtítulos, insignias y derivados HLS). Sin esa opción se pierden.
# - La clave de firma de insignias: vive fuera de Terraform y se queda en
#   Secret Manager (costo prácticamente nulo) para que las credenciales ya
#   emitidas sigan verificándose en el entorno recreado.
#
# Para reconstruir: desplegar-infra.sh, bd.sh recrear <export local>,
# publicar.sh, generar-env.sh -f, desplegar-vm.sh worker y web, y subir los
# objetos respaldados con `gcloud storage rsync`.
source "$(dirname "$0")/lib.sh"
requiere terraform
comprobar_gcloud
requiere_infra

confirmar=false
respaldo=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --confirmar) confirmar=true ;;
    --respaldar-buckets) respaldo="${2:?falta el directorio}"; shift ;;
    *) morir "opción desconocida '$1'" ;;
  esac
  shift
done
[[ "$confirmar" == true ]] || morir "esto borra toda la infraestructura del proyecto $(salida proyecto); repite con --confirmar"

if [[ -n "$(salida bd_instancia)" ]]; then
  "$DIR_GCP/bd.sh" exportar --descargar
fi

if [[ -n "$respaldo" ]]; then
  mkdir -p "$respaldo/objetos" "$respaldo/hls"
  aviso "copiando gs://$(salida bucket_objetos) y gs://$(salida bucket_hls) a $respaldo"
  gc storage rsync --recursive "gs://$(salida bucket_objetos)" "$respaldo/objetos"
  gc storage rsync --recursive "gs://$(salida bucket_hls)" "$respaldo/hls"
fi

# Igual que en bd.sh eliminar: las protecciones se bajan con un apply antes
# del destroy, porque Terraform las comprueba con el valor del estado.
VARS=(-var permitir_borrar_buckets=true -var bd_proteccion_terraform=false -var bd_proteccion_api=false)
aviso "bajando protecciones (buckets con objetos, instancia de Cloud SQL)"
tf apply -input=false "${VARS[@]}"
aviso "destruyendo la infraestructura"
tf destroy -input=false "${VARS[@]}"
rm -f "${DIR_TF:?}/bd.auto.tfvars"
aviso "infraestructura eliminada. Respaldos locales en deploy/gcp/respaldos/${respaldo:+ y $respaldo}"

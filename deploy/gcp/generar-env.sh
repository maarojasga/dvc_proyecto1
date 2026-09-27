#!/usr/bin/env bash
# Genera deploy/web/.env y deploy/worker/.env a partir de los .env.example y
# de las salidas de Terraform, y comprueba que los secretos que nombran
# existen en Secret Manager con una versión activa.
#
#   deploy/gcp/generar-env.sh [-f]     # -f sobrescribe los existentes
#
# Parámetros que se toman del entorno si están definidos (en CI, de las
# variables del repositorio), porque en CI el .env se regenera en cada
# despliegue y no hay dónde editarlo a mano: TLS_EMAIL, ADMIN_EMAIL,
# AUTH_RATE_LIMIT_PER_MINUTE (Web) y WORKER_CONCURRENCY (Worker). Vacíos, se
# queda el valor del .env.example.
#
# Los .env no llevan secretos, solo sus nombres: los valores los lee
# en-vm.sh en la VM, con la cuenta de servicio de esa VM, al desplegar. Así
# no pasan por el portátil de nadie ni quedan en disco en la VM.
source "$(dirname "$0")/lib.sh"
requiere terraform
comprobar_gcloud
requiere_infra

forzar=false
[[ "${1:-}" == "-f" ]] && forzar=true

[[ -n "$(salida bd_ip_privada)" ]] || aviso "sin instancia de Cloud SQL (crear_bd=false): DB_HOST quedará vacío"

comprobar_secreto() {
  gc secrets versions describe latest --secret "$1" --format 'value(state)' 2>/dev/null | grep -q ENABLED \
    || morir "el secreto $1 no tiene una versión activa en Secret Manager (¿desplegar-infra.sh terminó?)"
}
for s in "$(salida secreto_bd_password)" "$(salida secreto_hmac_web)" "$(salida secreto_hmac_worker)" \
         "$(salida secreto_admin_password)" "$(salida secreto_badge)"; do
  comprobar_secreto "$s"
done

# poner CLAVE valor archivo: reemplaza la línea CLAVE=... del archivo.
poner() {
  local clave="$1" valor="$2" archivo="$3"
  grep -q "^$clave=" "$archivo" || morir "$archivo no tiene la clave $clave"
  # | como separador: ningún valor de aquí lo contiene (IPs, hosts, URLs).
  sed -i.bak "s|^$clave=.*|$clave=$valor|" "$archivo"
  rm -f "${archivo:?}.bak"
}

# poner_si CLAVE valor archivo: como poner, solo si hay valor.
poner_si() {
  [[ -z "$2" ]] || poner "$@"
}

# Son parámetros del experimento de capacidad: un valor mal escrito haría
# arrancar el servicio con su valor por defecto sin avisar.
for n in WORKER_CONCURRENCY AUTH_RATE_LIMIT_PER_MINUTE; do
  [[ -z "${!n:-}" || "${!n}" =~ ^[1-9][0-9]*$ ]] || morir "$n tiene que ser un entero positivo (es '${!n}')"
done

generar() {
  local rol="$1" destino="$RAIZ/deploy/$1/.env"
  if [[ -f "$destino" && "$forzar" != true ]]; then
    aviso "$destino ya existe; se deja como está (usar -f para regenerarlo)"
    return
  fi
  cp "$RAIZ/deploy/$rol/.env.example" "$destino"
  chmod 600 "$destino"
  poner PROYECTO_GCP "$(salida proyecto)" "$destino"
  poner REGISTRO "$(salida registro)" "$destino"
  poner DB_HOST "$(salida bd_ip_privada)" "$destino"
  poner DB_USER "$(salida bd_usuario)" "$destino"
  poner DB_NAME "$(salida bd_nombre)" "$destino"
  poner SECRETO_DB_PASSWORD "$(salida secreto_bd_password)" "$destino"
  poner S3_BUCKET "$(salida bucket_objetos)" "$destino"
  poner S3_BUCKET_HLS "$(salida bucket_hls)" "$destino"
  if [[ "$rol" == web ]]; then
    poner S3_ACCESS_KEY "$(salida hmac_web_access_id)" "$destino"
    poner SECRETO_S3_SECRET_KEY "$(salida secreto_hmac_web)" "$destino"
    poner SECRETO_ADMIN_PASSWORD "$(salida secreto_admin_password)" "$destino"
    poner SECRETO_BADGE_SIGNING_KEY "$(salida secreto_badge)" "$destino"
    poner REDIS_ADDR "$(salida ip_worker):6379" "$destino"
    poner PUBLIC_BASE_URL "$(salida url_publica)" "$destino"
    poner TLS_HOST "$(salida host_web)" "$destino"
    poner TLS_EMAIL "${TLS_EMAIL:-}" "$destino"
    poner_si ADMIN_EMAIL "${ADMIN_EMAIL:-}" "$destino"
    poner_si AUTH_RATE_LIMIT_PER_MINUTE "${AUTH_RATE_LIMIT_PER_MINUTE:-}" "$destino"
  else
    poner S3_ACCESS_KEY "$(salida hmac_worker_access_id)" "$destino"
    poner SECRETO_S3_SECRET_KEY "$(salida secreto_hmac_worker)" "$destino"
    poner REDIS_BIND_IP "$(salida ip_worker)" "$destino"
    poner_si WORKER_CONCURRENCY "${WORKER_CONCURRENCY:-}" "$destino"
  fi
  aviso "generado $destino"
}

generar web
generar worker

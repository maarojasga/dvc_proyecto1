#!/usr/bin/env bash
# Imprime en Markdown la configuración efectiva del entorno: lo que de verdad
# está corriendo, leído de GCP y de las VM, no lo que dicen las variables.
# Es la evidencia que el informe de capacidad pide registrar en cada corrida
# (tipo de VM, tier de la base, concurrencia de los workers, commit).
#
#   deploy/gcp/resumen.sh                   # a la terminal
#   deploy/gcp/resumen.sh >> "$GITHUB_STEP_SUMMARY"
#
# Lee las VM por IAP solo si están encendidas. No imprime secretos: de los
# .env de las VM solo toma WORKER_CONCURRENCY y AUTH_RATE_LIMIT_PER_MINUTE.
source "$(dirname "$0")/lib.sh"
requiere terraform curl
comprobar_gcloud
requiere_infra

ZONA="$(salida zona)"
URL="$(salida url_publica)"
BD="$(salida bd_instancia)"

# vm <nombre>: tipo, estado y disco de arranque (se llama igual que la VM).
vm() {
  local datos disco
  datos="$(gc compute instances describe "$1" --zone "$ZONA" \
             --format 'value(machineType.basename(),status)' 2>/dev/null || true)"
  disco="$(gc compute disks describe "$1" --zone "$ZONA" \
             --format 'value(sizeGb,type.basename())' 2>/dev/null || true)"
  [[ -n "$datos" ]] || { echo "no existe"; return; }
  # value() separa con tabuladores.
  printf '%s, %s, disco %s GB %s' "${datos%%$'\t'*}" "${datos##*$'\t'}" "${disco%%$'\t'*}" "${disco##*$'\t'}"
}

encendida() {
  [[ "$(gc compute instances describe "$(vm_de "$1")" --zone "$ZONA" --format 'value(status)' 2>/dev/null)" == RUNNING ]]
}

# en_vm <rol> <comando>: salida del comando en la VM, o vacío si no se pudo.
en_vm() {
  encendida "$1" || return 0
  ssh_vm "$1" "$2" 2>/dev/null || true
}

version_web="$(en_vm web 'cat /opt/mooc/actual/VERSION 2>/dev/null')"
version_worker="$(en_vm worker 'cat /opt/mooc/actual/VERSION 2>/dev/null')"
# Solo esas dos claves: el resto del .env no es secreto, pero tampoco hace
# falta en un resumen que puede quedar público.
params_worker="$(en_vm worker "sudo grep -h '^WORKER_CONCURRENCY=' /opt/mooc/config/worker.env 2>/dev/null")"
params_web="$(en_vm web "sudo grep -h '^AUTH_RATE_LIMIT_PER_MINUTE=' /opt/mooc/config/web.env 2>/dev/null")"
concurrencia="${params_worker#WORKER_CONCURRENCY=}"
if [[ -z "$params_web" ]]; then
  limite="? (Web detenido o sin desplegar)"
else
  limite="${params_web#AUTH_RATE_LIMIT_PER_MINUTE=}"
  limite="${limite:-10 (valor por defecto)}"
fi

if [[ -n "$BD" ]]; then
  bd="$(gc sql instances describe "$BD" \
          --format 'value(settings.tier,settings.edition,state,settings.activationPolicy,settings.dataDiskSizeGb)' 2>/dev/null \
        | tr '\t' ' ' || true)"
  bd="$BD: ${bd:-sin datos}"
else
  bd="sin instancia (crear_bd=false)"
fi

# Salud: primero con verificación de certificado; si solo responde sin
# verificar, el certificado es el autofirmado provisional.
salud="sin respuesta"
if encendida web; then
  if codigo="$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 "$URL/api/v1/health")" && [[ "$codigo" != 000 ]]; then
    salud="HTTP $codigo, certificado válido"
  elif codigo="$(curl -sk -o /dev/null -w '%{http_code}' --max-time 10 "$URL/api/v1/health")" && [[ "$codigo" != 000 ]]; then
    salud="HTTP $codigo, certificado autofirmado (falta deploy/web/certificado.sh letsencrypt)"
  fi
else
  salud="Web Server detenido"
fi

cat <<EOF
### Configuración efectiva en GCP

| | |
|---|---|
| URL pública | $URL |
| Salud de la API | $salud |
| Commit desplegado | Web: \`${version_web:-?}\` · Worker: \`${version_worker:-?}\` |
| Proyecto, región y zona | $(salida proyecto), $(salida region), $ZONA |
| Web Server | $(vm "$(salida vm_web)") |
| Worker Server | $(vm "$(salida vm_worker)") |
| Cloud SQL (tier, edición, estado, activación, GB) | $bd |
| WORKER_CONCURRENCY | ${concurrencia:-? (Worker detenido o sin desplegar)} |
| AUTH_RATE_LIMIT_PER_MINUTE | $limite |
| Cloud NAT | $(salida nat_activa) |
| Registro de imágenes | $(salida registro) |

Leído el $(TZ=America/Bogota date '+%Y-%m-%d %H:%M') hora de Colombia ($(date -u '+%H:%M') UTC).
EOF

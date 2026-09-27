# shellcheck shell=bash
# Carga un .env de despliegue en el entorno del script que lo incluye. Se usa
# con `source`, EN la VM (en-vm.sh y deploy/web/certificado.sh).
#
# No se hace `source` del .env: ese archivo está escrito para
# `docker compose --env-file`, que toma cada línea CLAVE=valor al pie de la
# letra, y bash no. `ISSUER_NAME=Plataforma MOOC` en bash es la orden `MOOC`
# con ISSUER_NAME en su entorno, y bajo `set -e` el script muere con 127
# antes de desplegar nada. Aquí se lee igual que lo lee Compose: la clave
# hasta el primer "=", el resto como valor literal, y las comillas que lo
# envuelvan, si las hay, se quitan.
cargar_env() {
  local archivo="$1" linea clave valor
  while IFS= read -r linea || [[ -n "$linea" ]]; do
    linea="${linea%$'\r'}"
    [[ "$linea" =~ ^[[:space:]]*(#|$) ]] && continue
    [[ "$linea" == *=* ]] || continue
    clave="${linea%%=*}"
    clave="${clave#export }"
    clave="${clave//[[:space:]]/}"
    [[ "$clave" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || { echo "línea inválida en $archivo: $linea" >&2; return 1; }
    valor="${linea#*=}"
    if [[ "$valor" =~ ^\"(.*)\"$ || "$valor" =~ ^\'(.*)\'$ ]]; then
      valor="${BASH_REMATCH[1]}"
    fi
    export "$clave=$valor"
  done < "$archivo"
}

#!/usr/bin/env bash
# Certificado TLS del Web Server. Se ejecuta EN la máquina.
#
#   certificado.sh asegurar      # autofirmado solo si no hay ninguno (en-vm.sh)
#   certificado.sh autofirmado   # fuerza uno autofirmado nuevo
#   certificado.sh letsencrypt   # emite uno real para TLS_HOST (reto HTTP-01)
#   certificado.sh renovar       # renueva si toca y recarga nginx
#
# Con dominio propio o con <IP>.sslip.io, Let's Encrypt funciona: solo pide
# que el nombre resuelva a esta máquina y que el puerto 80 responda. El
# autofirmado es el respaldo cuando no hay nombre (o se agotó el límite de
# emisiones): cifra igual, pero el navegador lo marca como no confiable y los
# clientes de prueba necesitan ignorar la verificación (k6:
# insecureSkipTLSVerify, curl -k). Así debe constar en el informe.
#
# Los dos quedan en /etc/letsencrypt/live/mooc/ dentro del volumen, que es la
# ruta fija que lee deploy/nginx/web-tls.conf.
set -euo pipefail

accion="${1:?uso: certificado.sh <asegurar|autofirmado|letsencrypt|renovar>}"
DIR="$(cd "$(dirname "$0")/../.." && pwd)"
CONFIG="${MOOC_CONFIG:-/opt/mooc/config/web.env}"
[[ -f "$CONFIG" ]] || { echo "falta $CONFIG" >&2; exit 1; }
# shellcheck source=deploy/gcp/env.sh
source "$DIR/deploy/gcp/env.sh"
cargar_env "$CONFIG"
HOST="${TLS_HOST:?falta TLS_HOST en $CONFIG}"

compose() { docker compose -f "$DIR/deploy/web/docker-compose.yml" --env-file "$CONFIG" "$@"; }
# Shell dentro del contenedor de certbot, que monta el volumen con escritura.
en_certbot() { compose run --rm --no-deps --entrypoint sh certbot -c "$1"; }

hay_certificado() { en_certbot 'test -s /etc/letsencrypt/live/mooc/fullchain.pem' >/dev/null 2>&1; }

autofirmado() {
  local tmp
  tmp="$(mktemp -d)"
  # 30 días: es provisional, y que caduque pronto evita que se quede como
  # definitivo por olvido.
  openssl req -x509 -nodes -newkey rsa:2048 -days 30 \
    -keyout "$tmp/privkey.pem" -out "$tmp/fullchain.pem" \
    -subj "/CN=$HOST" -addext "subjectAltName=DNS:$HOST" 2>/dev/null
  # La marca distingue este certificado de uno de certbot, para poder
  # borrarlo sin tocar nunca uno real.
  touch "$tmp/AUTOFIRMADO"
  chmod 644 "$tmp"/*
  compose run --rm --no-deps -v "$tmp:/origen:ro" --entrypoint sh certbot -c \
    'rm -rf /etc/letsencrypt/live/mooc && mkdir -p /etc/letsencrypt/live/mooc && cp /origen/* /etc/letsencrypt/live/mooc/'
  rm -rf "$tmp"
  echo "certificado autofirmado para $HOST"
}

recargar_nginx() {
  if [[ -n "$(compose ps -q nginx 2>/dev/null)" ]]; then
    compose exec nginx nginx -s reload
  fi
}

case "$accion" in
  asegurar)
    hay_certificado || autofirmado
    ;;
  autofirmado)
    autofirmado
    recargar_nginx
    ;;
  letsencrypt)
    # nginx tiene que estar sirviendo el reto por el puerto 80, y para
    # arrancar necesita algún certificado.
    hay_certificado || autofirmado
    # --no-deps: esto corre sin los secretos ni las imágenes del despliegue
    # (los pone en-vm.sh), y si Compose revisara también la API la
    # recrearía con DB_PASSWORD vacío y una imagen inexistente. nginx
    # arranca sin la API porque la resuelve en cada petición.
    compose up -d --no-deps nginx
    # Solo se retira el autofirmado; uno real existente lo gestiona certbot.
    en_certbot '[ -f /etc/letsencrypt/live/mooc/AUTOFIRMADO ] && rm -rf /etc/letsencrypt/live/mooc || true'
    correo=(--register-unsafely-without-email)
    [[ -n "${TLS_EMAIL:-}" ]] && correo=(--email "$TLS_EMAIL")
    if ! compose run --rm --no-deps certbot certonly --webroot -w /var/www/certbot \
         -d "$HOST" --cert-name mooc --agree-tos --no-eff-email --non-interactive "${correo[@]}"; then
      echo "Let's Encrypt falló; se vuelve al autofirmado para que nginx siga sirviendo" >&2
      hay_certificado || autofirmado
      recargar_nginx
      exit 1
    fi
    recargar_nginx
    echo "certificado de Let's Encrypt para $HOST"
    ;;
  renovar)
    # Los certificados duran 90 días, más que la entrega; esto existe para la
    # sustentación tardía o un entorno que se quede más tiempo.
    compose run --rm --no-deps certbot renew --webroot -w /var/www/certbot
    recargar_nginx
    ;;
  *)
    echo "acción desconocida '$accion'" >&2
    exit 1
    ;;
esac

# shellcheck shell=bash
# Funciones comunes de los scripts de deploy/aws. Se carga con `source`; no se
# ejecuta. Lee deploy/aws/parametros.env (ignorado por git) si existe.

set -euo pipefail

RAIZ="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DIR_AWS="$RAIZ/deploy/aws"
PARAMETROS="${PARAMETROS:-$DIR_AWS/parametros.env}"
if [[ -f "$PARAMETROS" ]]; then
  # shellcheck disable=SC1090
  source "$PARAMETROS"
fi

PILA="${PILA:-mooc}"
REGION="${AWS_REGION:-${AWS_DEFAULT_REGION:-us-east-1}}"
# Sin paginador: los scripts leen la salida, no una persona en un less.
export AWS_PAGER=""
# Estado local entre scripts (último tag publicado, último snapshot).
# shellcheck disable=SC2034
ESTADO="$DIR_AWS/.estado"

morir() { echo "error: $*" >&2; exit 1; }
aviso() { echo ">> $*" >&2; }

aws_() { aws --region "$REGION" "$@"; }

requiere() {
  local c
  for c in "$@"; do
    command -v "$c" >/dev/null 2>&1 || morir "falta el comando '$c'"
  done
}

# Falla pronto y con un mensaje claro si las credenciales no sirven. En
# Learner Lab caducan a las ~4 h: el síntoma habitual es ExpiredToken aquí.
comprobar_credenciales() {
  requiere aws
  aws_ sts get-caller-identity --query Arn --output text >/dev/null \
    || morir "credenciales de AWS inválidas o caducadas (en Learner Lab: AWS Details > AWS CLI, y pegarlas en ~/.aws/credentials)"
}

# salida <Clave>: valor de una salida de la pila, vacío si no existe.
salida() {
  local v
  v="$(aws_ cloudformation describe-stacks --stack-name "$PILA" \
        --query "Stacks[0].Outputs[?OutputKey=='$1'].OutputValue | [0]" --output text)"
  [[ "$v" == "None" ]] && v=""
  printf '%s' "$v"
}

instancia_de() {
  case "$1" in
    web) salida WebInstanceId ;;
    worker) salida WorkerInstanceId ;;
    *) morir "rol desconocido '$1' (web|worker)" ;;
  esac
}

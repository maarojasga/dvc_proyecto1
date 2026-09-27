#!/usr/bin/env bash
# Muestra las salidas de la pila: IP y URL del Web Server, IP privada del
# Worker, endpoint de RDS, bucket.
#
#   deploy/aws/salidas.sh           # tabla
#   deploy/aws/salidas.sh --env     # CLAVE=valor, para `source` o scripts
source "$(dirname "$0")/lib.sh"
comprobar_credenciales

if [[ "${1:-}" == "--env" ]]; then
  aws_ cloudformation describe-stacks --stack-name "$PILA" \
    --query 'Stacks[0].Outputs[].[OutputKey,OutputValue]' --output text \
    | awk -F'\t' '{printf "%s=%s\n", $1, $2}'
else
  aws_ cloudformation describe-stacks --stack-name "$PILA" \
    --query 'Stacks[0].Outputs[].[OutputKey,OutputValue]' --output table
fi

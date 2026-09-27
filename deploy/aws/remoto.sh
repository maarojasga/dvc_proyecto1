#!/usr/bin/env bash
# Ejecuta un script bash en el Web o el Worker Server por SSM Run Command, sin
# SSH ni puertos abiertos, y muestra su salida.
#
#   echo 'docker ps' | deploy/aws/remoto.sh web
#   deploy/aws/remoto.sh worker < mi-script.sh
#
# Corre como root. El Worker solo responde con la NAT encendida: el agente de
# SSM necesita salir a los endpoints de Systems Manager. SSM recorta la salida
# a 24 000 caracteres; para registros largos, `docker compose logs` a un
# archivo y copiarlo aparte.
source "$(dirname "$0")/lib.sh"
comprobar_credenciales

rol="${1:?uso: remoto.sh <web|worker> < script}"
instancia="$(instancia_de "$rol")"
[[ -n "$instancia" ]] || morir "la pila no tiene instancia '$rol'"

# El script viaja en base64 dentro de un JSON: así no hay que escapar
# comillas, saltos de línea ni caracteres especiales.
b64="$(base64 | tr -d '\n')"
json="$(mktemp)"
trap 'rm -f "$json"' EXIT
printf '{"commands":["echo %s | base64 -d | bash"],"executionTimeout":["3600"]}' "$b64" > "$json"

id="$(aws_ ssm send-command --instance-ids "$instancia" \
        --document-name AWS-RunShellScript \
        --comment "mooc: remoto.sh $rol" \
        --parameters "file://$json" \
        --query Command.CommandId --output text)"
aviso "comando $id en $rol ($instancia)"

estado=Pending
while [[ "$estado" =~ ^(Pending|InProgress|Delayed)$ ]]; do
  sleep 5
  estado="$(aws_ ssm get-command-invocation --command-id "$id" --instance-id "$instancia" \
              --query Status --output text 2>/dev/null || echo Pending)"
done

aws_ ssm get-command-invocation --command-id "$id" --instance-id "$instancia" \
  --query StandardOutputContent --output text
err="$(aws_ ssm get-command-invocation --command-id "$id" --instance-id "$instancia" \
         --query StandardErrorContent --output text)"
[[ -n "$err" ]] && printf '%s\n' "$err" >&2
[[ "$estado" == Success ]] || morir "el comando terminó en estado $estado"

#!/usr/bin/env bash
# Genera deploy/web/.env y deploy/worker/.env a partir de los .env.example y
# de las salidas de la pila. No contienen secretos (la contraseña de la base
# se lee de SSM en la máquina), así que desplegar-vm.sh puede enviarlos.
#
#   deploy/aws/generar-env.sh [-f]     # -f sobrescribe los existentes
source "$(dirname "$0")/lib.sh"
comprobar_credenciales

forzar=false
[[ "${1:-}" == "-f" ]] && forzar=true

# Sin arrays asociativos: el bash de macOS es 3.2 y no los tiene.
SALIDAS="$("$DIR_AWS/salidas.sh" --env)"
s() { printf '%s\n' "$SALIDAS" | sed -n "s/^$1=//p"; }

[[ -n "$(s BdEndpoint)" ]] || aviso "la pila no tiene base de datos (CrearBaseDeDatos=false): DB_HOST quedará sin valor"

# poner CLAVE valor archivo: reemplaza la línea CLAVE=... del archivo.
poner() {
  local clave="$1" valor="$2" archivo="$3"
  grep -q "^$clave=" "$archivo" || morir "$archivo no tiene la clave $clave"
  # | como separador: ningún valor de aquí lo contiene (IPs, hosts, URLs).
  sed -i.bak "s|^$clave=.*|$clave=$valor|" "$archivo" && rm -f "$archivo.bak"
}

generar() {
  local rol="$1" destino="$RAIZ/deploy/$1/.env"
  if [[ -f "$destino" && "$forzar" != true ]]; then
    aviso "$destino ya existe; se deja como está (usar -f para regenerarlo)"
    return
  fi
  cp "$RAIZ/deploy/$rol/.env.example" "$destino"
  chmod 600 "$destino"
  poner DB_HOST "$(s BdEndpoint)" "$destino"
  poner DB_USER "$(s BdUsuario)" "$destino"
  poner DB_NAME "$(s BdNombre)" "$destino"
  poner DB_PASSWORD_SSM "$(s ParametroPasswordBD)" "$destino"
  poner SSM_PREFIJO "/$(s Proyecto)" "$destino"
  poner S3_ENDPOINT "$(s S3Endpoint)" "$destino"
  poner S3_REGION "$(s Region)" "$destino"
  poner S3_BUCKET "$(s NombreBucket)" "$destino"
  if [[ "$rol" == web ]]; then
    poner REDIS_ADDR "$(s WorkerIpPrivada):6379" "$destino"
    poner S3_PUBLIC_ENDPOINT "$(s S3Endpoint)" "$destino"
    poner PUBLIC_BASE_URL "$(s UrlPublica)" "$destino"
    poner TLS_HOST "$(s NombreHostWeb)" "$destino"
    poner TLS_EMAIL "${TLS_EMAIL:-}" "$destino"
  else
    poner REDIS_BIND_IP "$(s WorkerIpPrivada)" "$destino"
  fi
  aviso "generado $destino"
}

generar web
generar worker

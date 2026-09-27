#!/usr/bin/env bash
# Enciende o apaga el entorno entre sesiones de trabajo.
#
#   deploy/gcp/energia.sh detener [--con-bd]
#   deploy/gcp/energia.sh iniciar [--con-bd]
#   deploy/gcp/energia.sh estado
#
# Detener no deja el costo en cero: siguen cobrándose los discos (2 x 30 GB
# pd-balanced), la IP estática del Web (sin VM encendida pasa a la tarifa de
# IP reservada sin uso), Cloud NAT si está habilitada, el almacenamiento de
# Cloud SQL y sus backups, los buckets y Artifact Registry. Ver README.
source "$(dirname "$0")/lib.sh"
requiere terraform
comprobar_gcloud
requiere_infra

accion="${1:?uso: energia.sh <detener|iniciar|estado> [--con-bd]}"
con_bd=false
[[ "${2:-}" == "--con-bd" ]] && con_bd=true
ZONA="$(salida zona)"
BD="$(salida bd_instancia)"

case "$accion" in
  detener)
    gc compute instances stop "$(salida vm_web)" "$(salida vm_worker)" --zone "$ZONA"
    aviso "VM detenidas"
    if [[ "$con_bd" == true && -n "$BD" ]]; then
      # Cloud SQL detenida no se vuelve a encender sola, pero
      # sigue cobrando almacenamiento y backups.
      gc sql instances patch "$BD" --activation-policy NEVER --quiet
      aviso "Cloud SQL $BD detenida"
    fi
    if [[ "$(salida nat_activa)" == true ]]; then
      aviso "Cloud NAT sigue habilitada y cobrando. Para quitarla: deploy/gcp/desplegar-infra.sh -var habilitar_nat=false"
    fi
    ;;
  iniciar)
    if [[ "$con_bd" == true && -n "$BD" ]]; then
      gc sql instances patch "$BD" --activation-policy ALWAYS --quiet
      aviso "Cloud SQL $BD en marcha"
    fi
    # Primero el Worker: la API conecta con Redis al arrancar. Los
    # contenedores tienen restart: unless-stopped y Docker arranca con el
    # sistema, así que vuelven solos; las IP (estática del Web, interna fija
    # del Worker) no cambian.
    gc compute instances start "$(salida vm_worker)" --zone "$ZONA"
    gc compute instances start "$(salida vm_web)" --zone "$ZONA"
    aviso "VM en marcha. Comprobar: curl -s $(salida url_publica)/api/v1/health"
    ;;
  estado)
    gc compute instances list --filter "labels.proyecto=$(salida prefijo)" \
      --format 'table(name,zone.basename(),machineType.basename(),status,networkInterfaces[0].networkIP)'
    if [[ -n "$BD" ]]; then
      gc sql instances describe "$BD" \
        --format 'table(name,settings.tier,state,settings.activationPolicy,ipAddresses[0].ipAddress)'
    fi
    ;;
  *)
    morir "acción desconocida '$accion'"
    ;;
esac

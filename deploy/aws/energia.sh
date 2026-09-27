#!/usr/bin/env bash
# Enciende o apaga el entorno entre sesiones de trabajo.
#
#   energia.sh detener [--con-bd]
#   energia.sh iniciar [--con-bd]
#
# Detener no deja el costo en cero: siguen cobrándose los discos EBS (2 x 30
# GiB gp3), la IP elástica del Web Server, el almacenamiento de RDS y sus
# snapshots, el bucket y, sobre todo, la NAT Gateway si está encendida (no se
# puede detener; se elimina con HabilitarNatGateway=false).
source "$(dirname "$0")/lib.sh"
comprobar_credenciales

accion="${1:?uso: energia.sh <detener|iniciar> [--con-bd]}"
con_bd=false
[[ "${2:-}" == "--con-bd" ]] && con_bd=true
ids=("$(salida WebInstanceId)" "$(salida WorkerInstanceId)")

case "$accion" in
  detener)
    aws_ ec2 stop-instances --instance-ids "${ids[@]}" >/dev/null
    aws_ ec2 wait instance-stopped --instance-ids "${ids[@]}"
    aviso "instancias detenidas"
    [[ "$con_bd" == true ]] && "$DIR_AWS/rds.sh" detener
    if [[ "$(salida NatGatewayActiva)" == true ]]; then
      aviso "la NAT Gateway sigue cobrando (~0,045 USD/h). Para quitarla: deploy/aws/desplegar-infra.sh HabilitarNatGateway=false"
    fi
    ;;
  iniciar)
    [[ "$con_bd" == true ]] && "$DIR_AWS/rds.sh" iniciar
    aws_ ec2 start-instances --instance-ids "${ids[@]}" >/dev/null
    aws_ ec2 wait instance-status-ok --instance-ids "${ids[@]}"
    # Los contenedores tienen restart: unless-stopped y vuelven solos; la IP
    # del Web es elástica y la privada del Worker no cambia al detener.
    aviso "instancias en marcha"
    ;;
  *)
    morir "acción desconocida '$accion'"
    ;;
esac

#!/usr/bin/env bash
# Corre las migraciones contra Cloud SQL desde el Web Server. Cloud SQL solo
# tiene IP privada y el egress hacia ella solo se permite a las cuentas de
# servicio de las VM, así que no se puede migrar desde fuera de la VPC.
#
#   deploy/gcp/migrar.sh
#
# La API también migra al arrancar; esto existe para que un fallo de
# migración se vea en un código de salida y no enterrado en su arranque.
source "$(dirname "$0")/lib.sh"
requiere terraform
comprobar_gcloud
requiere_infra

ssh_vm web "sudo /opt/mooc/actual/deploy/gcp/en-vm.sh migrar"

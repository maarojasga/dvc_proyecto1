#!/usr/bin/env bash
# Arranque único del despliegue desde GitHub Actions. Lo corre UNA vez el
# dueño del proyecto (Cloud Shell o su máquina, con `gcloud auth login`);
# repetirlo es seguro y deja todo como lo describe este archivo.
#
#   deploy/gcp/bootstrap-ci.sh --proyecto desarrollo-soluciones-cloud \
#     [--repo maarojasga/dvc_proyecto1] [--region us-central1] [--zona us-central1-a] \
#     [--cuenta-facturacion XXXXXX-XXXXXX-XXXXXX] [--bucket <nombre>] \
#     [--rama main | --cualquier-rama] [--conservar-sa-compute]
#
# Qué deja hecho:
#   1. Las APIs: las de la federación y todas las de la plantilla, porque la
#      cuenta de CI no puede habilitar APIs.
#   2. El bucket del estado de Terraform.
#   3. Las cuentas de servicio: mooc-deployer con los roles mínimos, y las
#      de las VM (mooc-web, mooc-worker), de las que CI solo puede hacer uso.
#      Deshabilita la cuenta por defecto de Compute Engine.
#   4. Workload Identity Federation: GitHub se autentica con el token OIDC
#      de cada ejecución y no hay ninguna llave JSON que guardar ni rotar.
#   5. La clave de firma de insignias en Secret Manager.
#   6. Imprime las variables que hay que crear en GitHub.
#
# Por qué gcloud y no Terraform: esto crea el bucket donde vive el estado de
# Terraform. Hecho con Terraform, necesitaría a su vez un estado (local, en
# el portátil de alguien, o migrado al bucket que él mismo crea y del que
# depende para poder borrarse). Son una docena de recursos que no cambian y
# que se crean una vez; gcloud con comprobaciones de existencia es
# idempotente sin ese problema. Además los pools de Workload Identity
# borrados quedan 30 días en papelera con el ID reservado, y aquí se
# recuperan (undelete) en lugar de fallar al recrear.
source "$(dirname "$0")/lib.sh"
requiere gcloud
comprobar_gcloud

PROYECTO=""
REPO=""
REGION=us-central1
ZONA=us-central1-a
CUENTA_FACTURACION=""
BUCKET=""
# Por defecto solo la rama main obtiene credenciales (ver la condición).
SOLO_RAMA=main
CONSERVAR_SA_COMPUTE=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    --proyecto) PROYECTO="${2:?}"; shift ;;
    --repo) REPO="${2:?}"; shift ;;
    --region) REGION="${2:?}"; shift ;;
    --zona) ZONA="${2:?}"; shift ;;
    --cuenta-facturacion) CUENTA_FACTURACION="${2:?}"; shift ;;
    --bucket) BUCKET="${2:?}"; shift ;;
    --rama) SOLO_RAMA="${2:?}"; shift ;;
    --cualquier-rama) SOLO_RAMA="" ;;
    --conservar-sa-compute) CONSERVAR_SA_COMPUTE=true ;;
    *) morir "opción desconocida '$1'" ;;
  esac
  shift
done

PROYECTO="${PROYECTO:-$(valor_tfvars project_id)}"
PROYECTO="${PROYECTO:-$(gcloud config get-value project 2>/dev/null || true)}"
[[ -n "$PROYECTO" ]] || morir "falta --proyecto"
if [[ -z "$REPO" ]]; then
  # owner/repo del remoto origin (https o ssh).
  REPO="$(git -C "$RAIZ" remote get-url origin 2>/dev/null | sed -n 's#.*github\.com[:/]\(.*/.*\)$#\1#p' | sed 's#\.git$##')"
fi
[[ "$REPO" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || morir "no se pudo deducir el repositorio: --repo propietario/nombre"
BUCKET="${BUCKET:-$PROYECTO-tfstate}"
PREFIJO=mooc
SA_ID="$PREFIJO-deployer"
SA="$SA_ID@$PROYECTO.iam.gserviceaccount.com"
POOL=github
PROVEEDOR=github
# Los entornos de GitHub que pueden autenticarse (ver la condición abajo).
# gcp: lo que cambia la infraestructura o la aplicación; el equipo puede
# exigir aprobación. gcp-rutina: plan, estado y el apagado nocturno, que no
# pueden quedarse esperando a un revisor.
ENTORNOS=(gcp gcp-rutina)

TMP="$(mktemp -d)"
trap 'rm -rf "${TMP:?}"' EXIT

gcloud projects describe "$PROYECTO" --format 'value(projectId)' >/dev/null \
  || morir "no se encuentra el proyecto $PROYECTO o no hay acceso"
NUMERO="$(gcloud projects describe "$PROYECTO" --format 'value(projectNumber)')"
aviso "proyecto $PROYECTO ($NUMERO), repositorio $REPO"

# --- 1. APIs -------------------------------------------------------------------
# iamcredentials: generar tokens de la cuenta de servicio (impersonación).
# sts: cambiar el token OIDC de GitHub por uno federado de Google.
# iam y cloudresourcemanager: el pool, la cuenta de servicio y sus permisos.
# serviceusage y storage: habilitar APIs y el bucket del estado.
habilitar_apis "$PROYECTO" iamcredentials.googleapis.com sts.googleapis.com "${APIS[@]}"

# --- 2. Bucket del estado de Terraform ------------------------------------------
#
# El estado guarda EN CLARO la contraseña de la base, la del administrador y
# los secretos HMAC (Terraform los genera), y los planes guardados de CI
# (planes/) también los llevan. Por eso: acceso uniforme (sin ACL por
# objeto), prevención de acceso público forzada, versionado (un apply roto
# se puede deshacer recuperando la versión anterior) y una política IAM
# fijada entera, sin los permisos heredados que GCP da por defecto a los
# lectores y editores del proyecto sobre cada bucket nuevo. Límite: un rol de
# proyecto con permisos de Storage (roles/editor, roles/storage.admin) sigue
# leyendo todos los buckets; no se debe dar a quien no deba ver secretos.
if ! gcloud storage buckets describe "gs://$BUCKET" --format 'value(name)' >/dev/null 2>"$TMP/error"; then
  if grep -qiE '404|not found' "$TMP/error"; then
    aviso "creando gs://$BUCKET"
    gcloud storage buckets create "gs://$BUCKET" --project "$PROYECTO" --location "$REGION" \
      --uniform-bucket-level-access --public-access-prevention
  else
    # Los nombres de bucket son globales: otro proyecto puede tenerlo.
    morir "gs://$BUCKET existe pero no es accesible (¿de otro proyecto?): usar --bucket con otro nombre. $(cat "$TMP/error")"
  fi
fi

# Las versiones antiguas del estado se conservan hasta 30 más nuevas; los
# planes de CI, una semana (se borran solos si nadie los aplica).
cat > "$TMP/ciclo.json" <<'EOF'
{
  "rule": [
    {"action": {"type": "Delete"}, "condition": {"isLive": false, "numNewerVersions": 30}},
    {"action": {"type": "Delete"}, "condition": {"age": 7, "matchesPrefix": ["planes/"]}},
    {"action": {"type": "Delete"}, "condition": {"isLive": false, "daysSinceNoncurrentTime": 7, "matchesPrefix": ["planes/"]}}
  ]
}
EOF
gcloud storage buckets update "gs://$BUCKET" --versioning --uniform-bucket-level-access \
  --public-access-prevention --lifecycle-file "$TMP/ciclo.json" \
  --update-labels "proyecto=$PREFIJO,uso=tfstate" >/dev/null

# --- 3. Cuenta de servicio de despliegue ----------------------------------------
if ! gcloud iam service-accounts describe "$SA" --project "$PROYECTO" >/dev/null 2>&1; then
  aviso "creando la cuenta de servicio $SA"
  gcloud iam service-accounts create "$SA_ID" --project "$PROYECTO" \
    --display-name "MOOC deployer (GitHub Actions)" \
    --description "Despliegue desde GitHub Actions por Workload Identity Federation. Sin llaves."
  # IAM tarda unos segundos en ver una cuenta nueva: dar roles antes falla
  # con "does not exist".
  for _ in $(seq 1 20); do
    gcloud iam service-accounts describe "$SA" --project "$PROYECTO" >/dev/null 2>&1 && break
    sleep 3
  done
fi

# Política del bucket, entera (set-iam-policy sin etag la reemplaza): dueños
# del proyecto y la cuenta de despliegue. Nada de projectViewer ni
# projectEditor.
cat > "$TMP/politica-bucket.json" <<EOF
{
  "bindings": [
    {"role": "roles/storage.legacyBucketOwner", "members": ["projectOwner:$PROYECTO"]},
    {"role": "roles/storage.legacyObjectOwner", "members": ["projectOwner:$PROYECTO"]},
    {"role": "roles/storage.objectAdmin", "members": ["serviceAccount:$SA"]}
  ]
}
EOF
gcloud storage buckets set-iam-policy "gs://$BUCKET" "$TMP/politica-bucket.json" >/dev/null

# Roles de proyecto. Cada uno cubre recursos concretos de terraform/*.tf o
# un paso de los scripts; ninguno es roles/owner ni roles/editor.
ROLES=(
  # computo.tf: las dos VM y sus discos; energia.sh las detiene y enciende.
  # Incluye usar subredes y direcciones al crear la VM y compute.instances.get
  # que necesita `gcloud compute ssh`.
  roles/compute.instanceAdmin.v1
  # main.tf y basedatos.tf: VPC, subredes, Cloud Router y NAT, direcciones
  # (IP estática del Web, interna del Worker, rango global de PSA) y el
  # peering de la red. No incluye reglas de firewall.
  roles/compute.networkAdmin
  # main.tf: las reglas de firewall (networkAdmin las excluye a propósito).
  roles/compute.securityAdmin
  # basedatos.tf: google_service_networking_connection (Private Service
  # Access de Cloud SQL).
  roles/servicenetworking.networksAdmin
  # basedatos.tf: instancia, base y usuario; energia.sh y bd.sh la detienen,
  # la encienden, la exportan y la importan.
  roles/cloudsql.admin
  # almacenamiento.tf: buckets con CORS, ciclo de vida y su IAM (incluido el
  # allUsers de solo lectura del bucket hls). También el bucket del estado.
  roles/storage.admin
  # identidades.tf: las claves HMAC de las dos cuentas de servicio
  # (storage.admin no incluye storage.hmacKeys.*).
  roles/storage.hmacKeyAdmin
  # identidades.tf: secretos, sus versiones y quién los lee. Terraform lee el
  # valor de las versiones al refrescar, y generar-env.sh comprueba que
  # tengan una activa; la clave de insignias se crea si falta.
  roles/secretmanager.admin
  # identidades.tf: el repositorio Docker, su política de limpieza y su IAM;
  # publicar.sh sube las imágenes (incluye escribir).
  roles/artifactregistry.admin
  # user_project_override del provider (versions.tf): cada petición lleva el
  # proyecto de cuota y exige serviceusage.services.use. También lista las
  # APIs habilitadas (desplegar-infra.sh). No puede habilitar APIs.
  roles/serviceusage.serviceUsageConsumer
  # Despliegue: túnel de IAP hasta el 22 de las VM (no hay SSH público).
  roles/iap.tunnelResourceAccessor
  # Despliegue: OS Login con sudo (desplegar-vm.sh corre en-vm.sh como root).
  roles/compute.osAdminLogin
)
# Sin roles de Logging ni de Monitoring: la plantilla no crea alertas,
# paneles ni sumideros; solo da logWriter y metricWriter a las VM (abajo).
#
# Deliberadamente fuera: iam.serviceAccountAdmin (setIamPolicy sobre todas
# las cuentas del proyecto, incluida esta: podría darse tokenCreator o
# dárselo a un tercero y tener acceso fuera de GitHub) e
# iam.serviceAccountUser de proyecto (actAs sobre cualquier cuenta: una VM
# con la cuenta por defecto de Compute, que es Editor). Las cuentas de las
# VM se crean abajo y el actAs se da solo sobre ellas.
for rol in "${ROLES[@]}"; do
  gcloud projects add-iam-policy-binding "$PROYECTO" --member "serviceAccount:$SA" \
    --role "$rol" --condition None --quiet >/dev/null
  aviso "  $rol"
done

# tiene_rol_proyecto <miembro> <rol>
tiene_rol_proyecto() {
  gcloud projects get-iam-policy "$PROYECTO" --flatten 'bindings[].members' \
    --filter "bindings.role='$2' AND bindings.members='$1'" --format 'value(bindings.role)' | grep -q .
}

# Una corrida anterior de este script daba esos dos roles: se retiran.
for rol in roles/iam.serviceAccountAdmin roles/iam.serviceAccountUser; do
  if tiene_rol_proyecto "serviceAccount:$SA" "$rol"; then
    gcloud projects remove-iam-policy-binding "$PROYECTO" --member "serviceAccount:$SA" \
      --role "$rol" --all --quiet >/dev/null
    aviso "  retirado $rol de proyecto"
  fi
done

# Cuentas de las VM. Terraform las lee (data en identidades.tf) y las usa
# en las VM, el firewall, las claves HMAC y los permisos sobre buckets y
# secretos. mooc-deployer puede usarlas (actAs: crear la VM con ellas y
# entrar por OS Login) pero no cambiar su IAM ni crear otras.
for rol_vm in web worker; do
  cuenta="$PREFIJO-$rol_vm@$PROYECTO.iam.gserviceaccount.com"
  if ! gcloud iam service-accounts describe "$cuenta" --project "$PROYECTO" >/dev/null 2>&1; then
    aviso "creando la cuenta de servicio $cuenta"
    if [[ "$rol_vm" == web ]]; then
      gcloud iam service-accounts create "$PREFIJO-web" --project "$PROYECTO" \
        --display-name "MOOC Web Server (API)" \
        --description "VM Web y API: firma cargas y descargas, lee Secret Manager y Artifact Registry."
    else
      gcloud iam service-accounts create "$PREFIJO-worker" --project "$PROYECTO" \
        --display-name "MOOC Worker Server" \
        --description "VM Worker: lee originales, escribe derivados HLS y PDFs de presentaciones."
    fi
    for _ in $(seq 1 20); do
      gcloud iam service-accounts describe "$cuenta" --project "$PROYECTO" >/dev/null 2>&1 && break
      sleep 3
    done
  fi
  gcloud iam service-accounts add-iam-policy-binding "$cuenta" --project "$PROYECTO" \
    --member "serviceAccount:$SA" --role roles/iam.serviceAccountUser --condition None >/dev/null
  aviso "  roles/iam.serviceAccountUser sobre $cuenta"
done

# La cuenta por defecto de Compute Engine (<número>-compute@...) nace con
# roles/editor en proyectos sin organización. Nada de este despliegue la usa:
# las VM corren con mooc-web y mooc-worker, y Cloud SQL, Artifact Registry y
# servicenetworking trabajan con sus propios agentes de servicio
# (service-<número>@gcp-sa-*). Viva y con Editor, cualquiera con actAs sobre
# ella tendría Editor a través de una VM; se le quita el rol y se deshabilita.
SA_COMPUTE="$NUMERO-compute@developer.gserviceaccount.com"
if [[ "$CONSERVAR_SA_COMPUTE" == true ]]; then
  aviso "se conserva $SA_COMPUTE (--conservar-sa-compute)"
elif ! gcloud iam service-accounts describe "$SA_COMPUTE" --project "$PROYECTO" >/dev/null 2>&1; then
  aviso "no existe $SA_COMPUTE (o aún no aparece tras habilitar Compute): repetir el script más tarde"
else
  if tiene_rol_proyecto "serviceAccount:$SA_COMPUTE" roles/editor; then
    gcloud projects remove-iam-policy-binding "$PROYECTO" --member "serviceAccount:$SA_COMPUTE" \
      --role roles/editor --all --quiet >/dev/null
    aviso "retirado roles/editor de $SA_COMPUTE"
  fi
  en_uso="$(gcloud compute instances list --project "$PROYECTO" \
              --filter "serviceAccounts.email=$SA_COMPUTE" --format 'value(name)' 2>/dev/null || true)"
  if [[ -n "$en_uso" ]]; then
    aviso "AVISO: $SA_COMPUTE la usan estas VM y no se deshabilita: $en_uso"
  elif [[ "$(gcloud iam service-accounts describe "$SA_COMPUTE" --project "$PROYECTO" --format 'value(disabled)')" != True ]]; then
    gcloud iam service-accounts disable "$SA_COMPUTE" --project "$PROYECTO" --quiet >/dev/null
    aviso "deshabilitada $SA_COMPUTE"
  fi
fi

# identidades.tf da logWriter y metricWriter a las cuentas de las VM con
# google_project_iam_member, que exige cambiar la política IAM del proyecto.
# projectIamAdmin sin más permitiría a la cuenta de CI darse roles/owner; la
# condición limita lo que puede conceder o quitar a esos dos roles.
cat > "$TMP/condicion.yaml" <<'EOF'
title: solo-roles-de-las-vm
description: Solo concede o quita los roles de proyecto que identidades.tf da a las VM.
expression: api.getAttribute('iam.googleapis.com/modifiedGrantsByRole', []).hasOnly(['roles/logging.logWriter', 'roles/monitoring.metricWriter'])
EOF
gcloud projects add-iam-policy-binding "$PROYECTO" --member "serviceAccount:$SA" \
  --role roles/resourcemanager.projectIamAdmin --condition-from-file "$TMP/condicion.yaml" --quiet >/dev/null
aviso "  roles/resourcemanager.projectIamAdmin (solo logWriter y metricWriter)"

# Presupuesto (costos.tf): se crea en la cuenta de facturación, no en el
# proyecto, y exige billing.costsManager ahí. Solo puede darlo quien
# administra esa cuenta; con créditos educativos a menudo no es el equipo.
PRESUPUESTO_OK=false
if [[ -n "$CUENTA_FACTURACION" ]]; then
  if gcloud billing accounts add-iam-policy-binding "$CUENTA_FACTURACION" \
       --member "serviceAccount:$SA" --role roles/billing.costsManager >/dev/null 2>&1; then
    PRESUPUESTO_OK=true
    aviso "  roles/billing.costsManager en la cuenta de facturación $CUENTA_FACTURACION"
  else
    aviso "AVISO: no se pudo dar roles/billing.costsManager en $CUENTA_FACTURACION (hay que administrar la cuenta de facturación)."
    aviso "       El presupuesto no se podrá crear desde CI: dejar CUENTA_FACTURACION vacía en GitHub, crearlo en la consola"
    aviso "       y documentar la limitación en el informe."
  fi
fi

# --- 4. Workload Identity Federation ---------------------------------------------
#
# GitHub emite en cada trabajo un token OIDC firmado con el repositorio, la
# rama, el entorno y el actor. Google lo acepta solo si cumple la condición
# y lo cambia por credenciales de corta duración de mooc-deployer.
#
# La condición exige:
# - el repositorio por nombre y por ID numérico: si el repositorio se borra o
#   se renombra y alguien crea otro con el mismo nombre, el ID no coincide;
# - un entorno de GitHub (gcp o gcp-rutina). Un trabajo sin `environment:`
#   no recibe credenciales, y en los entornos el equipo decide en GitHub
#   qué ramas pueden desplegar y si hace falta aprobación;
# - la rama main (refs/heads/main, o la de --rama). Aunque alguien con
#   permiso de escritura empuje otra rama con un workflow modificado que
#   declare el entorno, no obtiene credenciales. --cualquier-rama lo quita
#   para probar desde otra rama con `gh workflow run --ref`, y deja la
#   restricción solo en las reglas de ramas de los entornos de GitHub.
REPO_ID=""
if command -v gh >/dev/null 2>&1; then
  REPO_ID="$(gh api "repos/$REPO" --jq .id 2>/dev/null || true)"
fi
if [[ -z "$REPO_ID" ]] && command -v curl >/dev/null 2>&1; then
  # El primer "id" de la respuesta, con dos espacios de sangría, es el del
  # repositorio; el del propietario va anidado.
  REPO_ID="$(curl -fsS "https://api.github.com/repos/$REPO" 2>/dev/null | sed -n 's/^  "id": \([0-9]*\),$/\1/p' | head -1 || true)"
fi
CONDICION="assertion.repository == '$REPO'"
if [[ -n "$REPO_ID" ]]; then
  CONDICION+=" && assertion.repository_id == '$REPO_ID'"
else
  aviso "AVISO: no se pudo leer el ID numérico de $REPO (¿privado?); la condición solo comprueba el nombre"
fi
entornos_cel=""
for e in "${ENTORNOS[@]}"; do
  entornos_cel+="${entornos_cel:+ || }assertion.environment == '$e'"
done
CONDICION+=" && ($entornos_cel)"
if [[ -n "$SOLO_RAMA" ]]; then
  CONDICION+=" && assertion.ref == 'refs/heads/$SOLO_RAMA'"
fi
MAPEO="google.subject=assertion.sub,attribute.repository=assertion.repository,attribute.repository_id=assertion.repository_id,attribute.ref=assertion.ref,attribute.environment=assertion.environment,attribute.actor=assertion.actor,attribute.workflow_ref=assertion.workflow_ref"

estado_pool="$(gcloud iam workload-identity-pools describe "$POOL" --project "$PROYECTO" \
                 --location global --format 'value(state)' 2>/dev/null || true)"
case "$estado_pool" in
  "")
    aviso "creando el pool de Workload Identity '$POOL'"
    gcloud iam workload-identity-pools create "$POOL" --project "$PROYECTO" --location global \
      --display-name "GitHub Actions" --description "Tokens OIDC de GitHub Actions de $REPO"
    ;;
  DELETED)
    aviso "recuperando el pool '$POOL' de la papelera"
    gcloud iam workload-identity-pools undelete "$POOL" --project "$PROYECTO" --location global
    ;;
esac

estado_proveedor="$(gcloud iam workload-identity-pools providers describe "$PROVEEDOR" --project "$PROYECTO" \
                      --location global --workload-identity-pool "$POOL" --format 'value(state)' 2>/dev/null || true)"
if [[ "$estado_proveedor" == DELETED ]]; then
  gcloud iam workload-identity-pools providers undelete "$PROVEEDOR" --project "$PROYECTO" \
    --location global --workload-identity-pool "$POOL"
fi
if [[ -z "$estado_proveedor" ]]; then
  aviso "creando el proveedor OIDC '$PROVEEDOR'"
  gcloud iam workload-identity-pools providers create-oidc "$PROVEEDOR" --project "$PROYECTO" \
    --location global --workload-identity-pool "$POOL" --display-name "GitHub OIDC" \
    --issuer-uri "https://token.actions.githubusercontent.com" \
    --attribute-mapping "$MAPEO" --attribute-condition "$CONDICION"
else
  # Si ya existía, se deja con la condición y el mapeo de ahora.
  gcloud iam workload-identity-pools providers update-oidc "$PROVEEDOR" --project "$PROYECTO" \
    --location global --workload-identity-pool "$POOL" \
    --attribute-mapping "$MAPEO" --attribute-condition "$CONDICION" >/dev/null
fi
aviso "condición del proveedor: $CONDICION"

# Solo las identidades de este repositorio (que ya pasaron la condición)
# pueden hacerse pasar por mooc-deployer.
gcloud iam service-accounts add-iam-policy-binding "$SA" --project "$PROYECTO" \
  --role roles/iam.workloadIdentityUser \
  --member "principalSet://iam.googleapis.com/projects/$NUMERO/locations/global/workloadIdentityPools/$POOL/attribute.repository/$REPO" \
  --condition None >/dev/null

# --- 5. Clave de firma de insignias ----------------------------------------------
# La necesita el primer plan (identidades.tf la lee como data source).
# asegurar_clave_insignias pone su propia limpieza de temporales; la de este
# script se restaura después.
trap - EXIT
rm -rf "${TMP:?}"
asegurar_clave_insignias "$PROYECTO" "$PREFIJO" "$REGION"

# --- 6. Variables de GitHub ---------------------------------------------------------
PROVEEDOR_COMPLETO="projects/$NUMERO/locations/global/workloadIdentityPools/$POOL/providers/$PROVEEDOR"
cat <<EOF

Listo. Crear estas variables en GitHub (Settings > Secrets and variables >
Actions > Variables > New repository variable). Ninguna es secreta: la
autenticación no usa llaves, y todo secreto de la aplicación vive en
Secret Manager.

  GCP_PROJECT_ID     = $PROYECTO
  GCP_WIF_PROVIDER   = $PROVEEDOR_COMPLETO
  GCP_DEPLOYER_SA    = $SA
  TF_STATE_BUCKET    = $BUCKET
  GCP_REGION         = $REGION
  GCP_ZONE           = $ZONA
  TLS_EMAIL          = <correo del equipo para Let's Encrypt>
  CREAR_BD           = true   (obligatoria; false tras bd.sh eliminar)
EOF
if [[ "$PRESUPUESTO_OK" == true ]]; then
  cat <<EOF
  CUENTA_FACTURACION = $CUENTA_FACTURACION
  PRESUPUESTO_MONTO  = 50
  PRESUPUESTO_MONEDA = USD   (la de la cuenta de facturación)
EOF
fi
cat <<EOF

Opcionales (vacías: el valor por defecto de variables.tf o del .env.example):
  TIPO_MAQUINA, BD_TIER, HABILITAR_NAT, DOMINIO_WEB,
  WORKER_CONCURRENCY, AUTH_RATE_LIMIT_PER_MINUTE, ADMIN_EMAIL,
  APAGADO_NOCTURNO=true (apaga VM y Cloud SQL cada noche)

Con la CLI de GitHub (gh auth login antes):

  gh variable set GCP_PROJECT_ID   -R $REPO -b '$PROYECTO'
  gh variable set GCP_WIF_PROVIDER -R $REPO -b '$PROVEEDOR_COMPLETO'
  gh variable set GCP_DEPLOYER_SA  -R $REPO -b '$SA'
  gh variable set TF_STATE_BUCKET  -R $REPO -b '$BUCKET'
  gh variable set GCP_REGION       -R $REPO -b '$REGION'
  gh variable set GCP_ZONE         -R $REPO -b '$ZONA'
  gh variable set CREAR_BD         -R $REPO -b 'true'

Después: crear los entornos gcp y gcp-rutina (Settings > Environments) y
seguir deploy/gcp/README.md, "Desplegar desde GitHub Actions".
EOF

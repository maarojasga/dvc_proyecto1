# Despliegue en Google Cloud (Entrega 2)

Infraestructura como código (Terraform, provider `google`) y scripts para
desplegar la plataforma en dos VM de Compute Engine, con Cloud SQL for
PostgreSQL 16 por IP privada, Cloud Storage por su API XML compatible con S3
y las imágenes en Artifact Registry.

```
deploy/
|-- gcp/
|   |-- terraform/          VPC, firewall, NAT, VM, Cloud SQL, buckets, IAM, secretos, presupuesto
|   |-- bootstrap-ci.sh     (una vez, el dueño) bucket del estado, cuenta de CI, Workload Identity
|   |-- desplegar-infra.sh  habilita APIs, terraform plan/apply, clave de insignias
|   |-- escribir-tfvars.sh  (CI) terraform.tfvars desde las variables del repositorio
|   |-- publicar.sh         construye y sube imágenes a Artifact Registry
|   |-- generar-env.sh      deploy/web/.env y deploy/worker/.env desde las salidas
|   |-- desplegar-vm.sh     copia deploy/ del commit por IAP y levanta el compose
|   |-- en-vm.sh            (en la VM) lee Secret Manager y ejecuta docker compose
|   |-- migrar.sh           migraciones contra Cloud SQL desde el Web Server
|   |-- remoto.sh           ssh / comandos / túneles por IAP
|   |-- energia.sh          detener e iniciar VM (y Cloud SQL)
|   |-- resumen.sh          configuración efectiva en Markdown (evidencia del informe)
|   |-- bd.sh               exportar, eliminar y recrear Cloud SQL
|   `-- destruir.sh         respaldar y eliminar todo
|-- web/                    compose del Web Server (nginx TLS, API, Mailpit, certbot)
|-- worker/                 compose del Worker Server (Redis, worker)
`-- nginx/web-tls.conf      proxy con TLS, 80 -> 443, X-Forwarded-* fijos

.github/workflows/desplegar-gcp.yml    despliegue desde GitHub Actions (manual)
.github/workflows/apagado-nocturno.yml apagado programado, opcional
.github/actions/preparar-gcp/          autenticación federada, gcloud y Terraform
```

Hay dos formas de operar, con los mismos scripts: desde GitHub Actions
(recomendada: nadie necesita credenciales en su portátil, ver
[Desplegar desde GitHub Actions](#desplegar-desde-github-actions)) o desde
un portátil ([paso a paso](#paso-a-paso-desde-un-portátil)). Las dos
comparten el estado de Terraform en un bucket.

## Arquitectura desplegada

| Componente del enunciado | Servicio de GCP | Configuración |
|---|---|---|
| Web Server | Compute Engine `e2-small` | Debian 12, pd-balanced 30 GB, Shielded VM, IP externa estática, subred `10.10.1.0/24` |
| Worker Server | Compute Engine `e2-small` | Igual, sin IP externa, IP interna fija `10.10.2.10`, subred `10.10.2.0/24` con Private Google Access |
| Red virtual privada | VPC modo custom | Dos subredes regionales, Cloud Router + Cloud NAT solo para la subred del Worker |
| Firewall | Reglas de VPC por cuenta de servicio | 80/443 públicos al Web; 6379 Web -> Worker; 22 solo desde IAP; egress 5432 al rango de Cloud SQL solo desde las dos VM |
| PostgreSQL administrado | Cloud SQL for PostgreSQL 16, Enterprise | `db-custom-1-3840`, zonal, sin réplicas, solo IP privada (Private Service Access), `ENCRYPTED_ONLY`, backups diarios x3 |
| Almacenamiento de objetos | Cloud Storage (API XML, HMAC) | Bucket privado `objetos`, bucket `hls` público de solo lectura, bucket `respaldos` |
| Cola de mensajería | Redis 7.4 en contenedor en el Worker | AOF, `noeviction`, publicado solo en la IP interna |
| Imágenes | Artifact Registry (Docker) | Se construyen fuera de las VM; pull con la cuenta de servicio |
| Secretos | Secret Manager | Contraseña de la base, secretos HMAC, contraseña del admin, clave de insignias |
| Administración | IAP TCP forwarding + OS Login | `gcloud compute ssh --tunnel-through-iap`; sin 22 abierto a Internet |
| Métricas de VM | Ops Agent + Cloud Monitoring | Memoria y disco, que GCE no reporta sin agente |
| Métricas de la base | Cloud SQL Query Insights | Latencia y carga por consulta |
| Costos | Cloud Billing Budget | Alertas al 50, 90 y 100 % (y 100 % proyectado), sin descontar créditos |

No se usan Cloud Load Balancing, Cloud CDN, Cloud Run, GKE ni Memorystore:
el enunciado los excluye o no los pide en esta etapa.

```
                      Internet
                         |  443 / 80 (solo IP estática del Web)
+------------------------|------------------------------ VPC mooc-vpc -----+
|  subred web 10.10.1.0/24                  subred worker 10.10.2.0/24     |
|  +---------------------------+   6379    +---------------------------+   |
|  | Web Server (sa mooc-web)  |---------->| Worker (sa mooc-worker)   |   |
|  | nginx TLS -> api, mailpit |           | redis, worker (FFmpeg/LO) |   |
|  +-------------+-------------+           +------+-------------+------+   |
|                | 5432 (TLS)                     | 5432        | PGA / NAT |
|        +-------v-------------------------------v---+         |           |
|        | Cloud SQL (peering servicenetworking)     |         |           |
|        | rango PSA 10.30.0.0/20, sin IP pública    |         |           |
|        +-------------------------------------------+         |           |
+--------------------------------------------------------------|-----------+
   Navegador --PUT/GET firmado--> storage.googleapis.com <------+
             --GET sin firma (hls/*)--> bucket hls
```

## Paso a paso desde un portátil

Requisitos locales: `gcloud`, `terraform` >= 1.7, `docker` con buildx, `git`,
`openssl`. En Windows, desde WSL.

1. **Proyecto y facturación.** Crear el proyecto y vincularlo a la cuenta de
   facturación de los créditos educativos (consola > Facturación). Anotar el
   ID del proyecto y, si se quiere presupuesto, el ID de la cuenta de
   facturación (`XXXXXX-XXXXXX-XXXXXX`).
2. **Sesión.**
   ```bash
   gcloud auth login
   gcloud auth application-default login   # Terraform usa estas credenciales
   gcloud config set project <proyecto>
   ```
   **Estado remoto** (una vez por proyecto): el estado de Terraform vive en
   el bucket `<proyecto>-tfstate`, que crea `bootstrap-ci.sh` junto con lo
   necesario para GitHub Actions (ver abajo). Quien solo quiera un estado
   local, sin bucket, crea `terraform/backend_override.tf` (instrucciones en
   `lib.sh`); no se debe mezclar con despliegues desde GitHub.
   ```bash
   deploy/gcp/bootstrap-ci.sh --proyecto <proyecto>
   ```
3. **Variables.**
   ```bash
   cp deploy/gcp/terraform/terraform.tfvars.example deploy/gcp/terraform/terraform.tfvars
   # editar project_id, región/zona, cuenta_facturacion
   ```
4. **Infraestructura** (habilita las APIs, crea todo y la clave de insignias;
   Cloud SQL tarda 10-15 minutos):
   ```bash
   deploy/gcp/desplegar-infra.sh
   ```
5. **Imágenes** (desde el portátil o CI, nunca en las e2-small):
   ```bash
   deploy/gcp/publicar.sh            # --frontend para incluir el frontend
   ```
6. **Configuración de cada VM:**
   ```bash
   TLS_EMAIL=equipo@ejemplo.co deploy/gcp/generar-env.sh
   # opcional: ADMIN_EMAIL=... en deploy/web/.env para el administrador inicial
   ```
7. **Despliegue**, primero el Worker (la API conecta con Redis al arrancar).
   El script comprueba que el arranque de la VM (Docker, Ops Agent) haya
   terminado; en la primera creación puede tardar 3-5 minutos.
   ```bash
   deploy/gcp/desplegar-vm.sh worker
   deploy/gcp/desplegar-vm.sh web
   deploy/gcp/migrar.sh      # la API también migra al arrancar; esto da un código de salida
   ```
   Datos sintéticos para las pruebas de carga. Cloud SQL solo es alcanzable
   desde las VM, así que el sembrador corre en el Web Server, y el escenario
   (ids y sesiones) se trae a la máquina del generador de carga:
   ```bash
   deploy/gcp/remoto.sh web 'sudo /opt/mooc/actual/deploy/gcp/en-vm.sh sembrar'
   deploy/gcp/remoto.sh web 'sudo cat /opt/mooc/salida/escenario.json' > load/salida/escenario.json
   ```
   **Las cuentas sintéticas tienen contraseña pública** (está en
   `backend/cmd/seed/main.go`, a propósito) y la URL del entorno también lo
   es. Tras las pruebas hay que neutralizarlas: ver [Cuentas
   sintéticas](#cuentas-sintéticas-después-de-las-pruebas).
8. **Certificado real** (sustituye al autofirmado provisional):
   ```bash
   deploy/gcp/remoto.sh web 'sudo /opt/mooc/actual/deploy/web/certificado.sh letsencrypt'
   curl -s "$(terraform -chdir=deploy/gcp/terraform output -raw url_publica)/api/v1/health"
   ```
9. **Tras el primer arranque**, si se usó `ADMIN_EMAIL`: la contraseña está
   en `gcloud secrets versions access latest --secret mooc-admin-password`.
   Vaciar `ADMIN_EMAIL` y volver a desplegar el Web.
10. **Opcional:** apagar Cloud NAT, que ya no hace falta (el Worker baja su
    imagen y la de Redis de Artifact Registry por Private Google Access):
    ```bash
    deploy/gcp/desplegar-infra.sh -var habilitar_nat=false
    ```
    Mejor fijarlo en `terraform.tfvars` para que el siguiente `apply` no la
    vuelva a crear. Hay que volver a encenderla para recrear el Worker.

Operación diaria:

```bash
deploy/gcp/energia.sh detener --con-bd      # fin de la sesión de trabajo
deploy/gcp/energia.sh iniciar --con-bd
deploy/gcp/remoto.sh web 'sudo /opt/mooc/actual/deploy/gcp/en-vm.sh estado'
deploy/gcp/remoto.sh worker 'sudo docker compose -f /opt/mooc/actual/deploy/worker/docker-compose.yml logs --tail 100 worker'
deploy/gcp/remoto.sh web -- -L 8025:127.0.0.1:8025 -N   # Mailpit en http://localhost:8025
deploy/gcp/desplegar-vm.sh web <tag-anterior>           # volver atrás
```

Para entrar a psql, desde el Web (Cloud SQL no es alcanzable desde fuera de
la VPC):

```bash
deploy/gcp/remoto.sh web
sudo docker run --rm -it postgres:16-alpine psql "host=<DB_HOST> user=mooc dbname=mooc sslmode=require"
```

### Permisos para el resto del equipo

Desplegando desde GitHub Actions, el resto del equipo no necesita ningún
permiso en GCP para desplegar: basta con permiso de escritura en el
repositorio (y ser revisor del entorno `gcp` si se exige aprobación).

Para entrar a las VM desde su portátil: `roles/iap.tunnelResourceAccessor`
(túnel), `roles/compute.osAdminLogin` (entrar con sudo por OS Login) y
`roles/iam.serviceAccountUser` sobre las dos cuentas de servicio de las VM
(OS Login lo exige cuando la VM corre con una), no sobre el proyecto:

```bash
for c in mooc-web mooc-worker; do
  gcloud iam service-accounts add-iam-policy-binding "$c@<proyecto>.iam.gserviceaccount.com" \
    --member user:<correo> --role roles/iam.serviceAccountUser
done
```

Los scripts leen además las
salidas del estado de Terraform, y el bucket del estado solo lo leen los
dueños del proyecto y la cuenta de CI, porque contiene secretos en claro.
Para desplegar desde un portátil hace falta, además, lo que tiene la cuenta
de CI (lista en `bootstrap-ci.sh`); en la práctica, ser dueño del proyecto.

## Desplegar desde GitHub Actions

El workflow [`desplegar-gcp.yml`](../../.github/workflows/desplegar-gcp.yml)
ejecuta los mismos scripts desde un runner de GitHub. Nadie necesita
credenciales de GCP en su portátil ni hay llaves en el repositorio:

```
job de GitHub --token OIDC firmado por GitHub (repo, rama, entorno)--> STS de Google
   |  (condición del proveedor: este repositorio, por nombre e ID, y entorno gcp o gcp-rutina)
   v
credenciales de 1 hora de mooc-deployer --> Terraform (estado en gs://<proyecto>-tfstate)
                                        --> Artifact Registry (docker push)
                                        --> IAP + OS Login --> Web / Worker (desplegar-vm.sh)
```

### Puesta en marcha (una vez)

1. **Arranque en GCP.** El dueño del proyecto, desde Cloud Shell (ya trae
   `gcloud`, `git` y `openssl`) o su máquina con `gcloud auth login`:
   ```bash
   git clone https://github.com/maarojasga/dvc_proyecto1 && cd dvc_proyecto1
   deploy/gcp/bootstrap-ci.sh --proyecto desarrollo-soluciones-cloud \
     --cuenta-facturacion XXXXXX-XXXXXX-XXXXXX   # opcional, para el presupuesto
   ```
   Confirmar antes el ID real del proyecto (`gcloud projects list`); el
   nombre visible no sirve. El script es idempotente: repetirlo deja todo
   como lo describe. Habilita las APIs, crea el bucket del estado, la cuenta
   `mooc-deployer` con sus roles, las cuentas de las VM (`mooc-web`,
   `mooc-worker`: Terraform ya no las crea, solo las lee), el pool y el
   proveedor de Workload Identity y la clave de firma de insignias, y
   deshabilita la cuenta por defecto de Compute Engine (ver
   [Permisos](#permisos-de-mooc-deployer)); al final imprime las
   variables del paso 2 (y los `gh variable set` equivalentes).

   Si el estado ya existía en un portátil, migrarlo ahora, antes de la
   primera ejecución en GitHub (ver [Estado de Terraform y
   secretos](#estado-de-terraform-y-secretos)).

2. **Variables del repositorio** (Settings > Secrets and variables > Actions
   > **Variables**, no Secrets: ninguna es secreta, y como variables se ven
   en los registros, lo que ayuda a depurar):

   | Variable | Obligatoria | Ejemplo | Para qué |
   |---|---|---|---|
   | `GCP_PROJECT_ID` | sí | `desarrollo-soluciones-cloud` | `project_id` |
   | `GCP_WIF_PROVIDER` | sí | `projects/123.../providers/github` | autenticación |
   | `GCP_DEPLOYER_SA` | sí | `mooc-deployer@<proyecto>.iam.gserviceaccount.com` | autenticación |
   | `TF_STATE_BUCKET` | sí | `<proyecto>-tfstate` | backend de Terraform |
   | `GCP_REGION`, `GCP_ZONE` | no | `us-central1`, `us-central1-a` | región y zona |
   | `TLS_EMAIL` | recomendada | correo del equipo | avisos de Let's Encrypt |
   | `CUENTA_FACTURACION` | no | `XXXXXX-XXXXXX-XXXXXX` | presupuesto (solo si el bootstrap pudo dar `billing.costsManager`) |
   | `PRESUPUESTO_MONTO`, `PRESUPUESTO_MONEDA` | no | `50`, `USD` | presupuesto |
   | `TIPO_MAQUINA`, `BD_TIER` | no | `e2-small`, `db-custom-1-3840` | configuración de la corrida |
   | `CREAR_BD` | **sí** | `true` | `false` tras `bd.sh eliminar`; sin valor el plan falla a propósito |
   | `HABILITAR_NAT` | no | `true` / `false` | NAT del Worker |
   | `DOMINIO_WEB` | no | vacío (sslip.io) | nombre del certificado |
   | `WORKER_CONCURRENCY`, `AUTH_RATE_LIMIT_PER_MINUTE` | no | `2`, vacío | parámetros del experimento |
   | `ADMIN_EMAIL` | no | correo | administrador inicial; quitarla tras el primer despliegue |
   | `APAGADO_NOCTURNO` | no | `true` | apagado programado |

   Con despliegues desde GitHub, **estas variables son la configuración**:
   un `terraform.tfvars` distinto en un portátil haría que un apply local
   deshiciera lo que aplicó CI, y al revés. Lo mismo `bd.auto.tfvars`, que
   `bd.sh` escribe solo en el portátil: tras `bd.sh eliminar`, fijar
   `CREAR_BD=false` aquí. Por eso `CREAR_BD` no tiene valor por defecto en
   CI: si faltara y CI asumiera `true`, un `infra` recrearía vacía una base
   que se eliminó a propósito.

   `CUENTA_FACTURACION` no es una credencial, pero el repositorio es
   público: el workflow la enmascara en los registros (`::add-mask::`) y la
   quita del plan que va al resumen.

3. **Entornos** (Settings > Environments > New environment): `gcp` y
   `gcp-rutina`. El proveedor de Workload Identity solo acepta tokens de
   trabajos que declaran uno de los dos: un trabajo sin entorno no recibe
   credenciales. Si no se crean, GitHub los crea sin protección en la
   primera ejecución.
   - `gcp` (infra, publicar, desplegar, certificado, sembrar, encender,
     apagar): **Deployment branches and tags** > *Selected branches* >
     `main` y, si el equipo lo quiere, **Required reviewers**: el apply
     espera la aprobación después de publicar el plan en el resumen de la
     ejecución.
   - `gcp-rutina` (plan, estado, apagado nocturno): también solo `main`, y
     **sin** revisores, o el apagado nocturno se quedaría esperando.

   Y proteger `main` (Settings > Branches o Rules > *Rulesets*): exigir PR
   con revisión y prohibir el push forzado y el borrado. Con eso, llegar a
   GCP exige pasar por una revisión en tres sitios: la condición de Google
   (el bootstrap exige por defecto `assertion.ref == 'refs/heads/main'`, así
   que otra rama no obtiene credenciales aunque su workflow declare el
   entorno), las reglas de ramas de cada entorno y la protección de `main`.
   Para probar desde otra rama, repetir el bootstrap con `--cualquier-rama`
   y añadir la rama a los entornos; deshacerlo después.

   **Una sola identidad para los dos entornos.** Separar una cuenta
   "operador" para `gcp-rutina` (plan, estado, apagar) sería posible, pero
   hoy aporta poco: el plan y el refresh de Terraform leen el estado y los
   valores de Secret Manager, que incluyen todos los secretos, así que esa
   cuenta ya vería lo más sensible. Tendría sentido si el plan dejara de
   correr en `gcp-rutina`.

4. **Llevar el workflow a `main`.** GitHub solo muestra el botón *Run
   workflow* si el archivo está en la rama por defecto, y la API y `gh`
   solo pueden lanzar en otra rama (`--ref`) un workflow que ya se ejecutó
   alguna vez. Este solo se dispara a mano, así que la primera vez tiene
   que estar en `main` (fusionar el PR). Después sí se puede ejecutar la
   versión de otra rama, si las reglas del entorno la admiten:
   ```bash
   gh workflow run desplegar-gcp.yml --ref <rama> -f accion=plan
   ```

### Primera vez

En Actions > *Desplegar en GCP* > *Run workflow*, o con `gh`, en este orden:

| # | Acción | Qué hace | Tiempo aprox. |
|---|---|---|---|
| 1 | `plan` | Opcional: ver qué se va a crear, sin tocar nada | 2 min |
| 2 | `infra` | Plan, aprobación (si `gcp` la exige) y apply de ese plan | 15-20 min (Cloud SQL) |
| 3 | `publicar` | Construye api, migrate, seed y worker en el runner y las sube con el commit como etiqueta | 10-15 min la primera, menos con caché |
| 4 | `desplegar` | `generar-env.sh`, Worker, Web y migraciones | 5-10 min |
| 5 | `certificado` | Let's Encrypt en el Web Server (una vez) | 1 min |

```bash
gh workflow run desplegar-gcp.yml -f accion=infra
gh run watch
gh workflow run desplegar-gcp.yml -f accion=publicar
gh workflow run desplegar-gcp.yml -f accion=desplegar
gh workflow run desplegar-gcp.yml -f accion=certificado
```

`todo` encadena 2, 3 y 4 en una ejecución (con dos aprobaciones si `gcp`
tiene revisores: la del apply y la del despliegue). Tras el paso 2 las VM
tardan 3-5 minutos en instalar Docker; si `desplegar` llega antes, falla con
"el arranque de la VM no terminó" y basta con repetirlo.

Si el primer despliegue se hace con `ADMIN_EMAIL`, la contraseña está en
Secret Manager (`mooc-admin-password`); después borrar la variable y volver a
desplegar.

### Operación

- **Cambio de código:** `publicar` y `desplegar` (o `todo`, que además
  planifica la infraestructura y no aplica nada si no hay cambios).
- **Volver atrás:** `desplegar` con `tag` = un commit o un tag de git ya
  publicado (p. ej. `entrega-2`). Se despliega `deploy/` de ese commit con sus
  imágenes; si no se publicaron, el trabajo falla antes de tocar las VM.
- **Cambio de infraestructura:** editar `terraform/` o las variables del
  repositorio y `infra`. El plan queda en el resumen de la ejecución; si el
  estado cambia entre el plan y la aprobación, el apply se rechaza por
  obsoleto y hay que repetir.
- **Datos sintéticos:** `sembrar`. El escenario (sesiones válidas de los
  usuarios sintéticos) se queda en la VM, no se sube como artefacto; se trae
  con `remoto.sh` (ver el resumen de la ejecución). Las cuentas tienen
  contraseña pública: neutralizarlas al terminar ([Cuentas
  sintéticas](#cuentas-sintéticas-después-de-las-pruebas)).
- **Encender y apagar:** `encender` / `apagar` (`con_bd` incluye Cloud SQL).
  Escriben en el resumen la hora de Colombia, la UTC y quién lo hizo: es la
  bitácora de horas de uso para el informe de costos.
- **Apagado nocturno:** con `APAGADO_NOCTURNO=true`,
  [`apagado-nocturno.yml`](../../.github/workflows/apagado-nocturno.yml)
  detiene VM y Cloud SQL a las 23:07 hora de Colombia (04:07 UTC). No
  comparte grupo de concurrencia con los despliegues (GitHub solo guarda
  una ejecución pendiente por grupo y cancela la anterior, así que el
  apagado en espera se perdería): consulta con la API de GitHub si hay un
  despliegue en curso y espera hasta 40 minutos; si sigue, omite el apagado
  esa noche y falla para que se note. Un despliegue que espera aprobación
  no lo retiene. GitHub desactiva los programados de un repositorio público
  tras 60 días sin actividad.
- **Evidencia:** cada ejecución que despliega termina con
  [`resumen.sh`](resumen.sh): URL, salud de la API (y si el certificado es
  el autofirmado), commit desplegado en cada VM leído de la propia VM, tipo
  de máquina y disco, tier, edición y estado de Cloud SQL,
  `WORKER_CONCURRENCY` y el límite de autenticación efectivos. `estado` lo
  da sin cambiar nada. En local: `deploy/gcp/resumen.sh`.
- Lo que no tiene acción (`bd.sh`, `destruir.sh`, túneles a Mailpit, psql)
  se sigue haciendo desde un portátil con permisos de dueño.

### Qué ve cada quien

Los registros y los resúmenes de un repositorio público los ve cualquiera.
No llevan secretos:

- Los `.env` que genera CI solo llevan nombres de secretos, IDs e IPs
  privadas; los valores los lee cada VM de Secret Manager con su propia
  cuenta de servicio (`en-vm.sh`) y no pasan por el runner.
- `terraform plan` enmascara los valores sensibles (`(sensitive value)`); el
  JSON del plan, que no los enmascara, solo se filtra con `jq` para listar
  recursos y nunca se imprime ni se guarda. El plan binario va al bucket del
  estado, no a un artefacto.
- `generar-env.sh` comprueba que cada secreto tenga versión activa sin
  leer su valor; `resumen.sh` solo lee dos parámetros no secretos de los
  `.env` de las VM.
- La clave de insignias se genera en `bootstrap-ci.sh` o `desplegar-infra.sh`
  y va directa a Secret Manager desde un archivo temporal.

### SSH desde CI

`desplegar-vm.sh`, `migrar.sh` y `remoto.sh` entran por IAP con OS Login
como `mooc-deployer` (usuario `sa_<id>`). `gcloud compute ssh` genera en el
runner una llave efímera (`~/.ssh/google_compute_engine`, sin frase por
`--quiet`) y registra la pública en el perfil de OS Login de la cuenta. La
privada muere con el runner, pero la pública quedaría en el perfil; por eso
`lib.sh` pasa `--ssh-key-expire-after 1h` cuando corre en GitHub Actions
(gcloud la aplica también con OS Login; `MOOC_SSH_CADUCIDAD` la cambia) y el
último paso de cada trabajo la retira con `gcloud compute os-login ssh-keys
remove`. No hay llaves SSH en los secretos del repositorio ni en los
metadatos de las VM.

### Permisos de `mooc-deployer`

Justificados uno a uno en `bootstrap-ci.sh`. Ni `roles/owner` ni
`roles/editor`:

| Rol | Por qué |
|---|---|
| `compute.instanceAdmin.v1` | VM y discos (`computo.tf`), detener y encender, `compute ssh` |
| `compute.networkAdmin` | VPC, subredes, router, NAT, direcciones, rango y peering de PSA |
| `compute.securityAdmin` | reglas de firewall (networkAdmin no las incluye) |
| `servicenetworking.networksAdmin` | conexión de Private Service Access de Cloud SQL |
| `cloudsql.admin` | instancia, base, usuario; detener, exportar, importar |
| `storage.admin` | buckets, su IAM (incluido `allUsers` en `hls`), estado de Terraform |
| `storage.hmacKeyAdmin` | claves HMAC de las cuentas de las VM |
| `secretmanager.admin` | secretos, versiones e IAM por secreto; clave de insignias |
| `artifactregistry.admin` | repositorio, limpieza, IAM y `docker push` |
| `resourcemanager.projectIamAdmin` **con condición** | solo puede conceder o quitar `logging.logWriter` y `monitoring.metricWriter` (`identidades.tf`); sin la condición podría darse `owner` |
| `serviceusage.serviceUsageConsumer` | `user_project_override` del provider; listar APIs. No puede habilitarlas: lo hace el bootstrap |
| `iap.tunnelResourceAccessor` | túnel de IAP al puerto 22 |
| `compute.osAdminLogin` | OS Login con sudo (`en-vm.sh` corre como root) |
| `billing.costsManager` (en la cuenta de facturación, opcional) | presupuesto (`costos.tf`); solo si quien corre el bootstrap administra la cuenta |

Sobre recursos concretos:

| Rol | Dónde | Por qué |
|---|---|---|
| `iam.serviceAccountUser` | solo `mooc-web` y `mooc-worker` | actAs: crear las VM con esas cuentas y entrar por OS Login |
| `storage.objectAdmin` | bucket del estado | estado y planes guardados |
| `iam.workloadIdentityUser` | `mooc-deployer`, concedido al `principalSet` del repositorio | el cambio de token de GitHub |

**Lo que no tiene, a propósito:**

- `iam.serviceAccountAdmin`: daría `setIamPolicy` sobre todas las cuentas
  del proyecto, incluida ella misma. Podría darse `serviceAccountTokenCreator`
  o dárselo a un usuario externo y conservar acceso fuera de GitHub. Por eso
  las cuentas de las VM las crea el bootstrap y Terraform solo las lee
  (`data "google_service_account"`, con bloques `removed` que las sacan sin
  borrarlas de un estado anterior). Nada más en `terraform/` cambia el IAM
  de una cuenta de servicio, y las claves HMAC solo necesitan
  `storage.hmacKeyAdmin`.
- `iam.serviceAccountUser` de proyecto: permitiría crear una VM con
  cualquier cuenta del proyecto. La más peligrosa es la cuenta por defecto
  de Compute Engine (`<número>-compute@developer.gserviceaccount.com`), que
  en un proyecto sin organización nace con `roles/editor`. El bootstrap le
  quita `roles/editor` y la deshabilita, salvo que alguna VM la use (o con
  `--conservar-sa-compute`). Nada de este despliegue la necesita: las VM
  usan sus propias cuentas y Cloud SQL, Artifact Registry y Service
  Networking trabajan con sus agentes de servicio (`service-<número>@...`).
- Si un bootstrap anterior dio esos dos roles de proyecto, repetirlo los
  retira.

`destruir.sh` ya no borra `mooc-web` ni `mooc-worker` (no cuestan nada). Se
reutilizan al recrear.

### Alternativas descartadas

- **Llave JSON de la cuenta de servicio en un secreto de GitHub:** no caduca,
  hay que rotarla a mano y quien la filtre despliega desde cualquier lugar.
  Con Workload Identity no hay nada que filtrar.
- **Acceso directo del principal federado, sin cuenta de servicio:** OS
  Login y la autorización de `actAs` para crear VM necesitan una cuenta de
  servicio.
- **Bootstrap en Terraform:** crea el bucket donde vive el estado; necesitaría
  su propio estado en otro sitio. Ver la cabecera de `bootstrap-ci.sh`.
- **Plan como artefacto de GitHub:** lleva los secretos del estado y el
  repositorio es público.
- **Construir con `docker/build-push-action`:** duplicaría en YAML la lista de
  imágenes y etiquetas de `publicar.sh`; el script ya sirve igual en local y
  en CI, y la caché `type=gha` se le da con `ghaction-github-runtime`.
- **`gcloud builds submit` (Cloud Build):** otro servicio y otra cuenta de
  servicio con permisos; el runner ya tiene Docker y basta.

### Cuentas sintéticas después de las pruebas

El sembrador crea `profesor@carga.mooc.local`, `sonda@carga.mooc.local` y
`estudiante-NNNN@carga.mooc.local`, todas con la misma contraseña, que está
en el código. Al terminar las pruebas, suspenderlas y revocar sus sesiones.
No se borran: el curso de carga las referencia (`teacher_id` es `ON DELETE
RESTRICT`) y sus intentos y progreso son parte de la evidencia. Suspender es
lo mismo que hace la API de administración (la cuenta no puede iniciar
sesión y sus sesiones quedan revocadas). Desde el Web Server, con la
contraseña de la base de Secret Manager:

```bash
deploy/gcp/remoto.sh web
# En la VM. La contraseña la lee la cuenta del Web; -e PGPASSWORD pasa la
# variable por nombre, sin que el valor aparezca en la línea de órdenes.
export PGPASSWORD="$(gcloud secrets versions access latest --secret mooc-bd-password)"
sudo -E docker run --rm -i -e PGPASSWORD \
  postgres:16-alpine psql "host=<DB_HOST> user=mooc dbname=mooc sslmode=require" <<'SQL'
UPDATE sessions SET revoked_at = now()
 WHERE revoked_at IS NULL
   AND user_id IN (SELECT id FROM users WHERE email LIKE '%@carga.mooc.local');
UPDATE users SET status = 'suspended', updated_at = now()
 WHERE email LIKE '%@carga.mooc.local';
SQL
```

Para otra corrida, `status = 'active'` con el mismo filtro y volver a
sembrar (el sembrador reutiliza las cuentas existentes y abre sesiones
nuevas).

## Migrar los datos de la entrega anterior

Las claves de los objetos no cambian; solo `hls/*` va a otro bucket.

```bash
# 1. Objetos de MinIO local a disco (con el compose local levantado)
mc alias set local http://localhost:9100 minioadmin minioadmin
mc mirror local/mooc ./export-objetos

# 2. A GCS: hls/ al bucket hls, lo demás al privado (se conservan las claves)
gcloud storage rsync --recursive ./export-objetos/hls gs://<bucket_hls>/hls
gcloud storage rsync --recursive --exclude '^hls/' ./export-objetos gs://<bucket_objetos>

# 3. Integridad: rsync valida el checksum de cada objeto al subirlo; para
#    una comprobación puntual, ambos en base64
gcloud storage hash ./export-objetos/resources/<id>/original
gcloud storage objects describe gs://<bucket_objetos>/resources/<id>/original --format='value(md5_hash)'

# 4. Base: volcado local, al bucket de respaldos e importación. Después del
#    paso 4 del despliegue y ANTES del 7: si la API arranca antes, migra la
#    base vacía y la importación choca con las tablas creadas.
docker compose exec -T postgres pg_dump -U mooc --no-owner --no-acl mooc | gzip > mooc.sql.gz
gcloud storage cp mooc.sql.gz gs://<bucket_respaldos>/exports/
gcloud sql import sql <bd_instancia> gs://<bucket_respaldos>/exports/mooc.sql.gz --database mooc --user mooc
```

La correspondencia entre la base y los objetos se comprueba con las claves
guardadas (`object_key` de recursos, `image_object_key` de insignias, etc.):
cada una debe responder a `gcloud storage objects describe`. El checksum
SHA-256 que la API guardó al verificar cada carga sirve para comparar el
contenido.

## Decisiones

### Tipo de máquina: e2-small

El enunciado fija 2 vCPU, 2 GiB y 30 GiB por VM. `e2-small` es exactamente
2 vCPU y 2 GB, pero sus vCPU son **compartidas**: la VM tiene garantizado el
25 % de cada una (0,5 vCPU sostenidas en total) y puede usar las dos al 100 %
en ráfagas mientras le queden créditos, que acumula cuando está por debajo de
su base. Para la API, que pasa la mayor parte del tiempo esperando a la base,
basta; para FFmpeg, no: una transcodificación larga agota la ráfaga y a partir
de ahí corre a la cuarta parte. Es el primer cuello de botella esperable del
escenario 2 y tiene que constar así en el informe de capacidad.

| Tipo | vCPU | Base sostenida | Memoria | Por qué no |
|---|---|---|---|---|
| **e2-small** | 2 compartidas | 0,5 | 2 GB | Elegida: coincide con el modelo del enunciado |
| e2-medium | 2 compartidas | 1 | 4 GB | Duplica la memoria: otra configuración |
| e2-standard-2 | 2 dedicadas | 2 | 8 GB | Cuadruplica la memoria |
| e2-custom-2-2048 | 2 dedicadas | 2 | 2 GB | Mismos recursos nominales sin CPU compartida, ~3 veces el precio |

`e2-custom-2-2048` es la alternativa si las mediciones con CPU compartida
resultan demasiado ruidosas (la ráfaga hace que la misma corrida dé
resultados distintos según los créditos acumulados). Se cambia con
`tipo_maquina` y, como pide el enunciado, los resultados se reportan como
otra configuración.

### Imagen: Debian 12 con Docker instalado al arrancar

Container-Optimized OS trae Docker, pero no el plugin compose, no tiene
gestor de paquetes (el Ops Agent no se instala ahí), su sistema de archivos
raíz es de solo lectura y `gcloud` solo está dentro de `toolbox`. Habría que
correr compose como contenedor y reescribir `en-vm.sh` y `certificado.sh`.
Debian 12 trae `google-cloud-cli`, admite el Ops Agent y el repositorio
oficial de Docker da Engine y compose v2 (el `docker.io` de Debian no trae
compose v2). El precio es depender de Internet en el primer arranque, que es
lo que motiva Cloud NAT.

El script de arranque (`terraform/arranque.sh.tftpl`) solo instala: los
metadatos de la VM los lee cualquiera con `compute.instances.get`, así que no
lleva configuración ni secretos. Deja `/var/log/mooc-arranque.log` y la marca
`/var/lib/mooc-arranque-ok`, que `desplegar-vm.sh` comprueba.

### Salida del Worker: Cloud NAT apagable

El Worker no tiene IP externa. Casi todo lo que necesita es de Google
(Cloud Storage, Artifact Registry, Secret Manager, Logging y Monitoring) y le
llega por **Private Google Access**, sin NAT y sin salir de la red de Google.
Lo que no es de Google son el repositorio de Docker, los paquetes de Debian y
el del Ops Agent, que solo se usan en el primer arranque. Redis se replica en
Artifact Registry (`publicar.sh`) para que tampoco dependa de Docker Hub.

Por eso Cloud NAT existe para el aprovisionamiento y se puede quitar después
(`habilitar_nat=false`). Cuesta poco (ver costos) y no toca el tráfico
pesado: con Private Google Access, las peticiones a las APIs de Google no
pasan por la NAT.

Alternativas descartadas:

- **Sin NAT y con Container-Optimized OS** (Docker de serie, todo desde
  Artifact Registry): los problemas de COS de arriba, y sin Ops Agent.
- **Imagen propia con Docker preinstalado** (Packer o una imagen de disco):
  otra herramienta y otro artefacto que mantener para ahorrar unos
  dólares al mes.
- **Proxy HTTP en el Web Server**: convierte al Web en ruta de salida del
  Worker, más configuración y un punto único más.
- **IP externa en el Worker con el firewall cerrado**: incumple que el
  Worker no se exponga a Internet, aunque ninguna regla lo abra.

La ruta por defecto `0.0.0.0/0 -> default-internet-gateway` se conserva: la
necesitan la IP pública del Web, la NAT y Private Google Access con el dominio
por defecto. minio-go solo reconoce GCS si el endpoint es exactamente
`storage.googleapis.com` (rechaza `private.googleapis.com` y los endpoints
regionales), así que no se usan los VIP privados.

### Administración: IAP + OS Login

La regla de SSH solo admite `35.235.240.0/20`, el rango de IAP TCP
forwarding. `gcloud compute ssh --tunnel-through-iap` autentica con la
cuenta de Google del integrante y OS Login decide si puede entrar y si tiene
sudo. No hay llaves que repartir ni puerto 22 expuesto, y funciona igual con
el Worker, que no tiene IP externa.

### Firewall por cuenta de servicio

Las reglas apuntan a las cuentas de servicio de las VM y no a etiquetas de
red: cambiar etiquetas solo pide `compute.instances.setTags`, mientras que
cambiar la cuenta de servicio de una VM exige detenerla y
`iam.serviceAccountUser` sobre la nueva. La regla sigue a la identidad.

Cloud SQL vive en la VPC del productor, enlazada por peering, y los firewalls
de esta VPC **no filtran su entrada**. Lo que sí se controla es la salida:
una regla de egress permite 5432 hacia el rango de Private Service Access
solo a las dos cuentas de servicio, y otra deniega todo lo demás hacia ese
rango. Una VM nueva en la VPC no llega a la base.

### Cloud SQL

- **Tier `db-custom-1-3840`** (1 vCPU dedicada, 3,75 GB, `max_connections`
  100 por defecto). El pool de la API es de 20 conexiones (`DB_MAX_CONNS`,
  antes fijo en el código) y el del worker se baja a 5 en su compose: con
  concurrencia 2 no necesita más. `db-f1-micro` (25 conexiones, 0,6 GB) no
  cabe; `db-g1-small` (50 conexiones, 1,7 GB) cabe con 20 + 5 + migraciones
  y psql, y cuesta la mitad, pero su vCPU compartida añade a las mediciones
  del escenario 1 un segundo recurso con ráfaga. Se cambia con `bd_tier`.
- **`edition = ENTERPRISE`** explícito: para PostgreSQL 16 la API puede crear
  Enterprise Plus, que no admite estos tiers y cuesta más.
- **Zonal, sin réplicas**, en la misma zona que las VM.
- **Solo IP privada** por Private Service Access (rango `10.30.0.0/20` y
  peering de `servicenetworking`), sin IP pública ni redes autorizadas.
- **`ssl_mode = ENCRYPTED_ONLY`**: rechaza conexiones sin TLS. Las URLs usan
  `sslmode=require` (cifra sin verificar la CA; `verify-ca` exigiría
  distribuir el certificado del servidor, y la red es privada).
- **Backups** diarios, 3 retenidos, sin PITR (el WAL se cobra aparte). Los
  backups automáticos **se borran con la instancia**: el respaldo que
  sobrevive es el export de `bd.sh` a un bucket y a disco local.
- **Dos protecciones contra borrado distintas**: `deletion_protection` es de
  Terraform (un plan que borre la instancia falla, activada por defecto) y
  `settings.deletion_protection_enabled` es de la API de Cloud SQL (frena
  también la consola y gcloud, desactivada por defecto para que el borrado
  del final de la entrega no exija un paso más). `bd.sh eliminar` y
  `destruir.sh` bajan ambas explícitamente.
- La contraseña la genera Terraform, va a Secret Manager y la lee cada VM al
  desplegar. Nunca está en el repositorio, en un `.env` ni en los metadatos.
- El nombre de la instancia lleva un sufijo aleatorio que cambia al
  recrearla: Cloud SQL reserva un nombre hasta una semana después de
  borrarlo.

### Cloud Storage por la API XML con claves HMAC

El código ya habla S3 con minio-go. La API XML de Cloud Storage acepta firmas
SigV4 con **claves HMAC** de cuenta de servicio, así que el cambio de código
es solo de configuración y de credenciales. Operaciones que usa la
plataforma y su soporte en la API XML:

| Operación | Dónde | API XML de GCS |
|---|---|---|
| PUT prefirmado (carga simple) | `PresignedPutURL` | Sí |
| Multipart: iniciar, subir parte prefirmada, listar partes, completar, abortar | `InitiateMultipartUpload`, `PresignedUploadPartURL`, `ListObjectParts`, `CompleteMultipartUpload`, `AbortMultipartUpload` | Sí (XML multipart upload) |
| GET prefirmado con `response-content-disposition` | `PresignedGetURL` | Sí |
| Stat, Get, Remove | verificación posterior a la carga | Sí |
| FGet / FPut, Put | worker (originales, HLS, PDF), insignias, subtítulos | Sí. minio-go detecta GCS y sube en un solo PUT (GCS no admite la firma en streaming), hasta 5 GiB por objeto |
| HeadBucket al arrancar | `asegurarBucket` | Exige `storage.buckets.get`, que las cuentas de servicio no tienen: el AccessDenied se tolera |

Ajustes de código (commit "Adaptar el almacén y el pool de PostgreSQL a
GCP"): la llave es siempre explícita y sin ella el proceso no arranca (se
retiró la cadena de credenciales de AWS, que termina en el servicio de
metadatos de EC2 y en GCE solo añadía un arranque lento y un modo anónimo);
`S3_REGION=auto` es la región del ámbito SigV4 que espera GCS;
`S3_CREATE_BUCKET=false` porque los buckets los crea Terraform; y
`S3_BUCKET_HLS` (abajo). Hay pruebas que firman contra
`storage.googleapis.com` y comprueban host virtual, región y enrutamiento de
claves.

**Permisos por componente** (cada uno con su cuenta de servicio y su clave
HMAC):

| | Bucket `objetos` | Bucket `hls` | Secretos |
|---|---|---|---|
| API (`mooc-web`) | `objectUser`: leer, escribir, borrar, multipart | `objectViewer` | contraseña BD, HMAC web, admin, insignias |
| Worker (`mooc-worker`) | `objectViewer` + `objectUser` **condicionado** a `presentaciones/` | `objectUser` | contraseña BD, HMAC worker |

El worker no puede escribir ni borrar originales (`resources/`), insignias ni
subtítulos, y la API no puede escribir derivados HLS. `objectUser` y no
`objectCreator` para el worker porque un reintento de asynq reescribe la
misma clave, y sobrescribir exige `storage.objects.delete`.

**CORS**: el origen real (`url_publica`), `PUT`, `GET` y `HEAD`, y
`ETag` en `responseHeader`: sin él el navegador no puede leer el ETag de cada
parte y la multipart no se completa. **Ciclo de vida**:
`AbortIncompleteMultipartUpload` a 1 día (las partes de una carga
abandonada se cobran y no aparecen al listar). **Eliminación temporal**
desactivada: por defecto GCS retiene y cobra 7 días lo borrado.

### HLS sin firma: bucket separado

El reproductor pide la lista maestra con la URL firmada que emite la API, y
de ahí las variantes y los segmentos con **rutas relativas**, que no heredan
la firma. En local se resuelve abriendo el prefijo `hls/` de MinIO a lectura
anónima. En GCS con acceso uniforme a nivel de bucket eso no se puede hacer:
los permisos son del bucket entero, y las IAM Conditions (que sí distinguen
prefijos) no admiten `allUsers` ni `allAuthenticatedUsers`.

Opciones evaluadas:

- **(a) Bucket aparte, público, solo para `hls/*`.** Elegida. Las claves no
  cambian (`hls/<recurso>/master.m3u8` sigue siendo lo que guarda la base);
  solo el bucket que las aloja (`S3_BUCKET_HLS`, un `strings.HasPrefix` en
  el cliente de almacenamiento, con pruebas). El bucket privado mantiene
  acceso uniforme y prevención de acceso público **forzada**, así que un
  error de configuración no puede exponer un original. La lectura anónima
  se da con `roles/storage.legacyObjectReader` (solo `objects.get`) y no con
  `objectViewer`, que permitiría listar el bucket y enumerar los recursos.
- **(b) ACL por objeto** (desactivar el acceso uniforme y subir `hls/*` con
  `x-goog-acl: public-read`). Descartada: las ACL son el modelo heredado, el
  bucket con los originales no podría forzar la prevención de acceso
  público, `UploadFile` tendría que enviar la ACL y la cuenta del worker
  necesitaría permiso para fijarla, y un error en una sola subida expondría
  un objeto privado.
- **(c) Reescribir las listas en la API** firmando cada variante y cada
  segmento. Descartada: bastante más código y la API en el camino de cada
  lista de reproducción.
- **(d) Nginx como proxy del bucket o Cloud CDN con cookies firmadas.**
  Descartadas: el enunciado pide servir el contenido directamente desde el
  almacenamiento y excluye la CDN en esta etapa.
- **`S3_PUBLIC_URL` como atajo**: descartado. Quita la firma de *todas* las
  claves, originales y PDFs incluidos (`media_handlers.go`), no solo de HLS.

La contrapartida, la misma que en local: quien conozca la URL de un
segmento puede descargarlo sin estar inscrito. Los identificadores son UUID
y el bucket no se puede listar; el control fuerte llega con la CDN de la
siguiente etapa.

**Políticas de organización.** En un proyecto sin organización (cuenta
personal con créditos) no aplican. Si el proyecto cuelga de la organización
de la universidad, dos pueden impedir la opción (a):
`constraints/iam.allowedPolicyMemberDomains` (rechaza el binding de
`allUsers`) y `constraints/storage.publicAccessPrevention` (fuerza la
prevención en todos los buckets). Comprobarlo antes de aplicar:

```bash
gcloud resource-manager org-policies describe iam.allowedPolicyMemberDomains --project <proyecto> --effective
gcloud resource-manager org-policies describe storage.publicAccessPrevention --project <proyecto> --effective
```

Si alguna está activa, `terraform apply` falla en `hls_publico`. El ajuste
mínimo es pedir la excepción para el proyecto; si no se concede, la única
salida que no abre nada es la opción (c), que habría que implementar. En
ningún caso se abre el bucket privado.

### HTTPS

nginx termina TLS en el Web con Let's Encrypt sobre `<IP estática>.sslip.io`
(o `dominio_web`), redirige 80 a 443, sobrescribe `X-Forwarded-For` con la
IP real (la API la usa para el límite de tasa) y fija
`X-Forwarded-Proto https`. `COOKIE_SECURE=true`. Hasta que se emite el
certificado real sirve uno autofirmado de 30 días.

## Costos

Precios de lista aproximados en `us-central1` a septiembre de 2026, sin
impuestos ni descuentos por uso continuo. **Verificarlos en la calculadora de
GCP con la fecha del informe** y contrastarlos con Facturación > Informes,
filtrando por la etiqueta `proyecto=mooc`.

| Recurso | Supuesto | USD/hora | USD/mes (730 h) |
|---|---|---|---|
| 2 x e2-small | encendidas | 2 x 0,0168 | 24,5 |
| 2 x disco pd-balanced 30 GB | siempre (también detenidas) | | 6,0 |
| IP externa estática del Web | en uso (0,005/h); **sin uso, 0,01/h** | 0,005 | 3,7 |
| Cloud NAT | 1 VM (0,0014/h) + su IP (0,005/h) + 0,045/GB procesado | 0,0064 | 4,7 + tráfico |
| Cloud SQL db-custom-1-3840 | encendida | 0,0676 | 49,3 |
| Cloud SQL almacenamiento | 10 GB SSD, 0,17/GB-mes (también detenida) | | 1,7 |
| Cloud SQL backups | ~3 x tamaño de la base, 0,08/GB-mes | | < 1 |
| Cloud Storage Standard | 20 GB a 0,020/GB-mes | | 0,4 |
| Operaciones GCS | clase A 0,05/10k, clase B 0,004/10k | | < 1 en pruebas |
| Salida a Internet | segmentos HLS y descargas, ~0,12/GB | | depende de las pruebas |
| Artifact Registry | ~2-3 GB tras varias publicaciones, 0,10/GB-mes (0,5 GB gratis) | | 0,3 |
| Secret Manager | 5 secretos (6 versiones gratis) | | ~0 |
| Ops Agent / Logging | dentro de las cuotas gratuitas para 2 VM | | ~0 |
| IAP TCP forwarding | | | 0 |

Todo encendido: unos **0,12 USD/hora**, ~90 USD/mes. Con Cloud SQL
`db-g1-small` baja a ~0,09 USD/hora. Lo que no se va con `energia.sh
detener --con-bd` (~15 USD/mes): discos, la IP estática (sube a la tarifa de
IP reservada sin uso), la NAT si sigue habilitada, el almacenamiento y los
backups de Cloud SQL, los buckets y el registro.

- **Cloud SQL detenida** (`activation-policy NEVER`) no se vuelve a encender
  sola y deja de cobrar CPU y memoria, pero sigue cobrando almacenamiento y
  backups. El enunciado pide registrar las condiciones del proveedor sobre
  una detención prolongada: consultarlas en la documentación vigente de Cloud
  SQL ("Start, stop, and restart instances") y citarlas con fecha.
- **Salida a Internet**: es lo que más puede crecer en el escenario 2. Un
  vídeo de 10 minutos en 720p son ~300-400 MB de segmentos por reproducción
  completa; cien reproducciones, ~40 GB, ~5 USD. El tráfico entre Cloud
  Storage y las VM en la misma región no se cobra.
- **Presupuesto**: `cuenta_facturacion` crea un Cloud Billing Budget con
  alertas al 50, 90 y 100 % y al 100 % proyectado, con
  `credit_types_treatment = EXCLUDE_ALL_CREDITS`: con créditos educativos el
  gasto neto es 0 y un presupuesto que descuente créditos nunca avisaría.
  Exige administrar la cuenta de facturación; si el crédito está en una
  cuenta que el equipo no administra, no se puede crear y hay que
  documentarlo. Un presupuesto avisa, no corta el gasto.

## Eliminar y recrear

Tras registrar las evidencias:

```bash
deploy/gcp/bd.sh eliminar        # export a gs://...-respaldos y a deploy/gcp/respaldos/, borra Cloud SQL
deploy/gcp/energia.sh detener    # VM detenidas: quedan discos, IP y buckets
```

Para la sustentación:

```bash
deploy/gcp/bd.sh recrear gs://<bucket_respaldos>/exports/<export>.sql.gz
deploy/gcp/generar-env.sh -f     # la IP privada de la base cambió
deploy/gcp/energia.sh iniciar
deploy/gcp/desplegar-vm.sh worker && deploy/gcp/desplegar-vm.sh web
```

La importación va antes de desplegar la API: si arrancara contra la base
vacía, migraría y la importación chocaría con las tablas creadas.

`bd.sh` y `destruir.sh` se corren desde un portátil (dueño del proyecto),
sobre el mismo estado remoto que usa GitHub Actions. Si el equipo despliega
desde GitHub, tras `bd.sh eliminar` fijar la variable `CREAR_BD=false`, y tras
`bd.sh recrear`, `CREAR_BD=true` (o borrarla) y la acción `desplegar`: la IP
privada de la base cambió.

Eliminación total (proyecto a cero salvo la clave de insignias):

```bash
deploy/gcp/destruir.sh --confirmar --respaldar-buckets ~/respaldo-mooc
```

Se conservan: el export de la base en `deploy/gcp/respaldos/` (ignorado por
git: contiene datos y hashes de contraseñas), los objetos en el directorio
indicado y la clave de firma de insignias en Secret Manager, que vive fuera
de Terraform para que las credenciales emitidas sigan verificándose al
recrear. Para reconstruir: pasos 4 a 8, `bd.sh recrear <export local>` y
`gcloud storage rsync` de los objetos respaldados.

## Estado de Terraform y secretos

El estado de Terraform contiene **en claro** la contraseña de la base, la del
administrador y los secretos HMAC, porque Terraform los genera. Vive en el
bucket `gs://<proyecto>-tfstate/mooc/` (backend `gcs` con configuración
parcial en `versions.tf`; `lib.sh` pasa el bucket al hacer `init`), que
`bootstrap-ci.sh` crea con:

- acceso uniforme y prevención de acceso público forzada;
- versionado: un apply que deja el estado mal se deshace recuperando la
  versión anterior (se conservan 30);
- una política IAM fijada entera: dueños del proyecto y la cuenta de CI. Se
  quitan los permisos que GCP da por defecto sobre cada bucket nuevo a los
  lectores y editores del proyecto (`legacyObjectReader` a los lectores
  bastaría para leer el estado). Un rol de proyecto con permisos de Storage
  (`roles/editor`, `roles/storage.admin`) sigue alcanzando el bucket: no darlo
  a quien no deba ver los secretos.

Los planes guardados de CI (`planes/`) llevan los mismos secretos y por eso
van a este bucket y no a un artefacto de GitHub, que en un repositorio
público puede descargar cualquiera. Se borran al aplicarse, y a los 7 días si
nadie los aplica.

**Migrar un estado local existente** (un despliegue hecho antes de este
cambio, con `terraform.tfstate` en `deploy/gcp/terraform/`), una sola vez y
antes de cualquier despliegue desde GitHub:

```bash
deploy/gcp/bootstrap-ci.sh --proyecto <proyecto>      # crea el bucket
deploy/gcp/desplegar-infra.sh --migrar-estado
```

Equivale a `terraform init -migrate-state -backend-config=bucket=<proyecto>-tfstate
-backend-config=prefix=mooc`; además se niega a sobrescribir un estado remoto
que ya exista y aparta el archivo local como `terraform.tfstate.migrado-<fecha>`
(sigue ignorado por git; borrarlo cuando `terraform output` responda desde el
bucket). Mientras exista un `terraform.tfstate` local sin migrar, los scripts
se niegan a inicializar contra el bucket: aplicar sobre un estado remoto vacío
intentaría crearlo todo otra vez. Nunca subir el estado al repositorio ni
adjuntarlo a la entrega.

Ningún secreto está en el repositorio, en los `.env` (solo nombres de
secretos y el identificador de la clave HMAC, que sin su secreto no sirve),
en los metadatos de las VM ni en las imágenes. En la VM, los secretos existen
solo en el entorno de los contenedores.

`.terraform.lock.hcl` no está en el repositorio: se genera en el primer
`terraform init`. Conviene confirmarlo tras ese primer `init` para fijar las
versiones del provider (con `terraform providers lock -platform=linux_amd64
-platform=darwin_arm64` para que valga en CI y en los portátiles). Mientras no
esté, el workflow lleva el lock del plan al apply para que ambos usen las
mismas versiones.

## Limitaciones conocidas

- **El despliegue desde GitHub Actions tampoco se ha ejecutado aún**: los
  workflows pasan actionlint, los scripts shellcheck y `bootstrap-ci.sh` se
  recorrió con un `gcloud` simulado, pero la federación, OS Login con la cuenta de servicio y la caché de
  buildx se comprueban en la primera ejecución real.
- **No se ha probado contra un proyecto real.** La plantilla pasa
  `terraform validate` con los providers 7.46 y 8.4 y un `terraform test`
  con proveedores simulados (`terraform/tests/`, se corre con
  `terraform test` dentro de `terraform/`), y los scripts pasan `bash -n` y
  shellcheck, pero nada de eso crea recursos.
- Puntos únicos de falla: cada VM, Redis (un solo contenedor, AOF en el disco
  del Worker), Cloud SQL zonal y la zona entera. Es lo que pide esta etapa.
- Redis sin contraseña (el código no la soporta): lo protegen el firewall por
  cuenta de servicio y el bind a la IP interna.
- CPU compartida de e2-small: ver arriba. Registrar en cada corrida.
- Las URLs firmadas de la API valen 15 minutos (entrega) y 24 horas (cargas);
  las claves HMAC no caducan, así que las URLs valen lo que dice su
  `X-Amz-Expires`. Rotar una clave HMAC invalida las URLs que firmó.

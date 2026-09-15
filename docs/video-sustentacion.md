# Cómo grabar el vídeo de sustentación

`demostracion.md` dice **qué** hay que demostrar en cada uno de los nueve
segmentos. Este documento dice **cómo grabarlo**: en qué orden, en qué pantalla,
qué decir mientras se ve, y qué hacer cuando algo se tuerce en mitad de la toma.

La regla que ordena todo lo demás: **no hace falta desplegar en la nube.** La
sección 10.1 pide la demostración «con datos sintéticos sobre el sistema
desplegado mediante Docker Compose». Levantarlo en AWS no suma un punto y añade
una hora de riesgo el día de la entrega.

## Antes de grabar

Media hora larga, y conviene hacerla el día antes, no diez minutos antes.

```bash
# 1. CI en verde. La sección 10.1 lo pide ANTES de la demostración.
#    Abre la pestaña Actions y déjala en una pestaña del navegador: se enseña.

# 2. Levantar la plataforma.
cp .env.example .env                              # pon ADMIN_EMAIL y ADMIN_PASSWORD
docker compose --profile antivirus up -d clamav   # tarda: baja las firmas
CLAMAV_ADDR=clamav:3310 docker compose up -d --build

# 3. Datos sintéticos.
docker compose --profile carga run --rm seed

# 4. Prueba de carga, ANTES de grabar. Son cuatro minutos y no se graban en
#    directo: se graba el informe.
docker compose --profile carga run --rm k6

# 5. Ensayo de la colección de Postman. Si esto pasa, la API está sana.
newman run postman/plataforma-mooc.postman_collection.json \
  -e postman/local.postman_environment.json --env-var adminPassword='la-del-.env' \
  --folder "0 · Preparación" --folder "1 · Identidad y administración" \
  --folder "2 · Autoría y publicación" --folder "3 · Carga multimedia" \
  --folder "4 · Procesamiento y fallos" --folder "5 · Consumo de contenido" \
  --folder "6 · Evaluación" --folder "7 · Progreso y aprobación" \
  --folder "8 · Insignia" --folder "9 · Operación"
```

Ten preparado además:

- Un **vídeo de 200–400 MB**. Más pequeño no deja tiempo a cortar la red a
  mitad; más grande alarga la toma sin enseñar nada nuevo.
- El **fichero EICAR** (`X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*`
  en un `.txt`), para el rechazo antimalware del segmento 3.
- Dos navegadores o dos perfiles, para enseñar la revocación de sesiones: una
  pestaña muere cuando la otra revoca.
- El zoom del navegador al 125 % y la fuente de la terminal grande. Un JSON
  ilegible no es evidencia.

## Cómo repartir la pantalla

Cuatro superficies, y merece la pena tenerlas ya abiertas en pestañas fijas:

| Superficie | Dónde | Para qué |
|---|---|---|
| Interfaz | `http://localhost:3000` | El flujo, tal como lo vive una persona |
| Postman | la colección de `postman/` | El cuerpo de la respuesta y sus aserciones |
| Mailpit | `http://localhost:8025` | Los correos de verificación e invitación |
| Terminal | — | Logs correlacionados, escalado, k6 |

El error típico es grabarlo todo desde la interfaz. La sección 10.1 pide cuatro
evidencias —**respuesta de la API, comportamiento de la interfaz, estado
persistido y logs correlacionados**— y la interfaz sola solo da una. Alterna.

## El guion, minuto a minuto

Los minutos son un reparto proporcional; ajústalos al tiempo que pida el
enunciado sin cambiar el orden ni las proporciones.

### Apertura · 1 min

Una frase sobre qué es la plataforma y otra sobre la arquitectura: monolito
modular en Go con el dominio desacoplado del transporte, Next.js delante,
Postgres, Redis, almacén S3 y un worker asíncrono. Enseña `docker compose ps`
con todo arriba. **Enseña el CI en verde**: es un requisito formal de 10.1 y se
despacha en quince segundos.

### 1 · Identidad y administración · 2 min

**En la interfaz.** Registra un estudiante. Salta a Mailpit, abre el correo,
pulsa el enlace, entra.

Invita a una profesora desde `/admin` y di la frase: *no hay autorregistro de
profesores, el rol solo lo concede un administrador*.

**En la terminal o en Postman.** Un estudiante llamando a un endpoint de
autoría: `403`. Y acto seguido su entrada en la bitácora. La autorización se
comprueba en el servidor; esconder el botón no es autorizar.

**En dos pestañas.** Revoca las sesiones desde `/cuenta/sesiones` y enseña cómo
la otra pestaña muere en su siguiente petición.

### 2 · Autoría y publicación · 2,5 min

**En la interfaz.** Crea el borrador, añade módulo, unidad y recursos, abre la
previsualización.

Intenta publicar **incompleto**. Esto es lo que hay que enseñar despacio: la
respuesta trae **la lista entera de motivos**, no el primero que falla. Un
formulario que te hace adivinar de uno en uno es inútil.

Completa, publica, y vuelve al editor: la versión publicada está bloqueada. Di
por qué: cambiarla movería el suelo a quien ya está matriculado; para eso están
los borradores de actualización, que salen en el segmento 8.

### 3 · Carga multimedia · 2,5 min

**En la interfaz, con el panel de red abierto.** Empieza a subir el vídeo
grande. A mitad, **corta la red** (el panel de red de Chrome tiene «Offline»).
Recarga la página, vuelve a elegir **el mismo archivo**: se reanuda desde donde
iba, no desde cero.

En el panel de red enseña que el binario **no pasa por la API**: va a una URL
prefirmada del almacén. Si prefieres verlo entero y legible, esa misma URL sale
en Postman, en la carpeta 3, con tres aserciones al lado: va firmada, caduca, y
no apunta a la API.

Termina con el rechazo: sube el EICAR y enseña el `422`. Con el perfil
`antivirus` levantado lo caza ClamAV con firmas reales; sin él, el escáner
integrado reconoce el vector estándar. **Dilo tal cual**, no lo vendas como un
antivirus completo si has grabado sin ClamAV.

### 4 · Procesamiento y fallos · 2 min

**En la terminal.** `docker compose logs -f worker` mientras el recurso pasa de
`pending` a `processing` y a `ready`. Enseña el `X-Request-Id` atando la
petición de la API con la línea del worker.

Entrega el mismo trabajo dos veces: no produce dos salidas. Fuerza tres fallos,
enseña la cola de fallidos en `/admin` y reencola con la misma clave.

> **Aquí toca ser honesto:** no hay trazas OpenTelemetry. La correlación se
> enseña con `X-Request-Id` en los logs, que es menos de lo que pide el
> criterio. Decirlo cuesta cinco segundos y vale más que esperar a que lo
> pregunten.

### 5 · Consumo de contenido · 2 min

**En la interfaz.** Inscríbete, reproduce el HLS, cambia la calidad. Cierra,
vuelve a entrar: reanuda en el segundo donde lo dejaste. Abre un PDF.

Recorre una pantalla **entera con el teclado**, sin tocar el ratón, y enseña
dónde está el foco en cada salto.

Cambia el idioma a inglés: la interfaz entera pasa, fechas incluidas. Son 463
claves y el catálogo está tipado contra las españolas, así que una traducción
que falte no compila.

### 6 · Evaluación · 2,5 min

**Empieza en la interfaz**, abre un intento, responde a medias, **recarga la
página**: no se ha perdido nada.

**Y ahora salta a Postman.** Este es el segmento donde la colección gana al
navegador: abre la carpeta 6, ejecuta «Abrir un intento» y enseña el cuerpo
crudo en pantalla, con la pestaña de resultados al lado diciendo en verde **«el
cuerpo no contiene is_correct»** y «cada opción solo trae su id y su texto». Esa
es la diferencia entre enseñar un JSON y demostrar una propiedad.

Vuelve a la interfaz para el envío, y repítelo con la **misma
`Idempotency-Key`**: misma nota, mismo intento. Un corte de red al pulsar
«enviar» no puede costar un intento.

### 7 · Progreso y aprobación · 2 min

**En Postman.** Abre la carpeta 7 y enseña el cuerpo de la petición de
progreso: **no admite ningún porcentaje**. El cliente solo puede decir «abrí»,
«sigo aquí» y «cerré»; el porcentaje lo calcula el servidor. Esa es la respuesta
a «¿y si el cliente miente?»: no hay campo con el que mentir.

Manda un latido antes del intervalo mínimo: se acepta la petición pero **no
acredita tiempo**, y aparece en la bitácora como `progress.rejected` con el
motivo. Las dos cosas, en pantalla.

Cierra completando lo obligatorio y enseña la transición a `completed` y a
`approved`.

### 8 · Insignia y actualización · 2,5 min

**En la interfaz.** La insignia aparece al aprobar. Abre su URL pública de
verificación **en una ventana de incógnito**, para que se vea que no hay sesión.

**En Postman**, carpeta 8, la aserción que importa: el cuerpo público **no
contiene el correo ni el id del estudiante**. Enseña también la credencial Open
Badges 3.0 firmada (`alg: EdDSA`) y el JWKS con la clave pública: sin eso, la
firma no la puede comprobar nadie de fuera.

Revoca y vuelve a cargar la verificación: ya no vale, y consta cuándo se revocó.

Termina con el borrador de actualización: clasificación de cambios y publicación
conservando el progreso por `stable_id`.

### 9 · Operación · 2 min

**En la terminal.**

```bash
docker compose up -d --scale api=3
```

Enseña el `docker compose ps` con las tres instancias y di que el proxy reparte
solo, sin tocar configuración: eso es lo que hace que la API sea escalable
horizontalmente de verdad.

**No corras k6 en directo**: son cuatro minutos de barra de progreso. Enseña el
informe que ya tienes y la tabla de `load/README.md`: 42.075 peticiones en cuatro
minutos, 0 % de error, p95 de 2 ms en catálogo, 5 ms en consumo, 5 ms en quiz y
277 ms en login. Explica el 277 ms del login antes de que te lo pregunten:
**Argon2id está calibrado para costar**; medirlo con el rasero de una lectura
invitaría a debilitarlo.

Enseña también el fichero de umbrales y di que k6 **sale con código 99** si
alguno se incumple. Un informe verde que no pueda fallar no acredita nada.

> **Y aquí lo pendiente, dicho en voz alta:** no hay backup, restauración ni
> prueba de RPO ≤ 15 min / RTO ≤ 4 h. Tampoco inyección de fallos durante la
> carga.

### Cierre · 1,5 min

Tres cosas, en este orden:

1. **Las pruebas encontraron defectos reales.** En su primera pasada, las E2E
   destaparon que un profesor no podía añadir **ningún** recurso (el editor
   mandaba PascalCase y la API espera snake_case: todo devolvía 400); axe
   destapó que la casilla de «opción correcta» de un quiz no tenía etiqueta, así
   que un profesor con lector de pantalla no sabía cuál marcar; y un contraste
   de 3,67:1, por debajo del mínimo AA. Los tres, corregidos y con una prueba
   que falla si vuelven. Ninguno lo vieron las 259 pruebas de backend y las 29
   de frontend, porque **ninguna cruzaba la frontera navegador↔API**. Eso dice
   más a favor de la suite que cualquier número de cobertura.
2. **La tabla de aceptación de la sección 10**, con las cuatro condiciones y
   cómo se cumple cada una.
3. **Lo que falta**, sin adornos: cursores y ETag, OpenTelemetry, cifrado en
   reposo, gestión externa de secretos, TLS 1.2+ y el plan de recuperación.

## Si algo se rompe en mitad de la toma

- **Un 429 inesperado.** Has gastado el presupuesto de autenticación ensayando.
  Espera un minuto o levanta `AUTH_RATE_LIMIT_PER_MINUTE`, **y dilo**: el
  limitador funcionando es una función, no un estorbo.
- **Un 403 `csrf_token_mismatch` en Postman.** Alguna petición salió con una
  cookie de sesión guardada y sin `X-CSRF-Token`. Es el producto defendiéndose
  bien. La colección vacía el tarro de cookies tras cada login precisamente por
  esto; si lo ves, es que estás repitiendo una petición a mano.
- **La subida del segmento 3 falla y luego falla la publicación.** No son dos
  fallos: sin objeto, el recurso se queda en `processing_status != ready` y
  publicar responde `422`. La causa está una pantalla más atrás.
- **El vídeo no reproduce.** Casi siempre es CORS del almacén contra el origen
  del frontend, o el prefijo `hls/` sin permiso de lectura. Está explicado en el
  README, en «Entrega del contenido multimedia».

## Tres frases que NO conviene decir

- «La plataforma **aguanta 2.000 concurrentes**.» No se ha medido. Lo medido son
  200 VU con dos órdenes de magnitud de margen: sugiere holgura, no la acredita.
  La frase correcta es «la Etapa 1 pasa con dos órdenes de magnitud de margen, y
  el escalón se sube cambiando una variable».
- «La plataforma **es accesible**.» Lo acreditado es que no tiene los defectos
  que una máquina detecta, en 13 pantallas y en dos idiomas. axe cubre del orden
  de la mitad de los problemas reales; el orden de foco y el recorrido con
  lector de pantalla siguen necesitando a una persona.
- «**Cumple RPO ≤ 15 min y RTO ≤ 4 h**.» No está probado. Está dicho en el
  segmento 9 y hay que dejarlo dicho también en el cierre.

Decir las tres cosas cuesta treinta segundos y es exactamente lo que separa una
sustentación que se sostiene de una que se cae en la primera pregunta.

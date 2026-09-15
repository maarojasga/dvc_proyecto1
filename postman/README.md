# Colección de Postman para la sustentación

Un recorrido por la API que sigue los **nueve segmentos** de la demostración de
aceptación (sección 10.2 del enunciado), una carpeta por segmento. No es un
catálogo de endpoints: cada petición encadena la siguiente y cada una comprueba
lo que su segmento tiene que acreditar.

Sirve para dos cosas en el vídeo:

1. **Enseñar la respuesta de la API**, que la sección 10.1 pide como evidencia
   explícita. En el navegador esa evidencia está escondida en el panel de red;
   aquí se ve el cuerpo entero en pantalla.
2. **Enseñar la comprobación, no solo el dato.** La pestaña «Test Results» dice
   en verde y con nombre propio *«el cuerpo no contiene is_correct»*. Un JSON
   bonito no demuestra nada; una aserción que podría fallar, sí.

## Lo que esta colección NO es: la prueba de carga

Postman no sirve para la prueba de carga de la Etapa 1, y conviene decirlo en el
vídeo en vez de disimularlo:

- Manda las peticiones **de una en una**. El enunciado pide concurrencia
  sostenida; el Collection Runner no la produce.
- **No tiene umbrales**. La condición de aceptación es «sin incumplimientos
  críticos», lo que exige que exista algo que se pueda incumplir. `load/etapa1.js`
  declara sus umbrales y k6 **sale con código 99** si alguno se rompe: por eso
  ese informe acredita algo.
- El número que saldría de Postman (unas decenas de peticiones seguidas desde un
  portátil) es un número bonito que no mide nada.

La carga vive en `load/`, se corre con `docker compose --profile carga run --rm k6`
y sus resultados medidos están en `load/README.md`. Postman cubre la evidencia de
**respuesta de la API**; k6, la de **rendimiento**. Son dos evidencias distintas
que la sección 10.1 pide por separado.

## Antes de correrla

```bash
cp .env.example .env          # define ADMIN_EMAIL y ADMIN_PASSWORD
docker compose up -d --build
```

`ADMIN_EMAIL` / `ADMIN_PASSWORD` siembran el administrador en el primer arranque.
La colección los necesita: el segmento 1 empieza entrando como administrador.

## Desde la aplicación de Postman

1. *Import* → los dos ficheros de esta carpeta.
2. Selecciona el entorno **MOOC local (Docker Compose)** arriba a la derecha.
3. Pon tu `adminPassword` en el entorno (viene con un valor de relleno).
4. *Run collection*, y **desmarca la carpeta 99** (ver más abajo).

## Desde la terminal

```bash
npm install -g newman
newman run postman/plataforma-mooc.postman_collection.json \
  -e postman/local.postman_environment.json \
  --env-var adminPassword='la-del-.env' \
  --folder "0 · Preparación" --folder "1 · Identidad y administración" \
  --folder "2 · Autoría y publicación" --folder "3 · Carga multimedia" \
  --folder "4 · Procesamiento y fallos" --folder "5 · Consumo de contenido" \
  --folder "6 · Evaluación" --folder "7 · Progreso y aprobación" \
  --folder "8 · Insignia" --folder "9 · Operación"
```

Medido tal cual, contra la plataforma levantada: **55 peticiones, 113
aserciones, 0 fallos**.

## La carpeta 99 va aparte, y a propósito

`99 · Límite de tasa` agota adrede el presupuesto de intentos de autenticación,
así que deja al resto de la colección sin servicio durante un minuto. Por eso no
entra en la ejecución normal. Para verla funcionar hace falta una API con
`AUTH_RATE_LIMIT_PER_MINUTE` en su **valor de producción** (10):

```bash
newman run postman/plataforma-mooc.postman_collection.json \
  -e postman/local.postman_environment.json \
  --env-var adminPassword='la-del-.env' \
  --folder "99 · Límite de tasa (opcional, va el último)"
```

Medido: corta en el **intento 11** con `429 rate_limited`. Si tu despliegue lo
tiene subido (la suite E2E lo pone en 300), la carpeta se da por vencida al
llegar al tope y **falla diciendo exactamente eso**, en vez de aprobar en falso.

## Dos detalles de implementación que no son manías

**Cada petición que cambia estado lleva la cabecera `X-CSRF-Token`, y tras cada
login se vacía el tarro de cookies.** Son dos defensas para el mismo problema, y
conviene entenderlo porque el síntoma desconcierta.

Postman guarda en un tarro por dominio las cookies de cada respuesta, así que en
cuanto haces un login queda ahí `mooc_session`. A partir de ese momento
**cualquier** POST, PUT o PATCH sale con sesión en cookie, y la defensa de doble
envío exige que repita en la cabecera el valor de la cookie `mooc_csrf`. Si no,
responde `403 csrf_token_mismatch` — y hace bien, porque eso es exactamente lo
que tiene que rechazar. Lo desconcertante es que afecta hasta al **registro**,
que no necesita sesión ninguna: el middleware solo mira si llega una cookie de
sesión, no si el endpoint la usa. En el navegador no pasa porque el frontend sí
repite la cabecera.

Vaciar el tarro sería suficiente… si funcionara siempre. En newman funciona,
pero en la **aplicación de Postman** `pm.cookies.jar()` exige que el dominio
esté en la lista de permitidos del gestor de cookies; si no lo está, el borrado
no ocurre y la colección empieza a dar 403. Por eso la cabecera va además por su
cuenta, desde el script de pre-petición de la colección: se captura `mooc_csrf`
de la respuesta del login —que se lee sin permiso especial, igual que
`mooc_session`— y se repite en cada petición insegura.

Verificado en las dos situaciones: con el tarro vaciándose (55 peticiones, 113
aserciones, 0 fallos) y **con el tarro intacto**, que es lo que hace la
aplicación sin la lista de permitidos: los mismos 113 en verde. Sin la cabecera,
ese segundo caso falla en la primera petición que cambia estado.

Si aun así ves un `403 csrf_token_mismatch`, casi siempre es una petición
repetida a mano fuera del orden de la colección, con una cookie vieja en el
tarro. Se arregla en un clic: **Cookies** (bajo el botón Send) → dominio
`localhost` → borra `mooc_session` y `mooc_csrf`, y vuelve a correr la carpeta 0
para que el login los emita de nuevo.

**Las consultas a Mailpit reintentan.** Mailpit indexa con un retardo pequeño
tras aceptar el SMTP, así que la primera búsqueda puede volver vacía. Reintentar
evita el falso negativo «no se envió el correo» a mitad de una grabación.

## Qué comprueba cada carpeta

| Carpeta | Lo que acredita |
|---|---|
| 0 · Preparación | La API responde; la cookie de sesión es `HttpOnly` y `SameSite` |
| 1 · Identidad | Alta con verificación por correo real; el profesor solo llega por invitación; un estudiante recibe 403 en autoría; todo queda en la bitácora |
| 2 · Autoría | Publicar incompleto devuelve **la lista entera** de motivos, no el primero; lo publicado es inmutable (409) |
| 3 · Multimedia | La URL de subida va **firmada, caducada y contra el almacén**: el binario no pasa por la API |
| 4 · Procesamiento | Estado de las colas y de la cola de fallidos |
| 5 · Consumo | El curso publicado sale en el catálogo y el borrador no; inscripción y entrega de contenido |
| 6 · Evaluación | **`is_correct` no aparece en el cuerpo**; guardado parcial que sobrevive a recargar; reenviar con la misma `Idempotency-Key` devuelve la misma nota |
| 7 · Progreso | El cuerpo no admite ningún porcentaje: lo calcula el servidor. Un latido implausible no acredita y **queda auditado** |
| 8 · Insignia | Verificación pública **sin correo ni id del estudiante**; credencial OB 3.0 firmada con Ed25519; la clave pública en el JWKS; revocación |
| 9 · Operación | Métricas agregadas, y que un estudiante no las ve |
| 99 · Límite de tasa | El limitador corta la fuerza bruta |

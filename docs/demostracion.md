# Guion de la demostración de aceptación

Esto no es un guion inventado: la sección 10.2 del enunciado fija los nueve
segmentos, qué evidencia aporta cada uno y contra qué criterio se evalúa. Lo que
sigue es esa tabla convertida en pasos ejecutables sobre este repositorio, con
una nota honesta en cada segmento sobre qué se puede demostrar hoy y qué no.

## Requisitos formales (sección 10.1)

- La demostración se ejecuta **con datos sintéticos sobre el sistema desplegado
  mediante Docker Compose**. No hace falta desplegar en un proveedor cloud: el
  enunciado pide exactamente esto.
- La evidencia incluye respuesta de la API, comportamiento de la interfaz,
  estado persistido y logs correlacionados.
- El pipeline de CI debe completar build, lint, análisis de seguridad,
  migraciones y pruebas **antes** de la demostración.

## Preparación

```bash
cp .env.example .env
# ADMIN_EMAIL y ADMIN_PASSWORD siembran el primer administrador.
docker compose --profile antivirus up -d clamav   # tarda: baja las firmas
CLAMAV_ADDR=clamav:3310 docker compose up -d --build
docker compose --profile carga run --rm seed      # datos sintéticos
```

El perfil `antivirus` importa: sin él opera el escáner integrado, que reconoce
el vector de prueba estándar pero no lleva firmas reales. El segmento 3 se ve
pobre sin ClamAV.

Superficies: interfaz en `http://localhost:3000`, API en `http://localhost:8080`
(a través del proxy), correo en `http://localhost:8025`, almacén en
`http://localhost:9101`.

## Los nueve segmentos

### 1. Identidad y administración
**Criterio:** identidad, autorización y seguridad.

Registro de estudiante → correo de verificación en Mailpit → inicio de sesión.
Invitación de profesor desde `/admin` (no hay autorregistro de profesores).
Suspensión de una cuenta y su entrada en la bitácora. Revocación de sesiones
desde `/cuenta/sesiones`: la otra pestaña muere en la siguiente petición.
Rechazo de una operación no autorizada (un estudiante llamando a un endpoint de
autoría) mostrando el 403 y su registro.

### 2. Autoría y publicación
**Criterio:** autoría y publicación.

Crear borrador → módulos, unidades y recursos → previsualización
(`/profesor/versiones/{id}/previsualizacion`) → intento de publicar incompleto,
que devuelve **la lista exhaustiva de motivos**, no el primero → completar y
publicar. Enseñar que la versión publicada es inmutable: el editor la bloquea.

### 3. Carga multimedia
**Criterio:** multimedia y distribución.

Subir un vídeo grande, **cortar la red a mitad**, recargar la página y volver a
elegir el mismo archivo: se reanuda desde donde iba, no desde cero. Enseñar la
URL prefirmada en el devtools (el binario no pasa por la API), el checksum
SHA-256, la validación de MIME real y el rechazo del vector EICAR.

### 4. Procesamiento y fallos
**Criterio:** arquitectura, multimedia, calidad operativa.

Estados del recurso (`pending → processing → ready`). Entrega duplicada del
mismo trabajo, que no produce dos salidas. Tres reintentos fallidos → DLQ
→ alerta → reencolado con la misma clave.

> Pendiente: no hay trazas OpenTelemetry. La correlación se enseña con
> `X-Request-Id` en los logs, que es menos de lo que pide el criterio.

### 5. Consumo de contenido
**Criterio:** multimedia, progreso y accesibilidad.

Inscripción → reproducción HLS con cambio de calidad → cerrar y volver, que
reanuda en el segundo donde se dejó → visor PDF → navegación completa por
teclado. Aprovechar para enseñar el cambio de idioma: la interfaz entera pasa a
inglés, fechas incluidas.

> La auditoría automática de accesibilidad cubre esta pantalla:
> `frontend/e2e/10-accesibilidad.spec.ts` la audita con axe en los dos idiomas.

### 6. Quiz
**Criterio:** evaluación académica.

Abrir un intento → enseñar en el devtools que **la respuesta correcta no llega
al cliente** → guardados parciales → recargar la página sin perder nada →
envío con `Idempotency-Key`, repetido, que devuelve la misma nota. Intento
expirado por límite de tiempo.

### 7. Progreso y aprobación
**Criterio:** progreso e insignias.

Heartbeats y permanencia mínima. **Enviar un porcentaje falso desde el cliente
y verlo rechazado y auditado.** Porcentaje de obligatorios, transición a
`completed` y luego a `approved`.

### 8. Insignia y actualización
**Criterio:** autoría, progreso e insignias.

Emisión única al aprobar → URL pública de verificación **sin correo del
estudiante** → credencial Open Badges 3.0 firmada → revocación auditada.
Después: borrador de actualización, clasificación de cambios (menor/mayor) y
publicación conservando el progreso por `stable_id`.

### 9. Operación
**Criterio:** arquitectura y calidad operativa.

```bash
docker compose up -d --scale api=3        # el proxy reparte solo
docker compose --profile carga run --rm k6
```

Enseñar el p95 por escenario y los umbrales cumpliéndose. Ver
`load/README.md` para los umbrales, por qué son esos y qué pasó al subir el
escalón a 1.000 VU.

> Pendiente, y hay que decirlo en el video: **backup, restauración y la prueba
> de RPO ≤ 15 min / RTO ≤ 4 h no están**. Tampoco la inyección de fallos
> durante la carga.

## Estado real frente a la condición de aceptación

La sección 10 exige cuatro cosas para aceptar. Las cuatro están:

| Condición | Estado |
|---|---|
| Los nueve flujos críticos superan pruebas **E2E** | 34 pruebas en `frontend/e2e/`, una por segmento, contra la plataforma levantada |
| La **prueba de carga** de Etapa 1 sin incumplimientos críticos | Ejecutada: 42.175 peticiones, 0 % de error, p95 de 3 a 7 ms |
| La **auditoría de accesibilidad** sin incumplimientos críticos | axe-core sobre WCAG 2.2 A y AA en 13 pantallas, español e inglés: cero violaciones |
| **CI** completo antes de la demostración | `.github/workflows/ci.yml` corre los cinco pasos que pide 10.1 |

Para reproducirlo:

```bash
cd frontend && npm run e2e && npm run e2e:informe
```

### Qué decir en el vídeo, y qué no

Las pruebas encontraron tres defectos reales en su primera pasada, y merece la
pena contarlo: es la mejor evidencia de que no son decorativas. Un profesor no
podía añadir ningún recurso (el editor mandaba PascalCase y la API espera
snake_case, así que todo devolvía 400); la casilla de «opción correcta» de un
quiz no tenía etiqueta, de modo que un profesor con lector de pantalla no sabía
cuál marcar; y el botón de mostrar contraseña quedaba en 3,67:1 de contraste,
por debajo del mínimo AA. Los tres están corregidos y con una prueba que falla
si vuelven.

Lo que **no** conviene afirmar en el vídeo:

- Que la plataforma «aguanta 2.000 concurrentes». No se ha medido. Lo medido
  es 200 VU con dos órdenes de magnitud de margen, que sugiere holgura pero no
  la acredita.
- Que la plataforma «es accesible». Lo acreditado es que no tiene los defectos
  que una máquina detecta. axe cubre del orden de la mitad de los problemas
  reales; el orden de foco y el recorrido con lector de pantalla siguen
  necesitando a una persona.
- Que la recuperación cumple RPO ≤ 15 min y RTO ≤ 4 h. Eso sigue sin probarse,
  y está dicho en el segmento 9.


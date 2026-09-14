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

> Pendiente: la auditoría automática de accesibilidad (axe / Lighthouse) es
> condición de aceptación y todavía no existe.

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

Enseñar el p95 por escenario y los umbrales cumpliéndose (o no). Ver
`load/README.md` para los umbrales y por qué son esos.

> Pendiente, y hay que decirlo en el video: **backup, restauración y la prueba
> de RPO ≤ 15 min / RTO ≤ 4 h no están**. Tampoco la inyección de fallos
> durante la carga.

## Estado real frente a la condición de aceptación

La sección 10 exige tres cosas para aceptar. Hoy:

| Condición | Estado |
|---|---|
| Los nueve flujos críticos superan pruebas **E2E** | **No.** Hay 259 pruebas de backend y 29 de frontend, pero llegan hasta la API, no hasta el navegador |
| La **prueba de carga** de Etapa 1 sin incumplimientos críticos | Escrita y con umbrales; **falta ejecutarla** |
| La **auditoría de accesibilidad** sin incumplimientos críticos | **No existe** |
| **CI** completo antes de la demostración | **No existe** |

Presentar el video sin esto es posible —los nueve segmentos se pueden grabar a
mano sobre el sistema desplegado—, pero conviene decir en el propio video qué
está cubierto por pruebas automáticas y qué se está enseñando a mano. Es la
diferencia entre una demostración y una demostración acreditada.

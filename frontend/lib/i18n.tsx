"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";

// Internacionalización de la interfaz (alcance opcional 5.2).
//
// El catálogo va tipado contra las claves del español, que es el idioma de
// origen: así añadir un texto sin traducirlo es un error de compilación y no
// una cadena que aparece en español dentro de una pantalla en inglés.
//
// Lo que no se traduce aquí es el contenido de los cursos. Ese lo escribe cada
// profesor y ya tiene su propio idioma declarado en la versión
// (`course_versions.language`): traducirlo automáticamente sería inventar
// material didáctico. Por el mismo motivo se dejan sin traducir los valores
// que vienen de la base y se enseñan tal cual —el rol de una cuenta, el estado
// de un intento, la acción de una entrada de auditoría—: son identificadores
// del dominio, y traducirlos rompería la correspondencia con lo que la API
// acepta y con lo que el administrador escribe en el filtro.

export const IDIOMAS = ["es", "en"] as const;
export type Idioma = (typeof IDIOMAS)[number];

export const NOMBRES_DE_IDIOMA: Record<Idioma, string> = {
  es: "Español",
  en: "English",
};

/**
 * LOCALES es la etiqueta con la que se formatean fechas y horas.
 *
 * Va aparte del idioma porque no coinciden: el catálogo distingue "es" de
 * "en", pero una fecha necesita región para elegir entre 14/09/2026 y
 * 9/14/2026, y equivocarse ahí no es un detalle estético.
 */
export const LOCALES: Record<Idioma, string> = {
  es: "es-CO",
  en: "en-US",
};

const es = {
  "nav.catalogo": "Catálogo",
  "nav.misCursos": "Mis cursos",
  "nav.misInsignias": "Mis insignias",
  "nav.autoria": "Autoría",
  "nav.administracion": "Administración",
  "nav.sesiones": "Mis sesiones",
  "nav.entrar": "Entrar",
  "nav.registrarse": "Crear cuenta",
  "nav.salir": "Salir",
  "nav.idioma": "Idioma",
  "nav.principal": "Principal",
  "nav.saltar": "Saltar al contenido principal",

  "catalogo.titulo": "Catálogo de cursos",
  "catalogo.subtitulo": "Cursos publicados y abiertos a inscripción.",
  "catalogo.buscar": "Buscar cursos",
  "catalogo.buscarPlaceholder": "Título o resumen",
  "catalogo.categoria": "Categoría",
  "catalogo.todas": "Todas",
  "catalogo.nivel": "Nivel",
  "catalogo.todos": "Todos",
  "catalogo.buscarBoton": "Buscar",
  "catalogo.limpiar": "Limpiar filtros",
  "catalogo.cargando": "Cargando cursos…",
  "catalogo.error": "No se pudo cargar el catálogo",
  "catalogo.sinResultados": "No hay cursos publicados que coincidan con la búsqueda.",
  "catalogo.pruebaOtro": "Prueba con otro término o quita algún filtro.",
  "catalogo.unCurso": "1 curso",
  "catalogo.nCursos": "{n} cursos",
  "catalogo.conFiltros": " con los filtros aplicados",
  "catalogo.publicados": " publicados",

  "cuenta.exportar": "Descargar mis datos",
  "cuenta.exportarDescripcion":
    "Un archivo con todo lo que la plataforma guarda sobre ti: cuenta, sesiones, inscripciones, progreso, evaluaciones e insignias.",
  "cuenta.exportando": "Preparando la descarga…",
  "cuenta.exportarError": "No se pudieron descargar tus datos.",

  "coautoria.titulo": "Coautoría",
  "coautoria.descripcion":
    "Otros profesores que pueden editar este curso. Editan el contenido, pero no pueden invitar ni quitar a nadie.",
  "coautoria.correo": "Correo del profesor",
  "coautoria.invitar": "Dar acceso",
  "coautoria.quitar": "Quitar",
  "coautoria.ninguno": "Nadie más tiene acceso a este curso.",
  "coautoria.noEsProfesor":
    "No se pudo dar acceso a ese correo. Solo las cuentas de profesor activas pueden coeditar.",
  "coautoria.errorQuitar": "No se pudo quitar el acceso",

  "comun.cargando": "Cargando…",
  "comun.cerrar": "Cerrar",
  "comun.guardar": "Guardar",
  "comun.guardando": "Guardando…",
  "comun.cancelar": "Cancelar",
  "comun.ver": "Ver",
  "comun.quitar": "Quitar",
  "comun.eliminar": "Eliminar",
  "comun.titulo": "Título",
  "comun.buscar": "Buscar",

  "auth.correo": "Correo electrónico",
  "auth.contrasena": "Contraseña",
  "auth.nombreCompleto": "Nombre completo",
  "auth.mostrar": "Mostrar",
  "auth.ocultar": "Ocultar",
  "auth.irALogin": "Ir a iniciar sesión",

  "auth.login.titulo": "Iniciar sesión",
  "auth.login.subtitulo": "Entra con el correo y la clave de tu cuenta.",
  "auth.login.enviando": "Ingresando…",
  "auth.login.enviar": "Ingresar",
  "auth.login.olvide": "¿Olvidaste tu contraseña?",
  "auth.login.sinCuenta": "¿No tienes cuenta?",
  "auth.login.registrate": "Regístrate",
  "auth.login.error": "No se pudo iniciar sesión",

  "auth.registro.titulo": "Crear cuenta de estudiante",
  "auth.registro.subtitulo":
    "El registro público crea cuentas de estudiante. Los profesores se dan de alta por administración.",
  "auth.registro.minimo": "Mínimo 10 caracteres",
  "auth.registro.enviando": "Creando cuenta…",
  "auth.registro.enviar": "Registrarme",
  "auth.registro.yaTienes": "¿Ya tienes cuenta?",
  "auth.registro.iniciaSesion": "Inicia sesión",
  "auth.registro.error": "No se pudo completar el registro",
  "auth.registro.revisaTitulo": "Revisa tu correo",
  "auth.registro.revisaTexto":
    "Si {email} está disponible, enviamos un enlace de verificación. Confírmalo para poder iniciar sesión. En desarrollo, revisa Mailpit en http://localhost:8025.",

  "auth.verificar.titulo": "Verificación de correo",
  "auth.verificar.enCurso": "Verificando…",
  "auth.verificar.ok": "Tu correo fue verificado. Ya puedes iniciar sesión.",
  "auth.verificar.sinToken": "Falta el token de verificación en el enlace.",
  "auth.verificar.error": "No se pudo verificar el correo",

  "auth.reset.titulo": "Restablecer contraseña",
  "auth.reset.enviado":
    "Si el correo existe, enviamos un enlace para restablecer la contraseña. Revisa Mailpit en desarrollo (http://localhost:8025).",
  "auth.reset.enviando": "Enviando…",
  "auth.reset.enviar": "Enviar enlace",
  "auth.reset.nueva": "Nueva contraseña",
  "auth.reset.actualizada":
    "Tu contraseña fue actualizada. Todas tus sesiones anteriores fueron cerradas.",
  "auth.reset.error": "No se pudo restablecer la contraseña",

  "estado.active": "En curso",
  "estado.withdrawn": "Retirado",
  "estado.completed": "Completado",
  "estado.approved": "Aprobado",

  "misCursos.subtitulo": "Los cursos a los que estás inscrito y su estado.",
  "misCursos.requiereSesion": "Para ver tus cursos necesitas una sesión iniciada.",
  "misCursos.vacio": "Aún no te has inscrito a ningún curso.",
  "misCursos.explorar": "Explorar el catálogo",
  "misCursos.verCurso": "Ver curso",
  "misCursos.retirarme": "Retirarme",
  "misCursos.error": "No se pudieron cargar tus cursos",
  "misCursos.errorRetirar": "No se pudo retirar la inscripción",

  "curso.error": "No se pudo cargar el curso",
  "curso.cargando": "Cargando curso…",
  "curso.inscribirme": "Inscribirme",
  "curso.inscrito": "¡Inscripción realizada! Ya aparece en «Mis cursos».",
  "curso.errorInscripcion": "No se pudo completar la inscripción",
  "curso.tuAvance": "Tu avance",
  "curso.recursosCompletados":
    "{hechos} / {total} recursos obligatorios completados ({pct}%)",
  "curso.evaluacionesPendientes": "Evaluaciones pendientes de aprobar: {n}",
  "curso.insigniaObtenida": "¡Insignia obtenida!",
  "curso.verVerificacion": "Ver verificación pública",
  "curso.contenido": "Contenido",
  "curso.obligatorio": "obligatorio",

  "recurso.procesando": "Este recurso todavía se está procesando. Vuelve en unos minutos.",
  "recurso.noInscrito": "Tienes que inscribirte en el curso para ver esta lección.",
  "recurso.sinAcceso": "El recurso no existe o no tienes acceso a él.",
  "recurso.error": "No se pudo cargar el recurso",
  "recurso.cargando": "Cargando recurso…",
  "recurso.volver": "Volver al curso",
  "recurso.reanuda": "Se reanuda donde lo dejaste.",
  "recurso.descargarOriginal": "Descargar la presentación original",
  "recurso.abrirExterno": "Abrir el recurso externo",
  "recurso.abrirArchivo": "Abrir el archivo",
  "recurso.verTranscripcion": "Ver la transcripción",
  "recurso.ocultarTranscripcion": "Ocultar la transcripción",
  "recurso.cargandoTranscripcion": "Cargando la transcripción…",
  "recurso.sinTranscripcion": "Este material todavía no tiene transcripción.",

  "media.audio": "Audio: {titulo}",
  "media.video": "Video: {titulo}",

  "pdf.documento": "Documento: {titulo}",
  "pdf.abrir": "Abrir el documento en una pestaña nueva",
  "pdf.descargable": " · Desde ahí puedes descargarlo.",
  "pdf.noDescargable": " · El autor no habilitó la descarga.",

  "marco.noAutorizado":
    "Este contenido incrustado no se puede mostrar porque el servidor no autorizó su presentación.",
  "marco.cargando": "Cargando el contenido incrustado…",
  "marco.origen": "Contenido incrustado desde",
  "marco.sitioExterno": "un sitio externo",
  "marco.abrirPestana": "Abrirlo en una pestaña nueva",

  "foro.dudasLeccion": "Dudas sobre esta lección",
  "foro.titulo": "Foro del curso",
  "foro.tituloPlaceholder": "¿Cómo se calcula el progreso?",
  "foro.mensaje": "Mensaje",
  "foro.abrirHilo": "Abrir hilo",
  "foro.errorAbrir": "No se pudo abrir el hilo",
  "foro.vacio": "Todavía no hay conversaciones. Empieza tú.",
  "foro.cerrado": "cerrado",
  "foro.unaRespuesta": "1 respuesta",
  "foro.nRespuestas": "{n} respuestas",
  "foro.eliminado": "Mensaje eliminado.",
  "foro.borrar": "Borrar",
  "foro.errorBorrar": "No se pudo borrar",
  "foro.responder": "Responder",
  "foro.errorResponder": "No se pudo responder",
  "foro.hiloCerrado": "Este hilo está cerrado a nuevas respuestas.",
  "foro.reabrir": "Reabrir el hilo",
  "foro.cerrarHilo": "Cerrar el hilo",
  "foro.errorEstado": "No se pudo cambiar el estado del hilo",

  "quiz.errorAbrir": "No se pudo abrir el intento",
  "quiz.abriendo": "Abriendo el intento…",
  "quiz.errorGuardarRespuesta": "No se pudo guardar la respuesta",
  "quiz.errorEnviar": "No se pudo enviar el intento",
  "quiz.confirmarFaltantes": "Faltan {n} pregunta(s) sin responder. ¿Enviar de todas formas?",
  "quiz.intentoExpirado": "Intento expirado.",
  "quiz.intentoCalificado": "Intento calificado.",
  "quiz.nota": "Nota: {nota} / 100.",
  "quiz.aprobaste": "Aprobaste.",
  "quiz.noAprobaste": "No alcanzaste el mínimo para aprobar.",
  "quiz.unPunto": "1 punto",
  "quiz.nPuntos": "{n} puntos",
  "quiz.enviando": "Enviando…",
  "quiz.enviar": "Enviar evaluación",

  "insignias.subtitulo": "Insignias verificables emitidas al aprobar un curso.",
  "insignias.error": "No se pudieron cargar tus insignias",
  "insignias.requiereSesion": "Para ver tus insignias necesitas una sesión iniciada.",
  "insignias.vacio": "Todavía no tienes ninguna insignia.",
  "insignias.vacioAccion":
    "Aprueba las evaluaciones obligatorias de un curso para obtener la tuya.",
  "insignias.alt": "Insignia emitida el {fecha}",
  "insignias.vigente": "vigente",
  "insignias.revocada": "revocada",
  "insignias.emitidaEl": "Emitida el {fecha}",
  "insignias.codigoVerificacion": "Código de verificación:",

  "verificacion.titulo": "Verificación de insignia",
  "verificacion.noExiste": "No existe ninguna insignia con este código.",
  "verificacion.error": "No se pudo verificar la insignia.",
  "verificacion.enCurso": "Verificando…",
  "verificacion.valida": "Esta insignia es válida.",
  "verificacion.revocada": "Esta insignia fue revocada y ya no es válida.",
  "verificacion.alt": "Insignia del curso {curso}",
  "verificacion.altSinCurso": "Insignia verificada",
  "verificacion.codigo": "Código",
  "verificacion.curso": "Curso",
  "verificacion.emitida": "Emitida el",
  "verificacion.revocadaEl": "Revocada el",
  "verificacion.credencial": "Ver la credencial Open Badges 3.0",
  "verificacion.firmada": "Descargarla firmada",

  "sesiones.cargando": "Cargando sesiones…",
  "sesiones.titulo": "Sesiones activas",
  "sesiones.subtitulo":
    "Cada dispositivo con la sesión abierta. Revocar una la invalida de inmediato.",
  "sesiones.cerrarLasDemas": "Cerrar las demás ({n})",
  "sesiones.esteDispositivo": "Este dispositivo",
  "sesiones.otroDispositivo": "Otro dispositivo",
  "sesiones.iniciada": "Iniciada: {fecha}",
  "sesiones.vence": "Vence: {fecha}",
  "sesiones.cerrarEsta": "Cerrar esta sesión",
  "sesiones.revocar": "Revocar",
  "sesiones.error": "No se pudieron cargar las sesiones",
  "sesiones.errorRevocar": "No se pudo revocar la sesión",
  "sesiones.errorRevocarTodas": "No se pudieron revocar las sesiones",

  "profesor.titulo": "Autoría de cursos",
  "profesor.subtitulo":
    "Crea borradores y publica versiones. Una versión publicada es inmutable.",
  "profesor.nuevoCurso": "Nuevo curso",
  "profesor.nuevoCursoAyuda": "El curso nace como borrador editable.",
  "profesor.slug": "Slug (identificador en la URL)",
  "profesor.slugPlaceholder": "introduccion-a-cloud",
  "profesor.creando": "Creando…",
  "profesor.crearBorrador": "Crear borrador",
  "profesor.sinCursos": "Aún no has creado ningún curso.",
  "profesor.publicado": "Publicado",
  "profesor.sinPublicar": "Sin publicar",
  "profesor.verPublicada": "Ver versión publicada",
  "profesor.publicadoAyuda":
    "Un curso publicado no se edita. Despublícalo para abrir un borrador: saldrá del catálogo hasta que vuelvas a publicar, y quienes ya están inscritos conservan su progreso.",
  "profesor.editarBorrador": "Editar borrador",
  "profesor.crearVersion": "Crear nueva versión",
  "profesor.despublicarParaEditar": "Despublicar para editar",
  "profesor.despublicando": "Despublicando…",
  "profesor.confirmarDespublicar":
    "Despublicar «{slug}» lo retirará del catálogo hasta que publiques la nueva versión. ¿Continuar?",
  "profesor.error": "No se pudieron cargar tus cursos",
  "profesor.errorCrear": "No se pudo crear el curso",
  "profesor.errorBorrador": "No se pudo abrir el borrador",
  "profesor.errorDespublicar": "No se pudo despublicar el curso",

  "version.error": "No se pudo cargar la versión",
  "version.publicada": "¡Versión publicada!",
  "version.errorPublicar": "No se pudo publicar",
  "version.sinTitulo": "(sin título)",
  "version.previsualizar": "Previsualizar",
  "version.etiqueta": "Versión {n} · {estado}",
  "version.inmutable":
    "Esta versión está publicada y es inmutable. Para editarla hay que despublicar el curso primero.",
  "version.irADespublicar": "Ir a mis cursos para despublicarlo",
  "version.motivos": "No se pudo publicar. Motivos:",
  "version.publicar": "Publicar versión",
  "version.estructura": "Estructura",
  "version.metadatos": "Metadatos",
  "version.resumen": "Resumen",
  "version.descripcion": "Descripción (Markdown)",
  "version.categoria": "Categoría",
  "version.nivel": "Nivel",
  "version.idioma": "Idioma",
  "version.notaMinima": "Nota mínima de aprobación (%)",
  "version.pctObligatorios": "% de recursos obligatorios requeridos",
  "version.guardarMetadatos": "Guardar metadatos",
  "version.errorGuardar": "No se pudo guardar",
  "version.nuevoModulo": "Nuevo módulo",
  "version.agregarModulo": "Agregar módulo",
  "version.errorModulo": "No se pudo agregar el módulo",
  "version.eliminarModulo": "Eliminar módulo",
  "version.confirmarModulo": "¿Eliminar el módulo «{titulo}» y todo su contenido?",
  "version.nuevaUnidad": "Nueva unidad",
  "version.agregarUnidad": "Agregar unidad",
  "version.eliminarUnidad": "Eliminar unidad",
  "version.confirmarUnidad": "¿Eliminar la unidad «{titulo}» y sus recursos?",
  "version.confirmarRecurso": "¿Eliminar el recurso «{titulo}»?",
  "version.procesamiento": "procesamiento: {estado}",
  "version.oculto": "oculto",
  "version.definirEvaluacion": "Definir evaluación",
  "version.subirArchivo": "Subir archivo",
  "version.reanudarSubida": "Reanudar subida",
  "version.subiendoAria": "Subiendo {titulo}",
  "version.cargaPendiente":
    "Hay una subida sin terminar. Vuelve a elegir el mismo archivo y continuará desde donde se quedó.",
  "version.empezarDeNuevo": "Empezar de nuevo",
  "version.editarContenido": "Editar contenido",
  "version.guardarRevision": "Guardar revisión",
  "version.guardado": "Guardado",
  "version.tipoRecurso": "Tipo de recurso",
  "version.tituloRecurso": "Título del recurso",
  "version.contenidoBloques": "Contenido del recurso (bloques)",
  "version.url": "URL",
  "version.visible": "Visible",
  "version.obligatorio": "Obligatorio",
  "version.agregarRecurso": "Agregar recurso",
  "version.errorRecurso": "No se pudo agregar el recurso",
  "version.sinDestinos":
    "No hay dominios autorizados para incrustar. Pide a la administración que añada el que necesitas.",
  "version.destinos": "Dominios autorizados: {hosts}.",

  "tipo.text": "Texto enriquecido",
  "tipo.image": "Imagen",
  "tipo.video": "Video",
  "tipo.audio": "Audio",
  "tipo.pdf": "PDF",
  "tipo.presentation": "Presentación",
  "tipo.file": "Archivo descargable",
  "tipo.iframe": "Iframe autorizado",
  "tipo.link": "Enlace externo",
  "tipo.quiz": "Quiz",

  "cambios.identico": "Este borrador es idéntico a la versión publicada.",
  "cambios.titulo": "Cambios respecto a lo publicado",
  "cambios.mayor":
    "Esta actualización cambia lo que hay que completar para aprobar. Al publicarla, el avance de los inscritos se recalcula sobre la nueva lista de recursos obligatorios. Lo que ya llevaban hecho se conserva, y a nadie se le retira una aprobación ya obtenida.",
  "cambios.menor":
    "Cambia el contenido o la presentación, pero no lo que hay que completar: el avance de los inscritos sigue midiéndose igual.",
  "cambios.afectaProgreso": "afecta al progreso",

  "historial.titulo": "Historial de revisiones",
  "historial.error": "No se pudo cargar el historial",
  "historial.errorAbrir": "No se pudo abrir la revisión",
  "historial.errorRestaurar": "No se pudo restaurar",
  "historial.confirmar": "¿Restaurar la revisión {n}? Se guardará como una revisión nueva.",
  "historial.vacio": "Todavía no hay revisiones guardadas de este recurso.",
  "historial.guardadaPor": "Guardada por {autor}",
  "historial.restaurar": "Restaurar",
  "historial.revision": "Revisión #{n}",

  "previsualizacion.titulo": "Previsualización",
  "previsualizacion.error": "No se pudo cargar la previsualización",
  "previsualizacion.cargando": "Cargando previsualización…",
  "previsualizacion.volver": "Volver a la edición",
  "previsualizacion.intro": "Así verá el curso quien se inscriba. Versión {n} ·",
  "previsualizacion.enProceso":
    "{n} recurso(s) todavía en procesamiento. No se puede publicar hasta que terminen.",
  "previsualizacion.ocultos": "{n} recurso(s) ocultos no se muestran aquí.",
  "previsualizacion.sinTitulo": "Sin título",
  "previsualizacion.aprobacion":
    "Aprobación: {nota}% de nota mínima y {pct}% de los recursos obligatorios.",
  "previsualizacion.sinModulos": "El borrador aún no tiene módulos.",
  "previsualizacion.sinVisibles": "Sin recursos visibles.",

  "quizAutoria.noEncontrado": "No se encontró el recurso en esta versión.",
  "quizAutoria.errorVersion": "No se pudo cargar la versión",
  "quizAutoria.guardada": "Evaluación guardada.",
  "quizAutoria.errorGuardar": "No se pudo guardar la evaluación",
  "quizAutoria.titulo": "Evaluación: {recurso}",
  "quizAutoria.volver": "Volver a la versión",
  "quizAutoria.aviso":
    "Guardar reemplaza por completo la evaluación anterior de este recurso. Los intentos ya calificados no se ven afectados: conservan su propio snapshot de preguntas y opciones.",
  "quizAutoria.notaMinima": "Nota mínima para aprobar (%)",
  "quizAutoria.limiteTiempo": "Límite de tiempo (minutos, opcional)",
  "quizAutoria.intentosMaximos": "Intentos máximos (opcional)",
  "quizAutoria.retroalimentacion": "Retroalimentación",
  "quizAutoria.inmediata": "Inmediata",
  "quizAutoria.alEnviar": "Al enviar el intento",
  "quizAutoria.alCerrar": "Al cerrarse el quiz",
  "quizAutoria.nunca": "Nunca",
  "quizAutoria.mezclar": "Mezclar el orden de las preguntas",
  "quizAutoria.preguntas": "Preguntas",
  "quizAutoria.pregunta": "Pregunta {n}",
  "quizAutoria.eliminarPregunta": "Eliminar pregunta",
  "quizAutoria.enunciado": "Enunciado",
  "quizAutoria.tipo": "Tipo",
  "quizAutoria.unica": "Respuesta única",
  "quizAutoria.multiple": "Respuesta múltiple",
  "quizAutoria.puntos": "Puntos",
  "quizAutoria.opcionesUna": "Opciones (marca la correcta):",
  "quizAutoria.opcionesVarias": "Opciones (marca las correctas):",
  "quizAutoria.opcion": "Opción {n}",
  "quizAutoria.marcarCorrecta": "Marcar la opción {n} de la pregunta {pregunta} como correcta",
  "quizAutoria.textoDeOpcion": "Texto de la opción {n} de la pregunta {pregunta}",
  "quizAutoria.agregarOpcion": "Agregar opción",
  "quizAutoria.agregarPregunta": "Agregar pregunta",
  "quizAutoria.guardar": "Guardar evaluación",

  "resultados.titulo": "Resultados",
  "resultados.sinIntentos": "Todavía no la ha presentado nadie.",
  "resultados.unIntento": "1 intento",
  "resultados.nIntentos": "{n} intentos",
  "resultados.unEstudiante": "1 estudiante",
  "resultados.nEstudiantes": "{n} estudiantes",
  "resultados.de": "de",
  "resultados.notaMedia": "Nota media: {n}",
  "resultados.mediana": "Mediana: {n}",
  "resultados.aprobados": "Aprobados: {n}%",
  "resultados.acierto": "Acierto: {n}%",
  "resultados.enBlanco": "En blanco: {n}",
  "resultados.malPlanteada":
    "La acierta menos de un tercio: puede estar mal planteada, no solo ser difícil.",
  "resultados.correcta": "correcta",

  "admin.subtitulo": "Cuentas, roles, sesiones y la bitácora inmutable de la plataforma.",
  "admin.errorUsuarios": "No se pudo cargar la lista de usuarios",
  "admin.errorRol": "No se pudo actualizar el rol",
  "admin.errorEstado": "No se pudo actualizar el estado",
  "admin.profesorCreado":
    "Profesor creado. Se le envió un correo para establecer su contraseña.",
  "admin.usuarios": "Usuarios",
  "admin.usuariosAyuda":
    "Cambia el rol o el estado de una cuenta, y revisa sus sesiones abiertas.",
  "admin.buscarUsuario": "Buscar por nombre o correo",
  "admin.colNombre": "Nombre",
  "admin.colCorreo": "Correo",
  "admin.colRol": "Rol",
  "admin.colEstado": "Estado",
  "admin.colSesiones": "Sesiones",
  "admin.rolDe": "Rol de {nombre}",
  "admin.estadoDe": "Estado de {nombre}",
  "admin.errorMetricas": "No se pudieron cargar las métricas",
  "admin.metricas": "Métricas de la plataforma",
  "admin.metricaUsuarios": "Usuarios",
  "admin.metricaCursos": "Versiones de curso",
  "admin.metricaInscripciones": "Inscripciones",
  "admin.metricaInsignias": "Insignias",
  "admin.metricaMultimedia": "Multimedia",
  "admin.metricaEvaluaciones": "Intentos de evaluación",
  "admin.sinRegistros": "Sin registros todavía.",
  "admin.errorDestinos": "No se pudo cargar la lista de destinos",
  "admin.errorAutorizar": "No se pudo autorizar el destino",
  "admin.errorQuitarDestino": "No se pudo quitar el destino",
  "admin.confirmarQuitarDestino":
    "¿Quitar {host} de la lista? Los recursos ya publicados que lo usen dejarán de mostrarse.",
  "admin.destinos": "Destinos incrustables",
  "admin.destinosAyuda":
    "Solo estos dominios pueden aparecer en un recurso de tipo iframe. Todo lo demás se rechaza al guardarlo y al servirlo.",
  "admin.dominio": "Dominio",
  "admin.permisos": "Permisos concedidos",
  "admin.paraQue": "Para qué",
  "admin.paraQuePlaceholder": "Reproductor de vídeo",
  "admin.incluirSubdominios": "Incluir subdominios",
  "admin.autorizar": "Autorizar",
  "admin.avisoSubdominios":
    "Incluir subdominios autoriza también los que ese tercero cree en el futuro, incluido cualquiera que aloje contenido de sus usuarios.",
  "admin.sinDestinos": "No hay ningún destino autorizado.",
  "admin.sinDestinosAyuda":
    "Mientras la lista esté vacía, no se puede guardar ningún recurso incrustado.",
  "admin.ySubdominios": "y subdominios",
  "admin.sinPermisosExtra": "sin permisos extra",
  "admin.sandboxAntes": "Todo marco se sirve con",
  "admin.sandboxDespues":
    ", que le niega navegar la ventana principal, abrir descargas y mostrar diálogos modales.",
  "admin.sesionesActivas": "{n} activas",
  "admin.cerrarTodas": "Cerrar todas",
  "admin.errorAuditoria": "No se pudo cargar la auditoría",
  "admin.auditoria": "Auditoría",
  "admin.auditoriaAyuda": "Registro inmutable: la base rechaza modificarlo o borrarlo.",
  "admin.filtrarAccion": "Filtrar por acción",
  "admin.filtrar": "Filtrar",
  "admin.sinEntradas": "Sin entradas para ese filtro.",
  "admin.colCuando": "Cuándo",
  "admin.colAccion": "Acción",
  "admin.colActor": "Actor",
  "admin.colEntidad": "Entidad",
  "admin.colIP": "IP",
  "admin.invitarProfesor": "Invitar profesor",
  "admin.invitarAyuda":
    "Los profesores no se autorregistran: reciben un enlace para fijar su clave.",
  "admin.errorProfesor": "No se pudo crear el profesor",
  "admin.crearProfesor": "Crear profesor",

  "editor.titulo": "Editor de bloques (Markdown canónico)",
  "editor.borradorGuardado": "Borrador guardado",
  "editor.guardando": "Guardando…",
  "editor.autoguardado": "Autoguardado a las {hora}",
  "editor.errorAutoguardar": "Error al autoguardar",
  "editor.moverArriba": "Mover arriba",
  "editor.moverAbajo": "Mover abajo",
  "editor.eliminarBloque": "Eliminar bloque",
  "editor.encabezadoPlaceholder": "Texto del encabezado…",
  "editor.codigoPlaceholder": "// Código aquí…",
  "editor.lenguajePlaceholder": "lenguaje (ej: python, go)",
  "editor.calloutPlaceholder": "Nota importante o advertencia…",
  "editor.formulaAntes": "Notación LaTeX. Se guarda entre",
  "editor.formulaDespues": ", que es lo que reconocen los renderizadores de fórmulas.",
  "editor.listaPlaceholder": "Un elemento por línea…",
  "editor.parrafoPlaceholder": "Escribe el contenido del párrafo…",
  "editor.parrafo": "+ Párrafo",
  "editor.encabezado": "+ Encabezado",
  "editor.codigo": "+ Código",
  "editor.nota": "+ Nota / Cita",
  "editor.lista": "+ Lista",
  "editor.tareas": "+ Tareas",
  "editor.tabla": "+ Tabla",
  "editor.formula": "+ Fórmula",
  "editor.limpiar": "Limpiar borrador",
  "editor.columnaN": "Columna {n}",
  "editor.primeraTarea": "Primera tarea",
  "editor.encabezadoColumna": "Encabezado de la columna {n}",
  "editor.celda": "Fila {fila}, columna {columna}",
  "editor.masFila": "+ Fila",
  "editor.masColumna": "+ Columna",
  "editor.menosFila": "− Fila",
  "editor.primeraFilaEncabezado": "La primera fila es el encabezado.",
  "editor.marcarHecha": "Marcar «{tarea}» como hecha",
  "editor.tareaN": "tarea {n}",
  "editor.quitarTarea": "Quitar la tarea {n}",
  "editor.descripcionTarea": "Descripción de la tarea",
  "editor.masTarea": "+ Tarea",

  "carga.fase.huella": "Calculando la huella del archivo…",
  "carga.fase.preparando": "Preparando la subida…",
  "carga.fase.subiendo": "Subiendo…",
  "carga.fase.verificando": "Verificando integridad y seguridad…",
  "carga.fase.encolado": "En cola de procesamiento",
  "carga.fase.listo": "Archivo listo",
  "carga.fase.retomando": "Retomando la subida interrumpida…",
  "carga.fase.iniciando": "Iniciando la subida…",
  "carga.fase.parteSubida": "La parte {numero} de {total} ya estaba subida",
  "carga.fase.parteEnCurso": "Subiendo la parte {numero} de {total}…",
  "carga.error.subida": "La subida falló (HTTP {status}).",
  "carga.error.parte":
    "Falló la parte {numero} (HTTP {status}). Vuelve a intentarlo: se reanudará aquí.",
  "carga.error.etag":
    "El almacenamiento no expuso el ETag de la parte. Añade ETag a Access-Control-Expose-Headers en MinIO o el CDN.",
  "carga.error.generico": "No se pudo subir el archivo",
} as const;

/** Clave es cualquiera de los textos del catálogo de origen. */
export type Clave = keyof typeof es;

// El catálogo en inglés se declara con el mismo tipo, así que falta una clave
// o sobra una inventada, TypeScript lo dice.
const en: Record<Clave, string> = {
  "nav.catalogo": "Catalog",
  "nav.misCursos": "My courses",
  "nav.misInsignias": "My badges",
  "nav.autoria": "Authoring",
  "nav.administracion": "Administration",
  "nav.sesiones": "My sessions",
  "nav.entrar": "Sign in",
  "nav.registrarse": "Create account",
  "nav.salir": "Sign out",
  "nav.idioma": "Language",
  "nav.principal": "Main",
  "nav.saltar": "Skip to the main content",

  "catalogo.titulo": "Course catalog",
  "catalogo.subtitulo": "Published courses open for enrollment.",
  "catalogo.buscar": "Search courses",
  "catalogo.buscarPlaceholder": "Title or summary",
  "catalogo.categoria": "Category",
  "catalogo.todas": "All",
  "catalogo.nivel": "Level",
  "catalogo.todos": "All",
  "catalogo.buscarBoton": "Search",
  "catalogo.limpiar": "Clear filters",
  "catalogo.cargando": "Loading courses…",
  "catalogo.error": "Could not load the catalog",
  "catalogo.sinResultados": "No published courses match your search.",
  "catalogo.pruebaOtro": "Try another term or remove a filter.",
  "catalogo.unCurso": "1 course",
  "catalogo.nCursos": "{n} courses",
  "catalogo.conFiltros": " matching the filters",
  "catalogo.publicados": " published",

  "cuenta.exportar": "Download my data",
  "cuenta.exportarDescripcion":
    "A file with everything the platform stores about you: account, sessions, enrollments, progress, assessments and badges.",
  "cuenta.exportando": "Preparing the download…",
  "cuenta.exportarError": "Your data could not be downloaded.",

  "coautoria.titulo": "Co-authoring",
  "coautoria.descripcion":
    "Other teachers who can edit this course. They edit the content, but cannot invite or remove anyone.",
  "coautoria.correo": "Teacher's email",
  "coautoria.invitar": "Grant access",
  "coautoria.quitar": "Remove",
  "coautoria.ninguno": "Nobody else has access to this course.",
  "coautoria.noEsProfesor":
    "Access could not be granted to that address. Only active teacher accounts can co-edit.",
  "coautoria.errorQuitar": "Could not remove the access",

  "comun.cargando": "Loading…",
  "comun.cerrar": "Close",
  "comun.guardar": "Save",
  "comun.guardando": "Saving…",
  "comun.cancelar": "Cancel",
  "comun.ver": "View",
  "comun.quitar": "Remove",
  "comun.eliminar": "Delete",
  "comun.titulo": "Title",
  "comun.buscar": "Search",

  "auth.correo": "Email address",
  "auth.contrasena": "Password",
  "auth.nombreCompleto": "Full name",
  "auth.mostrar": "Show",
  "auth.ocultar": "Hide",
  "auth.irALogin": "Go to sign in",

  "auth.login.titulo": "Sign in",
  "auth.login.subtitulo": "Sign in with your account's email and password.",
  "auth.login.enviando": "Signing in…",
  "auth.login.enviar": "Sign in",
  "auth.login.olvide": "Forgot your password?",
  "auth.login.sinCuenta": "Don't have an account?",
  "auth.login.registrate": "Sign up",
  "auth.login.error": "Could not sign in",

  "auth.registro.titulo": "Create a student account",
  "auth.registro.subtitulo":
    "Public registration creates student accounts. Teachers are enrolled by an administrator.",
  "auth.registro.minimo": "At least 10 characters",
  "auth.registro.enviando": "Creating account…",
  "auth.registro.enviar": "Sign up",
  "auth.registro.yaTienes": "Already have an account?",
  "auth.registro.iniciaSesion": "Sign in",
  "auth.registro.error": "Could not complete the registration",
  "auth.registro.revisaTitulo": "Check your email",
  "auth.registro.revisaTexto":
    "If {email} is available, we sent a verification link. Confirm it to be able to sign in. In development, check Mailpit at http://localhost:8025.",

  "auth.verificar.titulo": "Email verification",
  "auth.verificar.enCurso": "Verifying…",
  "auth.verificar.ok": "Your email was verified. You can sign in now.",
  "auth.verificar.sinToken": "The link is missing the verification token.",
  "auth.verificar.error": "Could not verify the email",

  "auth.reset.titulo": "Reset password",
  "auth.reset.enviado":
    "If the address exists, we sent a link to reset the password. In development, check Mailpit (http://localhost:8025).",
  "auth.reset.enviando": "Sending…",
  "auth.reset.enviar": "Send link",
  "auth.reset.nueva": "New password",
  "auth.reset.actualizada": "Your password was updated. All your previous sessions were closed.",
  "auth.reset.error": "Could not reset the password",

  "estado.active": "In progress",
  "estado.withdrawn": "Withdrawn",
  "estado.completed": "Completed",
  "estado.approved": "Passed",

  "misCursos.subtitulo": "The courses you are enrolled in and their status.",
  "misCursos.requiereSesion": "You need to be signed in to see your courses.",
  "misCursos.vacio": "You are not enrolled in any course yet.",
  "misCursos.explorar": "Browse the catalog",
  "misCursos.verCurso": "View course",
  "misCursos.retirarme": "Withdraw",
  "misCursos.error": "Could not load your courses",
  "misCursos.errorRetirar": "Could not withdraw the enrollment",

  "curso.error": "Could not load the course",
  "curso.cargando": "Loading course…",
  "curso.inscribirme": "Enroll",
  "curso.inscrito": "You are enrolled. It already shows up under “My courses”.",
  "curso.errorInscripcion": "Could not complete the enrollment",
  "curso.tuAvance": "Your progress",
  "curso.recursosCompletados": "{hechos} / {total} required resources completed ({pct}%)",
  "curso.evaluacionesPendientes": "Assessments still to pass: {n}",
  "curso.insigniaObtenida": "Badge earned.",
  "curso.verVerificacion": "View public verification",
  "curso.contenido": "Contents",
  "curso.obligatorio": "required",

  "recurso.procesando": "This resource is still being processed. Come back in a few minutes.",
  "recurso.noInscrito": "You have to enroll in the course to see this lesson.",
  "recurso.sinAcceso": "The resource does not exist or you do not have access to it.",
  "recurso.error": "Could not load the resource",
  "recurso.cargando": "Loading resource…",
  "recurso.volver": "Back to the course",
  "recurso.reanuda": "It resumes where you left off.",
  "recurso.descargarOriginal": "Download the original presentation",
  "recurso.abrirExterno": "Open the external resource",
  "recurso.abrirArchivo": "Open the file",
  "recurso.verTranscripcion": "Show the transcript",
  "recurso.ocultarTranscripcion": "Hide the transcript",
  "recurso.cargandoTranscripcion": "Loading the transcript…",
  "recurso.sinTranscripcion": "This material does not have a transcript yet.",

  "media.audio": "Audio: {titulo}",
  "media.video": "Video: {titulo}",

  "pdf.documento": "Document: {titulo}",
  "pdf.abrir": "Open the document in a new tab",
  "pdf.descargable": " · You can download it from there.",
  "pdf.noDescargable": " · The author did not enable downloads.",

  "marco.noAutorizado":
    "This embedded content cannot be shown because the server did not authorize its presentation.",
  "marco.cargando": "Loading the embedded content…",
  "marco.origen": "Content embedded from",
  "marco.sitioExterno": "an external site",
  "marco.abrirPestana": "Open it in a new tab",

  "foro.dudasLeccion": "Questions about this lesson",
  "foro.titulo": "Course forum",
  "foro.tituloPlaceholder": "How is progress calculated?",
  "foro.mensaje": "Message",
  "foro.abrirHilo": "Start thread",
  "foro.errorAbrir": "Could not open the thread",
  "foro.vacio": "No conversations yet. Start one.",
  "foro.cerrado": "closed",
  "foro.unaRespuesta": "1 reply",
  "foro.nRespuestas": "{n} replies",
  "foro.eliminado": "Message deleted.",
  "foro.borrar": "Delete",
  "foro.errorBorrar": "Could not delete",
  "foro.responder": "Reply",
  "foro.errorResponder": "Could not reply",
  "foro.hiloCerrado": "This thread is closed to new replies.",
  "foro.reabrir": "Reopen the thread",
  "foro.cerrarHilo": "Close the thread",
  "foro.errorEstado": "Could not change the thread's state",

  "quiz.errorAbrir": "Could not open the attempt",
  "quiz.abriendo": "Opening the attempt…",
  "quiz.errorGuardarRespuesta": "Could not save the answer",
  "quiz.errorEnviar": "Could not submit the attempt",
  "quiz.confirmarFaltantes": "{n} question(s) are unanswered. Submit anyway?",
  "quiz.intentoExpirado": "Attempt expired.",
  "quiz.intentoCalificado": "Attempt graded.",
  "quiz.nota": "Score: {nota} / 100.",
  "quiz.aprobaste": "You passed.",
  "quiz.noAprobaste": "You did not reach the passing score.",
  "quiz.unPunto": "1 point",
  "quiz.nPuntos": "{n} points",
  "quiz.enviando": "Submitting…",
  "quiz.enviar": "Submit assessment",

  "insignias.subtitulo": "Verifiable badges issued when a course is passed.",
  "insignias.error": "Could not load your badges",
  "insignias.requiereSesion": "You need to be signed in to see your badges.",
  "insignias.vacio": "You do not have any badge yet.",
  "insignias.vacioAccion": "Pass a course's required assessments to earn yours.",
  "insignias.alt": "Badge issued on {fecha}",
  "insignias.vigente": "valid",
  "insignias.revocada": "revoked",
  "insignias.emitidaEl": "Issued on {fecha}",
  "insignias.codigoVerificacion": "Verification code:",

  "verificacion.titulo": "Badge verification",
  "verificacion.noExiste": "There is no badge with this code.",
  "verificacion.error": "Could not verify the badge.",
  "verificacion.enCurso": "Verifying…",
  "verificacion.valida": "This badge is valid.",
  "verificacion.revocada": "This badge was revoked and is no longer valid.",
  "verificacion.alt": "Badge for the course {curso}",
  "verificacion.altSinCurso": "Verified badge",
  "verificacion.codigo": "Code",
  "verificacion.curso": "Course",
  "verificacion.emitida": "Issued on",
  "verificacion.revocadaEl": "Revoked on",
  "verificacion.credencial": "View the Open Badges 3.0 credential",
  "verificacion.firmada": "Download it signed",

  "sesiones.cargando": "Loading sessions…",
  "sesiones.titulo": "Active sessions",
  "sesiones.subtitulo":
    "Every device with an open session. Revoking one invalidates it immediately.",
  "sesiones.cerrarLasDemas": "Close the others ({n})",
  "sesiones.esteDispositivo": "This device",
  "sesiones.otroDispositivo": "Another device",
  "sesiones.iniciada": "Started: {fecha}",
  "sesiones.vence": "Expires: {fecha}",
  "sesiones.cerrarEsta": "Close this session",
  "sesiones.revocar": "Revoke",
  "sesiones.error": "Could not load the sessions",
  "sesiones.errorRevocar": "Could not revoke the session",
  "sesiones.errorRevocarTodas": "Could not revoke the sessions",

  "profesor.titulo": "Course authoring",
  "profesor.subtitulo": "Create drafts and publish versions. A published version is immutable.",
  "profesor.nuevoCurso": "New course",
  "profesor.nuevoCursoAyuda": "The course starts out as an editable draft.",
  "profesor.slug": "Slug (identifier in the URL)",
  "profesor.slugPlaceholder": "introduction-to-cloud",
  "profesor.creando": "Creating…",
  "profesor.crearBorrador": "Create draft",
  "profesor.sinCursos": "You have not created any course yet.",
  "profesor.publicado": "Published",
  "profesor.sinPublicar": "Not published",
  "profesor.verPublicada": "View the published version",
  "profesor.publicadoAyuda":
    "A published course cannot be edited. Unpublish it to open a draft: it will leave the catalog until you publish again, and students already enrolled keep their progress.",
  "profesor.editarBorrador": "Edit draft",
  "profesor.crearVersion": "Create new version",
  "profesor.despublicarParaEditar": "Unpublish to edit",
  "profesor.despublicando": "Unpublishing…",
  "profesor.confirmarDespublicar":
    "Unpublishing “{slug}” will remove it from the catalog until you publish the new version. Continue?",
  "profesor.error": "Could not load your courses",
  "profesor.errorCrear": "Could not create the course",
  "profesor.errorBorrador": "Could not open the draft",
  "profesor.errorDespublicar": "Could not unpublish the course",

  "version.error": "Could not load the version",
  "version.publicada": "Version published.",
  "version.errorPublicar": "Could not publish",
  "version.sinTitulo": "(untitled)",
  "version.previsualizar": "Preview",
  "version.etiqueta": "Version {n} · {estado}",
  "version.inmutable":
    "This version is published and immutable. To edit it, the course has to be unpublished first.",
  "version.irADespublicar": "Go to my courses to unpublish it",
  "version.motivos": "It could not be published. Reasons:",
  "version.publicar": "Publish version",
  "version.estructura": "Structure",
  "version.metadatos": "Metadata",
  "version.resumen": "Summary",
  "version.descripcion": "Description (Markdown)",
  "version.categoria": "Category",
  "version.nivel": "Level",
  "version.idioma": "Language",
  "version.notaMinima": "Minimum passing score (%)",
  "version.pctObligatorios": "% of required resources needed",
  "version.guardarMetadatos": "Save metadata",
  "version.errorGuardar": "Could not save",
  "version.nuevoModulo": "New module",
  "version.agregarModulo": "Add module",
  "version.errorModulo": "Could not add the module",
  "version.eliminarModulo": "Delete module",
  "version.confirmarModulo": "Delete the module “{titulo}” and all its content?",
  "version.nuevaUnidad": "New unit",
  "version.agregarUnidad": "Add unit",
  "version.eliminarUnidad": "Delete unit",
  "version.confirmarUnidad": "Delete the unit “{titulo}” and its resources?",
  "version.confirmarRecurso": "Delete the resource “{titulo}”?",
  "version.procesamiento": "processing: {estado}",
  "version.oculto": "hidden",
  "version.definirEvaluacion": "Define assessment",
  "version.subirArchivo": "Upload file",
  "version.reanudarSubida": "Resume upload",
  "version.subiendoAria": "Uploading {titulo}",
  "version.cargaPendiente":
    "There is an unfinished upload. Pick the same file again and it will continue from where it stopped.",
  "version.empezarDeNuevo": "Start over",
  "version.editarContenido": "Edit content",
  "version.guardarRevision": "Save revision",
  "version.guardado": "Saved",
  "version.tipoRecurso": "Resource type",
  "version.tituloRecurso": "Resource title",
  "version.contenidoBloques": "Resource content (blocks)",
  "version.url": "URL",
  "version.visible": "Visible",
  "version.obligatorio": "Required",
  "version.agregarRecurso": "Add resource",
  "version.errorRecurso": "Could not add the resource",
  "version.sinDestinos":
    "There are no domains allowed for embedding. Ask an administrator to add the one you need.",
  "version.destinos": "Allowed domains: {hosts}.",

  "tipo.text": "Rich text",
  "tipo.image": "Image",
  "tipo.video": "Video",
  "tipo.audio": "Audio",
  "tipo.pdf": "PDF",
  "tipo.presentation": "Presentation",
  "tipo.file": "Downloadable file",
  "tipo.iframe": "Allowed iframe",
  "tipo.link": "External link",
  "tipo.quiz": "Quiz",

  "cambios.identico": "This draft is identical to the published version.",
  "cambios.titulo": "Changes from what is published",
  "cambios.mayor":
    "This update changes what has to be completed in order to pass. When it is published, enrolled students' progress is recalculated against the new list of required resources. What they already did is kept, and nobody loses a pass they already earned.",
  "cambios.menor":
    "It changes the content or the presentation, but not what has to be completed: enrolled students' progress is still measured the same way.",
  "cambios.afectaProgreso": "affects progress",

  "historial.titulo": "Revision history",
  "historial.error": "Could not load the history",
  "historial.errorAbrir": "Could not open the revision",
  "historial.errorRestaurar": "Could not restore",
  "historial.confirmar": "Restore revision {n}? It will be saved as a new revision.",
  "historial.vacio": "There are no saved revisions of this resource yet.",
  "historial.guardadaPor": "Saved by {autor}",
  "historial.restaurar": "Restore",
  "historial.revision": "Revision #{n}",

  "previsualizacion.titulo": "Preview",
  "previsualizacion.error": "Could not load the preview",
  "previsualizacion.cargando": "Loading preview…",
  "previsualizacion.volver": "Back to editing",
  "previsualizacion.intro": "This is how someone enrolled will see the course. Version {n} ·",
  "previsualizacion.enProceso":
    "{n} resource(s) still being processed. It cannot be published until they finish.",
  "previsualizacion.ocultos": "{n} hidden resource(s) are not shown here.",
  "previsualizacion.sinTitulo": "Untitled",
  "previsualizacion.aprobacion":
    "Passing: {nota}% minimum score and {pct}% of the required resources.",
  "previsualizacion.sinModulos": "The draft does not have modules yet.",
  "previsualizacion.sinVisibles": "No visible resources.",

  "quizAutoria.noEncontrado": "The resource was not found in this version.",
  "quizAutoria.errorVersion": "Could not load the version",
  "quizAutoria.guardada": "Assessment saved.",
  "quizAutoria.errorGuardar": "Could not save the assessment",
  "quizAutoria.titulo": "Assessment: {recurso}",
  "quizAutoria.volver": "Back to the version",
  "quizAutoria.aviso":
    "Saving replaces this resource's previous assessment entirely. Attempts that were already graded are unaffected: they keep their own snapshot of questions and options.",
  "quizAutoria.notaMinima": "Minimum score to pass (%)",
  "quizAutoria.limiteTiempo": "Time limit (minutes, optional)",
  "quizAutoria.intentosMaximos": "Maximum attempts (optional)",
  "quizAutoria.retroalimentacion": "Feedback",
  "quizAutoria.inmediata": "Immediate",
  "quizAutoria.alEnviar": "When the attempt is submitted",
  "quizAutoria.alCerrar": "When the quiz closes",
  "quizAutoria.nunca": "Never",
  "quizAutoria.mezclar": "Shuffle the order of the questions",
  "quizAutoria.preguntas": "Questions",
  "quizAutoria.pregunta": "Question {n}",
  "quizAutoria.eliminarPregunta": "Delete question",
  "quizAutoria.enunciado": "Prompt",
  "quizAutoria.tipo": "Type",
  "quizAutoria.unica": "Single answer",
  "quizAutoria.multiple": "Multiple answers",
  "quizAutoria.puntos": "Points",
  "quizAutoria.opcionesUna": "Options (mark the correct one):",
  "quizAutoria.opcionesVarias": "Options (mark the correct ones):",
  "quizAutoria.opcion": "Option {n}",
  "quizAutoria.marcarCorrecta": "Mark option {n} of question {pregunta} as correct",
  "quizAutoria.textoDeOpcion": "Text of option {n} of question {pregunta}",
  "quizAutoria.agregarOpcion": "Add option",
  "quizAutoria.agregarPregunta": "Add question",
  "quizAutoria.guardar": "Save assessment",

  "resultados.titulo": "Results",
  "resultados.sinIntentos": "Nobody has taken it yet.",
  "resultados.unIntento": "1 attempt",
  "resultados.nIntentos": "{n} attempts",
  "resultados.unEstudiante": "1 student",
  "resultados.nEstudiantes": "{n} students",
  "resultados.de": "from",
  "resultados.notaMedia": "Mean score: {n}",
  "resultados.mediana": "Median: {n}",
  "resultados.aprobados": "Pass rate: {n}%",
  "resultados.acierto": "Correct: {n}%",
  "resultados.enBlanco": "Blank: {n}",
  "resultados.malPlanteada":
    "Fewer than a third get it right: it may be badly worded, not just hard.",
  "resultados.correcta": "correct",

  "admin.subtitulo": "Accounts, roles, sessions and the platform's immutable audit log.",
  "admin.errorUsuarios": "Could not load the list of users",
  "admin.errorRol": "Could not update the role",
  "admin.errorEstado": "Could not update the status",
  "admin.profesorCreado": "Teacher created. An email was sent for them to set their password.",
  "admin.usuarios": "Users",
  "admin.usuariosAyuda": "Change an account's role or status, and review its open sessions.",
  "admin.buscarUsuario": "Search by name or email",
  "admin.colNombre": "Name",
  "admin.colCorreo": "Email",
  "admin.colRol": "Role",
  "admin.colEstado": "Status",
  "admin.colSesiones": "Sessions",
  "admin.rolDe": "Role of {nombre}",
  "admin.estadoDe": "Status of {nombre}",
  "admin.errorMetricas": "Could not load the metrics",
  "admin.metricas": "Platform metrics",
  "admin.metricaUsuarios": "Users",
  "admin.metricaCursos": "Course versions",
  "admin.metricaInscripciones": "Enrollments",
  "admin.metricaInsignias": "Badges",
  "admin.metricaMultimedia": "Media",
  "admin.metricaEvaluaciones": "Assessment attempts",
  "admin.sinRegistros": "No records yet.",
  "admin.errorDestinos": "Could not load the list of destinations",
  "admin.errorAutorizar": "Could not allow the destination",
  "admin.errorQuitarDestino": "Could not remove the destination",
  "admin.confirmarQuitarDestino":
    "Remove {host} from the list? Published resources that use it will stop being shown.",
  "admin.destinos": "Embeddable destinations",
  "admin.destinosAyuda":
    "Only these domains can appear in an iframe resource. Everything else is rejected both when saving and when serving.",
  "admin.dominio": "Domain",
  "admin.permisos": "Granted permissions",
  "admin.paraQue": "What for",
  "admin.paraQuePlaceholder": "Video player",
  "admin.incluirSubdominios": "Include subdomains",
  "admin.autorizar": "Allow",
  "admin.avisoSubdominios":
    "Including subdomains also allows the ones that third party creates in the future, including any that hosts its users' content.",
  "admin.sinDestinos": "There is no allowed destination.",
  "admin.sinDestinosAyuda":
    "While the list is empty, no embedded resource can be saved.",
  "admin.ySubdominios": "and subdomains",
  "admin.sinPermisosExtra": "no extra permissions",
  "admin.sandboxAntes": "Every frame is served with",
  "admin.sandboxDespues":
    ", which denies it navigating the top window, starting downloads and showing modal dialogs.",
  "admin.sesionesActivas": "{n} active",
  "admin.cerrarTodas": "Close all",
  "admin.errorAuditoria": "Could not load the audit log",
  "admin.auditoria": "Audit log",
  "admin.auditoriaAyuda": "Immutable record: the database refuses to modify or delete it.",
  "admin.filtrarAccion": "Filter by action",
  "admin.filtrar": "Filter",
  "admin.sinEntradas": "No entries for that filter.",
  "admin.colCuando": "When",
  "admin.colAccion": "Action",
  "admin.colActor": "Actor",
  "admin.colEntidad": "Entity",
  "admin.colIP": "IP",
  "admin.invitarProfesor": "Invite a teacher",
  "admin.invitarAyuda":
    "Teachers do not register themselves: they get a link to set their password.",
  "admin.errorProfesor": "Could not create the teacher",
  "admin.crearProfesor": "Create teacher",

  "editor.titulo": "Block editor (canonical Markdown)",
  "editor.borradorGuardado": "Draft saved",
  "editor.guardando": "Saving…",
  "editor.autoguardado": "Autosaved at {hora}",
  "editor.errorAutoguardar": "Autosave failed",
  "editor.moverArriba": "Move up",
  "editor.moverAbajo": "Move down",
  "editor.eliminarBloque": "Delete block",
  "editor.encabezadoPlaceholder": "Heading text…",
  "editor.codigoPlaceholder": "// Code here…",
  "editor.lenguajePlaceholder": "language (e.g. python, go)",
  "editor.calloutPlaceholder": "Important note or warning…",
  "editor.formulaAntes": "LaTeX notation. It is stored between",
  "editor.formulaDespues": ", which is what formula renderers recognize.",
  "editor.listaPlaceholder": "One item per line…",
  "editor.parrafoPlaceholder": "Write the paragraph's content…",
  "editor.parrafo": "+ Paragraph",
  "editor.encabezado": "+ Heading",
  "editor.codigo": "+ Code",
  "editor.nota": "+ Note / Quote",
  "editor.lista": "+ List",
  "editor.tareas": "+ Tasks",
  "editor.tabla": "+ Table",
  "editor.formula": "+ Formula",
  "editor.limpiar": "Clear draft",
  "editor.columnaN": "Column {n}",
  "editor.primeraTarea": "First task",
  "editor.encabezadoColumna": "Header of column {n}",
  "editor.celda": "Row {fila}, column {columna}",
  "editor.masFila": "+ Row",
  "editor.masColumna": "+ Column",
  "editor.menosFila": "− Row",
  "editor.primeraFilaEncabezado": "The first row is the header.",
  "editor.marcarHecha": "Mark “{tarea}” as done",
  "editor.tareaN": "task {n}",
  "editor.quitarTarea": "Remove task {n}",
  "editor.descripcionTarea": "Task description",
  "editor.masTarea": "+ Task",

  "carga.fase.huella": "Computing the file's fingerprint…",
  "carga.fase.preparando": "Preparing the upload…",
  "carga.fase.subiendo": "Uploading…",
  "carga.fase.verificando": "Checking integrity and safety…",
  "carga.fase.encolado": "Queued for processing",
  "carga.fase.listo": "File ready",
  "carga.fase.retomando": "Resuming the interrupted upload…",
  "carga.fase.iniciando": "Starting the upload…",
  "carga.fase.parteSubida": "Part {numero} of {total} was already uploaded",
  "carga.fase.parteEnCurso": "Uploading part {numero} of {total}…",
  "carga.error.subida": "The upload failed (HTTP {status}).",
  "carga.error.parte":
    "Part {numero} failed (HTTP {status}). Try again: it will resume here.",
  "carga.error.etag":
    "The storage did not expose the part's ETag. Add ETag to Access-Control-Expose-Headers in MinIO or the CDN.",
  "carga.error.generico": "Could not upload the file",
};

const CATALOGOS: Record<Idioma, Record<Clave, string>> = { es, en };

const CLAVE_ALMACEN = "mooc.idioma";

/** Traducir es la firma de t, para tiparla donde se pasa como argumento. */
export type Traducir = (clave: Clave, valores?: Record<string, string | number>) => string;

interface Contexto {
  idioma: Idioma;
  /** locale es la etiqueta con la que formatear fechas, horas y números. */
  locale: string;
  cambiar: (idioma: Idioma) => void;
  /** t traduce una clave, sustituyendo los marcadores {nombre}. */
  t: Traducir;
}

const I18nContext = createContext<Contexto | null>(null);

/**
 * idiomaInicial elige el idioma de arranque.
 *
 * Manda lo que la persona eligió; si no eligió nada, el del navegador; y si
 * tampoco, español, que es el idioma de origen del contenido.
 */
function idiomaInicial(): Idioma {
  if (typeof window === "undefined") return "es";
  try {
    const guardado = localStorage.getItem(CLAVE_ALMACEN);
    if (guardado && (IDIOMAS as readonly string[]).includes(guardado)) {
      return guardado as Idioma;
    }
  } catch {
    /* sin localStorage se usa el del navegador */
  }
  const navegador = navigator.language?.slice(0, 2);
  return (IDIOMAS as readonly string[]).includes(navegador) ? (navegador as Idioma) : "es";
}

export function I18nProvider({ children }: { children: React.ReactNode }) {
  // Se arranca siempre en español y se ajusta tras montar: leer localStorage o
  // navigator durante el render del servidor produce una hidratación distinta
  // de la que sirvió el servidor, y React lo descarta con un aviso.
  const [idioma, setIdioma] = useState<Idioma>("es");

  useEffect(() => {
    setIdioma(idiomaInicial());
  }, []);

  useEffect(() => {
    document.documentElement.lang = idioma;
  }, [idioma]);

  const cambiar = useCallback((nuevo: Idioma) => {
    setIdioma(nuevo);
    try {
      localStorage.setItem(CLAVE_ALMACEN, nuevo);
    } catch {
      /* la elección no sobrevivirá a la recarga, pero la sesión sí */
    }
  }, []);

  const t = useCallback<Traducir>(
    (clave, valores) => {
      const texto = CATALOGOS[idioma][clave] ?? CATALOGOS.es[clave] ?? clave;
      if (!valores) return texto;
      return Object.entries(valores).reduce(
        (acc, [nombre, valor]) => acc.replaceAll(`{${nombre}}`, String(valor)),
        texto,
      );
    },
    [idioma],
  );

  const valor = useMemo(
    () => ({ idioma, locale: LOCALES[idioma], cambiar, t }),
    [idioma, cambiar, t],
  );
  return <I18nContext.Provider value={valor}>{children}</I18nContext.Provider>;
}

export function useI18n(): Contexto {
  const ctx = useContext(I18nContext);
  if (!ctx) {
    throw new Error("useI18n debe usarse dentro de I18nProvider");
  }
  return ctx;
}

/**
 * useTraductorEstable devuelve un traductor cuya identidad no cambia al
 * cambiar de idioma.
 *
 * Es para traducir dentro de un efecto que no debe volver a ejecutarse solo
 * porque alguien eligió otro idioma. `t` cambia con el idioma, así que
 * declararlo como dependencia relanza el efecto: eso recargaría el catálogo
 * perdiendo los filtros aplicados, o volvería a gastar el token de un enlace
 * de verificación, que es de un solo uso. Nadie pidió eso al pulsar
 * "English".
 *
 * A cambio, un mensaje ya escrito en pantalla se queda en el idioma en que se
 * generó. Es un texto concreto sobre algo que ya pasó, no una etiqueta de la
 * interfaz, y desaparece en cuanto se reintenta la acción.
 */
export function useTraductorEstable(): Traducir {
  const { t } = useI18n();
  const actual = useRef(t);
  // La ref se actualiza en un efecto, no durante el render: con el render
  // concurrente React puede descartar un render a medias, y una escritura
  // hecha ahí se habría aplicado igual.
  useEffect(() => {
    actual.current = t;
  }, [t]);
  return useCallback<Traducir>((clave, valores) => actual.current(clave, valores), []);
}

"use client";

import { useI18n } from "@/lib/i18n";

/**
 * Enlace de salto al contenido principal.
 *
 * Va en su propio componente de cliente porque el layout es de servidor y el
 * catálogo vive en un contexto de React: es la primera parada de quien navega
 * con teclado o lector de pantalla, y debe estar en su idioma.
 */
export function SaltarAlContenido() {
  const { t } = useI18n();
  return (
    <a href="#main-content" className="skip-link">
      {t("nav.saltar")}
    </a>
  );
}

/** Salto al contenido principal: requisito de navegacion por teclado (WCAG 2.2 AA). */
export function SkipLink() {
  return (
    <a
      href="#contenido"
      className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-50 focus:rounded-md focus:bg-acento focus:px-4 focus:py-2 focus:text-acento-contraste"
    >
      Saltar al contenido principal
    </a>
  );
}

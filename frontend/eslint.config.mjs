import nextConfig from "eslint-config-next/core-web-vitals";

// Configuración de ESLint en formato plano.
//
// El formato antiguo (.eslintrc.json) dejó de estar soportado en ESLint 9, y
// `next lint` desapareció en Next 16: ahora se invoca eslint directamente.
// El conjunto de reglas sigue siendo core-web-vitals de Next, así que el
// cambio es de envoltorio, no de criterio.

const config = [
  {
    // Lo generado y lo descargado no se revisa: ni es nuestro ni se corrige
    // editándolo.
    ignores: [
      ".next/**",
      "node_modules/**",
      "out/**",
      "playwright-report/**",
      "test-results/**",
    ],
  },

  ...nextConfig,

  {
    rules: {
      // Aviso y no error, con motivo.
      //
      // La regla viene del React Compiler y avisa de que llamar a setState en
      // el cuerpo de un efecto encadena renders. Tiene razón cuando la
      // llamada es síncrona, y aquí hay un caso así: el provider de idioma
      // ajusta el idioma tras montar, a propósito, porque leer localStorage
      // durante el render del servidor produciría una hidratación distinta de
      // la que se sirvió.
      //
      // Los otros quince avisos son el patrón de carga de datos al montar:
      // `useEffect(() => { void cargar(); }, [cargar])`, donde `cargar` es
      // asíncrona y el setState ocurre después de un await, no en el cuerpo
      // del efecto. El análisis estático no atraviesa esa frontera, así que
      // los marca igual. No son renders encadenados.
      //
      // Se queda en aviso y no se desactiva: así sigue visible, y el día que
      // aparezca un setState de verdad síncrono se verá entre los demás.
      // Quitarlos de raíz exige adoptar una biblioteca de datos (SWR, React
      // Query) o mover la carga a componentes de servidor, que es una
      // decisión de arquitectura y no un arreglo de lint.
      "react-hooks/set-state-in-effect": "warn",
    },
  },
];

export default config;

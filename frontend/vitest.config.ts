import { defineConfig } from "vitest/config";
import { resolve } from "node:path";

export default defineConfig({
  resolve: {
    // Mismo alias que tsconfig, para que las pruebas importen igual que el
    // código de la aplicación.
    alias: { "@": resolve(__dirname, ".") },
  },
  test: {
    environment: "node",
    // Solo *.test.ts: las pruebas de extremo a extremo son *.spec.ts y las
    // ejecuta Playwright, que necesita la pila levantada.
    include: ["**/*.test.ts"],
    exclude: ["node_modules/**", ".next/**", "e2e/**"],
  },
});

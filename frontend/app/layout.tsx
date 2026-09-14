import type { Metadata } from "next";
import "./globals.css";
import { AuthProvider } from "@/lib/auth-context";
import { I18nProvider } from "@/lib/i18n";
import { NavBar } from "@/components/NavBar";

export const metadata: Metadata = {
  title: "Plataforma MOOC",
  description: "Plataforma web de cursos masivos abiertos en línea",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  // El atributo lang lo actualiza I18nProvider cuando la persona cambia de
  // idioma: un lector de pantalla necesita saber en qué idioma está el texto
  // que va a pronunciar. Aquí queda el idioma de origen, que es con el que se
  // sirve la primera carga.
  return (
    <html lang="es">
      <body>
        <a href="#main-content" className="skip-link">
          Saltar al contenido principal
        </a>
        <I18nProvider>
          <AuthProvider>
            <NavBar />
            <main id="main-content" className="container">
              {children}
            </main>
          </AuthProvider>
        </I18nProvider>
      </body>
    </html>
  );
}

import type { Metadata } from "next";
import "./globals.css";
import { AuthProvider } from "@/lib/auth-context";
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
  return (
    <html lang="es">
      <body>
        <a href="#main-content" className="skip-link">
          Saltar al contenido principal
        </a>
        <AuthProvider>
          <NavBar />
          <main id="main-content" className="container">
            {children}
          </main>
        </AuthProvider>
      </body>
    </html>
  );
}

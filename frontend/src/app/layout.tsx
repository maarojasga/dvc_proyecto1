import type { Metadata } from "next";
import type { ReactNode } from "react";
import { Geist, Geist_Mono } from "next/font/google";
import { SkipLink } from "@/components/layout/skip-link";
import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";
import "./globals.css";

const geistSans = Geist({ variable: "--font-geist-sans", subsets: ["latin"] });
const geistMono = Geist_Mono({ variable: "--font-geist-mono", subsets: ["latin"] });

export const metadata: Metadata = {
  title: {
    default: "Plataforma MOOC",
    template: "%s · Plataforma MOOC",
  },
  description:
    "Plataforma web de cursos masivos abiertos en linea: autoria versionada, multimedia, evaluacion e insignias verificables.",
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html
      lang="es"
      className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}
    >
      <body className="flex min-h-full flex-col font-sans">
        <SkipLink />
        <SiteHeader />
        <main id="contenido" className="flex-1 py-10">
          {children}
        </main>
        <SiteFooter />
      </body>
    </html>
  );
}

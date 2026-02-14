import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Prelude - Cluster Access",
  description: "Get access to a Red Hat AI cluster with your phone number",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body className="bg-rh-gray-10 min-h-screen font-rh-text text-rh-gray-95">
        {children}
      </body>
    </html>
  );
}

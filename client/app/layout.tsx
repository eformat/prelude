import type { Metadata } from "next";
import Script from "next/script";
import "./globals.css";
import ReCaptchaProvider from "./recaptcha-provider";

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
      <head>
        <Script
          src="https://www.googletagmanager.com/gtag/js?id=G-8Z588DR46R"
          strategy="afterInteractive"
        />
        <Script id="google-analytics" strategy="afterInteractive">
          {`
            window.dataLayer = window.dataLayer || [];
            function gtag(){dataLayer.push(arguments);}
            gtag('js', new Date());
            gtag('config', 'G-8Z588DR46R');
          `}
        </Script>
      </head>
      <body className="bg-rh-gray-10 min-h-screen font-rh-text text-rh-gray-95">
        <ReCaptchaProvider>{children}</ReCaptchaProvider>
      </body>
    </html>
  );
}

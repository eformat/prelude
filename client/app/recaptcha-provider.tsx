"use client";

import { useEffect, useState } from "react";
import { GoogleReCaptchaProvider } from "react-google-recaptcha-v3";

export default function ReCaptchaProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  const [siteKey, setSiteKey] = useState("");

  useEffect(() => {
    fetch("/api/config")
      .then((res) => res.json())
      .then((data) => {
        if (data.recaptchaSiteKey) {
          setSiteKey(data.recaptchaSiteKey);
        }
      })
      .catch(() => {});
  }, []);

  if (!siteKey) {
    return <>{children}</>;
  }

  return (
    <GoogleReCaptchaProvider reCaptchaKey={siteKey}>
      {children}
    </GoogleReCaptchaProvider>
  );
}

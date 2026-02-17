"use client";

import { useState, useEffect } from "react";
import { useGoogleReCaptcha } from "react-google-recaptcha-v3";
import { claimCluster } from "./actions";
import { getFingerprint } from "./fingerprint";

interface ClusterInfo {
  webConsoleURL: string;
  aiConsoleURL: string;
  kubeconfig: string;
  expiresAt: string;
}

function CopyIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
      <rect x="5" y="5" width="9" height="9" rx="1" stroke="currentColor" strokeWidth="1.5" />
      <path d="M11 3H3C2.44772 3 2 3.44772 2 4V12" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
    </svg>
  );
}

function CheckIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M3 8.5L6.5 12L13 4" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function ArrowIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 14 14" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M1 7H13M13 7L7.5 1.5M13 7L7.5 12.5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function ExternalIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 14 14" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M11 7.5V11.5C11 12.0523 10.5523 12.5 10 12.5H2.5C1.94772 12.5 1.5 12.0523 1.5 11.5V4C1.5 3.44772 1.94772 3 2.5 3H6.5" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M9 1.5H12.5V5" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M12.5 1.5L6.5 7.5" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
    </svg>
  );
}

function DownloadIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M8 2V10.5M8 10.5L4.5 7M8 10.5L11.5 7" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M2.5 13H13.5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
    </svg>
  );
}

function EyeIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 18 18" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M1.5 9C1.5 9 4 3.75 9 3.75C14 3.75 16.5 9 16.5 9C16.5 9 14 14.25 9 14.25C4 14.25 1.5 9 1.5 9Z" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
      <circle cx="9" cy="9" r="2.25" stroke="currentColor" strokeWidth="1.4" />
    </svg>
  );
}

function EyeOffIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 18 18" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M7.58 7.58A2.013 2.013 0 0 0 7 9a2 2 0 0 0 2.42 1.955M10.73 10.18A2 2 0 0 1 7.58 7.58" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M5.1 5.1C3.36 6.26 2.07 8 1.5 9c.96 1.68 3.54 5.25 7.5 5.25 1.55 0 2.9-.54 4.02-1.28M14.85 12.15C16.24 10.98 16.5 9 16.5 9c-.96-1.68-3.54-5.25-7.5-5.25-.63 0-1.23.09-1.8.23" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M2.25 2.25L15.75 15.75" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
    </svg>
  );
}

function ClockIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
      <circle cx="8" cy="8" r="6.5" stroke="currentColor" strokeWidth="1.5" />
      <path d="M8 4.5V8L10.5 10.5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function CountdownTimer({ expiresAt }: { expiresAt: string }) {
  const [remaining, setRemaining] = useState("");
  const [expired, setExpired] = useState(false);

  useEffect(() => {
    function update() {
      const now = Date.now();
      const end = new Date(expiresAt).getTime();
      const diff = end - now;

      if (diff <= 0) {
        setRemaining("0h 0m 0s");
        setExpired(true);
        return;
      }

      setExpired(false);
      const days = Math.floor(diff / (1000 * 60 * 60 * 24));
      const hours = Math.floor((diff % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60));
      const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));
      const seconds = Math.floor((diff % (1000 * 60)) / 1000);

      const parts = [];
      if (days > 0) parts.push(`${days}d`);
      parts.push(`${hours}h`);
      parts.push(`${minutes}m`);
      parts.push(`${seconds}s`);
      setRemaining(parts.join(" "));
    }

    update();
    const interval = setInterval(update, 1000);
    return () => clearInterval(interval);
  }, [expiresAt]);

  const expiryDate = new Date(expiresAt);
  const formatted = expiryDate.toLocaleString(undefined, {
    weekday: "short",
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    timeZoneName: "short",
  });

  return (
    <div className="px-6 py-5">
      <div className="flex items-center gap-3 mb-3">
        <span className="font-rh-text text-rh-gray-60 text-sm">Expires</span>
        <span className="font-rh-text text-rh-gray-95 text-sm font-medium">
          {formatted}
        </span>
      </div>
      <div className={`font-mono text-2xl font-bold tracking-wider ${expired ? "text-rh-red-50" : "text-rh-gray-95"}`}>
        {remaining}
      </div>
      {expired && (
        <p className="mt-2 font-rh-text text-sm text-rh-red-50">
          This cluster has expired and will be reclaimed.
        </p>
      )}
    </div>
  );
}

export default function Home() {
  const [phone, setPhone] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [cluster, setCluster] = useState<ClusterInfo | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [copied, setCopied] = useState<string | null>(null);
  const { executeRecaptcha } = useGoogleReCaptcha();

  function validatePhone(value: string): string | null {
    const digits = value.replace(/\D/g, "");
    if (digits.length < 7) {
      return "Phone number must have at least 7 digits";
    }
    if (digits.length > 15) {
      return "Phone number must have no more than 15 digits";
    }
    return null;
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setCluster(null);

    const phoneError = validatePhone(phone);
    if (phoneError) {
      setError(phoneError);
      return;
    }

    setLoading(true);

    try {
      let fingerprint = "";
      try {
        fingerprint = await getFingerprint();
      } catch {
        // Fingerprint not available, continue without it
      }

      let recaptchaToken = "";
      try {
        if (executeRecaptcha) {
          recaptchaToken = await executeRecaptcha("claim");
        }
      } catch {
        // reCAPTCHA not available, continue without token
      }

      const result = await claimCluster(phone, password, recaptchaToken, fingerprint);

      if (!result.success) {
        setError(result.error);
        return;
      }

      setCluster(result.data);
    } catch {
      setError("Network error. Please try again.");
    } finally {
      setLoading(false);
    }
  }

  function downloadKubeconfig(content: string) {
    const blob = new Blob([content], { type: "text/yaml" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "kubeconfig";
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  }

  async function copyToClipboard(text: string, label: string) {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(label);
      setTimeout(() => setCopied(null), 2000);
    } catch {
      const textarea = document.createElement("textarea");
      textarea.value = text;
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand("copy");
      document.body.removeChild(textarea);
      setCopied(label);
      setTimeout(() => setCopied(null), 2000);
    }
  }

  return (
    <div className="min-h-screen">
      {/* ── Navigation Bar ── */}
      <nav className="bg-rh-gray-95 border-b border-rh-gray-80">
        <div className="max-w-7xl mx-auto px-6 lg:px-8">
          <div className="flex items-center justify-between h-16">
            <div className="flex items-center gap-3">
              {/* Red Hat-style hat accent */}
              <div className="w-9 h-9 relative flex items-center justify-center">
                <div className="absolute w-full h-1 bg-rh-red-50 top-1.5 rounded-sm" />
                <div className="w-6 h-5 bg-rh-red-50 rounded-sm mt-1" />
              </div>
              <span className="font-rh-display text-white text-xl font-bold tracking-tight">
                Prelude
              </span>
            </div>
            <div className="hidden sm:flex items-center gap-8">
              <span className="font-rh-text text-rh-gray-40 text-sm">
                Red Hat AI Cluster Access
              </span>
            </div>
          </div>
        </div>
      </nav>

      {/* ── Hero Section ── */}
      <section className="relative bg-rh-gray-95 overflow-hidden">
        <div className="rh-hero-accent" />

        <div className="relative max-w-7xl mx-auto px-6 lg:px-8 py-20 lg:py-28">
          <div className="max-w-3xl">
            {/* Red accent bar */}
            <div className="w-16 h-1 bg-rh-red-50 mb-8 animate-expand" />

            <h1 className="font-rh-display text-white text-4xl sm:text-5xl lg:text-6xl font-bold leading-tight tracking-tight mb-6 animate-fade-in-up">
              Get instant access to your Red Hat AI cluster
            </h1>

            <p className="font-rh-text text-rh-gray-40 text-lg sm:text-xl leading-relaxed mb-12 max-w-2xl animate-fade-in-up"
               style={{ animationDelay: '0.1s' }}>
              Enter your phone number and choose an admin password to claim
              a dedicated cluster environment. You&apos;ll receive your login details immediately.
            </p>

            {/* Phone Input Form */}
            <form
              onSubmit={handleSubmit}
              className="animate-fade-in-up"
              style={{ animationDelay: '0.2s' }}
            >
              <div className="flex flex-col gap-4 max-w-xl">
                <div className="flex flex-col sm:flex-row gap-4">
                  <div className="flex-1 relative">
                    <label htmlFor="phone" className="sr-only">Phone number</label>
                    <input
                      id="phone"
                      type="tel"
                      value={phone}
                      onChange={(e) => setPhone(e.target.value)}
                      placeholder="Enter phone number"
                      className="w-full px-5 py-4 bg-rh-gray-90 border border-rh-gray-70 text-white font-rh-text text-base placeholder-rh-gray-50 focus:outline-none focus:border-rh-red-50 focus:ring-1 focus:ring-rh-red-50 transition-colors"
                      required
                    />
                  </div>
                  <div className="flex-1 relative">
                    <label htmlFor="password" className="sr-only">Admin password</label>
                    <input
                      id="password"
                      type={showPassword ? "text" : "password"}
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      placeholder="Admin password"
                      className="w-full px-5 py-4 pr-12 bg-rh-gray-90 border border-rh-gray-70 text-white font-rh-text text-base placeholder-rh-gray-50 focus:outline-none focus:border-rh-red-50 focus:ring-1 focus:ring-rh-red-50 transition-colors"
                      required
                    />
                    <button
                      type="button"
                      onClick={() => setShowPassword(!showPassword)}
                      className="absolute right-3 top-1/2 -translate-y-1/2 text-rh-gray-50 hover:text-white transition-colors"
                      aria-label={showPassword ? "Hide password" : "Show password"}
                    >
                      {showPassword ? <EyeOffIcon /> : <EyeIcon />}
                    </button>
                  </div>
                </div>
                <button
                  type="submit"
                  disabled={loading}
                  className="group flex items-center justify-center gap-3 px-8 py-4 bg-rh-red-50 text-white font-rh-text font-bold text-base hover:bg-rh-red-60 disabled:bg-rh-gray-70 disabled:text-rh-gray-50 transition-all duration-200"
                >
                  {loading ? (
                    <>
                      <svg className="animate-spin h-5 w-5" viewBox="0 0 24 24">
                        <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="3" fill="none" />
                        <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                      </svg>
                      <span>Claiming cluster</span>
                    </>
                  ) : (
                    <>
                      <span>Get cluster</span>
                      <ArrowIcon />
                    </>
                  )}
                </button>
              </div>
            </form>

            {/* Error Message */}
            {error && (
              <div className="mt-6 max-w-xl animate-fade-in-up">
                {error === "all_clusters_in_use" ? (
                  <div className="px-6 py-5 bg-rh-gray-80 border border-rh-gray-70 text-center">
                    <p className="font-rh-text text-white text-lg leading-relaxed">
                      Sorry, all of our clusters are in use at the moment, try again later.
                    </p>
                  </div>
                ) : error === "device_already_claimed" ? (
                  <div className="flex items-start gap-3 px-5 py-4 bg-rh-red-80 border border-rh-red-70">
                    <svg width="20" height="20" viewBox="0 0 20 20" fill="none" className="mt-0.5 flex-shrink-0">
                      <circle cx="10" cy="10" r="9" stroke="#F56E6E" strokeWidth="1.5" />
                      <path d="M10 6V11" stroke="#F56E6E" strokeWidth="1.5" strokeLinecap="round" />
                      <circle cx="10" cy="14" r="0.75" fill="#F56E6E" />
                    </svg>
                    <p className="font-rh-text text-rh-red-30 text-sm leading-relaxed">
                      This device has already been used to claim a cluster. Please use your original phone number.
                    </p>
                  </div>
                ) : (
                  <div className="flex items-start gap-3 px-5 py-4 bg-rh-red-80 border border-rh-red-70">
                    <svg width="20" height="20" viewBox="0 0 20 20" fill="none" className="mt-0.5 flex-shrink-0">
                      <circle cx="10" cy="10" r="9" stroke="#F56E6E" strokeWidth="1.5" />
                      <path d="M10 6V11" stroke="#F56E6E" strokeWidth="1.5" strokeLinecap="round" />
                      <circle cx="10" cy="14" r="0.75" fill="#F56E6E" />
                    </svg>
                    <p className="font-rh-text text-rh-red-30 text-sm leading-relaxed">{error}</p>
                  </div>
                )}
              </div>
            )}
          </div>
        </div>

        {/* Bottom edge transition */}
        <div className="h-px bg-gradient-to-r from-rh-red-50 via-rh-red-50/20 to-transparent" />
      </section>

      {/* ── Cluster Info Section ── */}
      {cluster && (
        <section className="bg-rh-gray-10">
          <div className="max-w-7xl mx-auto px-6 lg:px-8 py-16 lg:py-20">
            {/* Section Header */}
            <div className="mb-10 animate-fade-in-up">
              <div className="w-10 h-0.5 bg-rh-red-50 mb-5" />
              <h2 className="font-rh-display text-rh-gray-95 text-2xl sm:text-3xl font-bold tracking-tight">
                Your cluster is ready
              </h2>
              <p className="font-rh-text text-rh-gray-60 text-base mt-2">
                Use the details below to access your Red Hat AI environment.
              </p>
            </div>

            <div className="grid gap-6 lg:grid-cols-2">
              {/* AI Console URL Card */}
              <div
                className="bg-white border border-rh-gray-20 animate-fade-in-up"
                style={{ animationDelay: '0.1s' }}
              >
                <div className="border-b border-rh-gray-20 px-6 py-4 flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <div className="w-2 h-2 rounded-full bg-rh-red-50 animate-pulse-red" />
                    <h3 className="font-rh-display text-rh-gray-95 text-base font-bold">
                      AI Console URL
                    </h3>
                  </div>
                  <button
                    onClick={() => copyToClipboard(cluster.aiConsoleURL, "aiurl")}
                    className="flex items-center gap-2 px-3 py-1.5 text-sm font-rh-text font-medium text-rh-gray-60 border border-rh-gray-20 hover:border-rh-gray-50 hover:text-rh-gray-95 transition-colors"
                  >
                    {copied === "aiurl" ? <CheckIcon /> : <CopyIcon />}
                    <span>{copied === "aiurl" ? "Copied" : "Copy"}</span>
                  </button>
                </div>
                <div className="px-6 py-5">
                  <a
                    href={cluster.aiConsoleURL}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="group inline-flex items-center gap-2 font-rh-text text-rh-red-50 hover:text-rh-red-60 text-base break-all transition-colors"
                  >
                    <span className="underline underline-offset-2 decoration-rh-red-50/40 group-hover:decoration-rh-red-60">
                      {cluster.aiConsoleURL}
                    </span>
                    <ExternalIcon />
                  </a>
                  <p className="mt-3 font-rh-text text-sm text-rh-gray-60">
                    Access Red Hat OpenShift AI.<br /><br />Use the <span className="font-bold text-rh-gray-95">admin</span> user and your password to login. It may take a moment for the cluster to be ready.
                  </p>
                </div>
              </div>

              {/* OpenShift Console URL Card */}
              <div
                className="bg-white border border-rh-gray-20 animate-fade-in-up"
                style={{ animationDelay: '0.15s' }}
              >
                <div className="border-b border-rh-gray-20 px-6 py-4 flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <div className="w-2 h-2 rounded-full bg-rh-red-50 animate-pulse-red" />
                    <h3 className="font-rh-display text-rh-gray-95 text-base font-bold">
                      OpenShift Console URL
                    </h3>
                  </div>
                  <button
                    onClick={() => copyToClipboard(cluster.webConsoleURL, "url")}
                    className="flex items-center gap-2 px-3 py-1.5 text-sm font-rh-text font-medium text-rh-gray-60 border border-rh-gray-20 hover:border-rh-gray-50 hover:text-rh-gray-95 transition-colors"
                  >
                    {copied === "url" ? <CheckIcon /> : <CopyIcon />}
                    <span>{copied === "url" ? "Copied" : "Copy"}</span>
                  </button>
                </div>
                <div className="px-6 py-5">
                  <a
                    href={cluster.webConsoleURL}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="group inline-flex items-center gap-2 font-rh-text text-rh-red-50 hover:text-rh-red-60 text-base break-all transition-colors"
                  >
                    <span className="underline underline-offset-2 decoration-rh-red-50/40 group-hover:decoration-rh-red-60">
                      {cluster.webConsoleURL}
                    </span>
                    <ExternalIcon />
                  </a>
                  <p className="mt-3 font-rh-text text-sm text-rh-gray-60">
                    Access Red Hat OpenShift cluster.<br /><br />Use the <span className="font-bold text-rh-gray-95">admin</span> user and your password to login. It may take a moment for the cluster to be ready.
                  </p>
                </div>
              </div>

              {/* Cluster Lifetime Card */}
              <div
                className="bg-white border border-rh-gray-20 animate-fade-in-up"
                style={{ animationDelay: '0.2s' }}
              >
                <div className="border-b border-rh-gray-20 px-6 py-4">
                  <div className="flex items-center gap-3">
                    <ClockIcon />
                    <h3 className="font-rh-display text-rh-gray-95 text-base font-bold">
                      Cluster Lifetime
                    </h3>
                  </div>
                </div>
                <CountdownTimer expiresAt={cluster.expiresAt} />
              </div>

              {/* Kubeconfig Card */}
              <div
                className="bg-white border border-rh-gray-20 lg:row-span-1 animate-fade-in-up"
                style={{ animationDelay: '0.25s' }}
              >
                <div className="border-b border-rh-gray-20 px-6 py-4 flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <div className="w-2 h-2 rounded-full bg-rh-red-50 animate-pulse-red" />
                    <h3 className="font-rh-display text-rh-gray-95 text-base font-bold">
                      Kubeconfig
                    </h3>
                  </div>
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => downloadKubeconfig(cluster.kubeconfig)}
                      className="flex items-center gap-2 px-3 py-1.5 text-sm font-rh-text font-medium text-rh-gray-60 border border-rh-gray-20 hover:border-rh-gray-50 hover:text-rh-gray-95 transition-colors"
                    >
                      <DownloadIcon />
                      <span>Download</span>
                    </button>
                    <button
                      onClick={() => copyToClipboard(cluster.kubeconfig, "kubeconfig")}
                      className="flex items-center gap-2 px-3 py-1.5 text-sm font-rh-text font-medium text-rh-gray-60 border border-rh-gray-20 hover:border-rh-gray-50 hover:text-rh-gray-95 transition-colors"
                    >
                      {copied === "kubeconfig" ? <CheckIcon /> : <CopyIcon />}
                      <span>{copied === "kubeconfig" ? "Copied" : "Copy"}</span>
                    </button>
                  </div>
                </div>
                <div className="px-6 py-5">
                  <pre className="font-mono text-sm text-rh-gray-80 bg-rh-gray-95 border border-rh-gray-90 p-4 overflow-x-auto max-h-72 overflow-y-auto whitespace-pre-wrap leading-relaxed text-rh-gray-30">
                    {cluster.kubeconfig}
                  </pre>
                </div>
              </div>
            </div>
          </div>
        </section>
      )}

      {/* ── Footer ── */}
      <footer className="bg-rh-gray-95 border-t border-rh-gray-80">
        <div className="max-w-7xl mx-auto px-6 lg:px-8 py-8">
          <div className="flex flex-col sm:flex-row items-center justify-between gap-4">
            <div className="flex items-center gap-2">
              <div className="w-5 h-5 relative flex items-center justify-center">
                <div className="absolute w-full h-0.5 bg-rh-red-50 top-0.5 rounded-sm" />
                <div className="w-3 h-3 bg-rh-red-50 rounded-sm mt-0.5" />
              </div>
              <span className="font-rh-display text-rh-gray-40 text-sm font-medium">
                Prelude
              </span>
            </div>
            <p className="font-rh-text text-rh-gray-60 text-xs">
              Powered by Red Hat AI
            </p>
          </div>
        </div>
      </footer>
    </div>
  );
}

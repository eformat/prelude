"use client";

import { useState, useEffect, useRef } from "react";
import { useGoogleReCaptcha } from "react-google-recaptcha-v3";
import { RecaptchaVerifier, signInWithPhoneNumber, ConfirmationResult } from "firebase/auth";
import { auth } from "./firebase";
import { claimCluster, checkClusterReady } from "./actions";
import { getFingerprint } from "./fingerprint";

interface ClusterInfo {
  webConsoleURL: string;
  aiConsoleURL: string;
  kubeconfig: string;
  expiresAt: string;
  passwordChanged: boolean;
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

function PreparingCountdown({ deadline }: { deadline: number }) {
  const [secondsLeft, setSecondsLeft] = useState(() =>
    Math.max(0, Math.ceil((deadline - Date.now()) / 1000))
  );

  useEffect(() => {
    const timer = setInterval(() => {
      const remaining = Math.max(0, Math.ceil((deadline - Date.now()) / 1000));
      setSecondsLeft(remaining);
      if (remaining <= 0) clearInterval(timer);
    }, 1000);
    return () => clearInterval(timer);
  }, [deadline]);

  return (
    <span className="font-mono text-lg text-rh-gray-95 font-bold">
      {secondsLeft}s
    </span>
  );
}

const COUNTRIES = [
  { code: "US", dial: "+1", name: "United States", flag: "\u{1F1FA}\u{1F1F8}" },
  { code: "AU", dial: "+61", name: "Australia", flag: "\u{1F1E6}\u{1F1FA}" },
  { code: "AF", dial: "+93", name: "Afghanistan", flag: "\u{1F1E6}\u{1F1EB}" },
  { code: "AL", dial: "+355", name: "Albania", flag: "\u{1F1E6}\u{1F1F1}" },
  { code: "DZ", dial: "+213", name: "Algeria", flag: "\u{1F1E9}\u{1F1FF}" },
  { code: "AR", dial: "+54", name: "Argentina", flag: "\u{1F1E6}\u{1F1F7}" },
  { code: "AT", dial: "+43", name: "Austria", flag: "\u{1F1E6}\u{1F1F9}" },
  { code: "BD", dial: "+880", name: "Bangladesh", flag: "\u{1F1E7}\u{1F1E9}" },
  { code: "BE", dial: "+32", name: "Belgium", flag: "\u{1F1E7}\u{1F1EA}" },
  { code: "BR", dial: "+55", name: "Brazil", flag: "\u{1F1E7}\u{1F1F7}" },
  { code: "CA", dial: "+1", name: "Canada", flag: "\u{1F1E8}\u{1F1E6}" },
  { code: "CL", dial: "+56", name: "Chile", flag: "\u{1F1E8}\u{1F1F1}" },
  { code: "CN", dial: "+86", name: "China", flag: "\u{1F1E8}\u{1F1F3}" },
  { code: "CO", dial: "+57", name: "Colombia", flag: "\u{1F1E8}\u{1F1F4}" },
  { code: "CZ", dial: "+420", name: "Czech Republic", flag: "\u{1F1E8}\u{1F1FF}" },
  { code: "DK", dial: "+45", name: "Denmark", flag: "\u{1F1E9}\u{1F1F0}" },
  { code: "EG", dial: "+20", name: "Egypt", flag: "\u{1F1EA}\u{1F1EC}" },
  { code: "FI", dial: "+358", name: "Finland", flag: "\u{1F1EB}\u{1F1EE}" },
  { code: "FR", dial: "+33", name: "France", flag: "\u{1F1EB}\u{1F1F7}" },
  { code: "DE", dial: "+49", name: "Germany", flag: "\u{1F1E9}\u{1F1EA}" },
  { code: "GH", dial: "+233", name: "Ghana", flag: "\u{1F1EC}\u{1F1ED}" },
  { code: "GR", dial: "+30", name: "Greece", flag: "\u{1F1EC}\u{1F1F7}" },
  { code: "HK", dial: "+852", name: "Hong Kong", flag: "\u{1F1ED}\u{1F1F0}" },
  { code: "HU", dial: "+36", name: "Hungary", flag: "\u{1F1ED}\u{1F1FA}" },
  { code: "IN", dial: "+91", name: "India", flag: "\u{1F1EE}\u{1F1F3}" },
  { code: "ID", dial: "+62", name: "Indonesia", flag: "\u{1F1EE}\u{1F1E9}" },
  { code: "IE", dial: "+353", name: "Ireland", flag: "\u{1F1EE}\u{1F1EA}" },
  { code: "IL", dial: "+972", name: "Israel", flag: "\u{1F1EE}\u{1F1F1}" },
  { code: "IT", dial: "+39", name: "Italy", flag: "\u{1F1EE}\u{1F1F9}" },
  { code: "JP", dial: "+81", name: "Japan", flag: "\u{1F1EF}\u{1F1F5}" },
  { code: "KE", dial: "+254", name: "Kenya", flag: "\u{1F1F0}\u{1F1EA}" },
  { code: "KR", dial: "+82", name: "South Korea", flag: "\u{1F1F0}\u{1F1F7}" },
  { code: "MY", dial: "+60", name: "Malaysia", flag: "\u{1F1F2}\u{1F1FE}" },
  { code: "MX", dial: "+52", name: "Mexico", flag: "\u{1F1F2}\u{1F1FD}" },
  { code: "NL", dial: "+31", name: "Netherlands", flag: "\u{1F1F3}\u{1F1F1}" },
  { code: "NZ", dial: "+64", name: "New Zealand", flag: "\u{1F1F3}\u{1F1FF}" },
  { code: "NG", dial: "+234", name: "Nigeria", flag: "\u{1F1F3}\u{1F1EC}" },
  { code: "NO", dial: "+47", name: "Norway", flag: "\u{1F1F3}\u{1F1F4}" },
  { code: "PK", dial: "+92", name: "Pakistan", flag: "\u{1F1F5}\u{1F1F0}" },
  { code: "PE", dial: "+51", name: "Peru", flag: "\u{1F1F5}\u{1F1EA}" },
  { code: "PH", dial: "+63", name: "Philippines", flag: "\u{1F1F5}\u{1F1ED}" },
  { code: "PL", dial: "+48", name: "Poland", flag: "\u{1F1F5}\u{1F1F1}" },
  { code: "PT", dial: "+351", name: "Portugal", flag: "\u{1F1F5}\u{1F1F9}" },
  { code: "RO", dial: "+40", name: "Romania", flag: "\u{1F1F7}\u{1F1F4}" },
  { code: "SA", dial: "+966", name: "Saudi Arabia", flag: "\u{1F1F8}\u{1F1E6}" },
  { code: "SG", dial: "+65", name: "Singapore", flag: "\u{1F1F8}\u{1F1EC}" },
  { code: "ZA", dial: "+27", name: "South Africa", flag: "\u{1F1FF}\u{1F1E6}" },
  { code: "ES", dial: "+34", name: "Spain", flag: "\u{1F1EA}\u{1F1F8}" },
  { code: "SE", dial: "+46", name: "Sweden", flag: "\u{1F1F8}\u{1F1EA}" },
  { code: "CH", dial: "+41", name: "Switzerland", flag: "\u{1F1E8}\u{1F1ED}" },
  { code: "TW", dial: "+886", name: "Taiwan", flag: "\u{1F1F9}\u{1F1FC}" },
  { code: "TH", dial: "+66", name: "Thailand", flag: "\u{1F1F9}\u{1F1ED}" },
  { code: "TR", dial: "+90", name: "Turkey", flag: "\u{1F1F9}\u{1F1F7}" },
  { code: "UA", dial: "+380", name: "Ukraine", flag: "\u{1F1FA}\u{1F1E6}" },
  { code: "AE", dial: "+971", name: "United Arab Emirates", flag: "\u{1F1E6}\u{1F1EA}" },
  { code: "GB", dial: "+44", name: "United Kingdom", flag: "\u{1F1EC}\u{1F1E7}" },
  { code: "VN", dial: "+84", name: "Vietnam", flag: "\u{1F1FB}\u{1F1F3}" },
];

function ChevronDownIcon() {
  return (
    <svg width="12" height="12" viewBox="0 0 12 12" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M3 4.5L6 7.5L9 4.5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

export default function Home() {
  const [phone, setPhone] = useState("");
  const [selectedCountry, setSelectedCountry] = useState(COUNTRIES[0]);
  const [countryDropdownOpen, setCountryDropdownOpen] = useState(false);
  const [countrySearch, setCountrySearch] = useState("");
  const countryDropdownRef = useRef<HTMLDivElement>(null);
  const countrySearchRef = useRef<HTMLInputElement>(null);
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [hideKubeconfig, setHideKubeconfig] = useState(false);
  const [htpassEnabled, setHtpassEnabled] = useState(true);
  const [cluster, setCluster] = useState<ClusterInfo | null>(null);
  const [preparing, setPreparing] = useState(false);
  const [preparingDeadline, setPreparingDeadline] = useState(0);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [copied, setCopied] = useState<string | null>(null);
  const [step, setStep] = useState<"input" | "verify">("input");
  const [verificationCode, setVerificationCode] = useState("");
  const [confirmationResult, setConfirmationResult] = useState<ConfirmationResult | null>(null);
  const recaptchaVerifierRef = useRef<RecaptchaVerifier | null>(null);
  const { executeRecaptcha } = useGoogleReCaptcha();

  // Fetch runtime config from server
  useEffect(() => {
    fetch("/api/config")
      .then((res) => res.json())
      .then((data) => {
        if (data.hideKubeconfig) {
          setHideKubeconfig(true);
        }
        if (data.createHtpassSecret === false) {
          setHtpassEnabled(false);
        }
      })
      .catch(() => {});
  }, []);

  // Close country dropdown on outside click
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (countryDropdownRef.current && !countryDropdownRef.current.contains(e.target as Node)) {
        setCountryDropdownOpen(false);
        setCountrySearch("");
      }
    }
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  // Focus search input when dropdown opens
  useEffect(() => {
    if (countryDropdownOpen && countrySearchRef.current) {
      countrySearchRef.current.focus();
    }
  }, [countryDropdownOpen]);

  const filteredCountries = countrySearch
    ? COUNTRIES.filter(
        (c) =>
          c.name.toLowerCase().includes(countrySearch.toLowerCase()) ||
          c.dial.includes(countrySearch) ||
          c.code.toLowerCase().includes(countrySearch.toLowerCase())
      )
    : COUNTRIES;

  const fullPhoneNumber = `${selectedCountry.dial}${phone.replace(/\D/g, "")}`;

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

  async function handleSendCode(e: React.FormEvent) {
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
      // Clean up any existing verifier
      if (recaptchaVerifierRef.current) {
        recaptchaVerifierRef.current.clear();
        recaptchaVerifierRef.current = null;
      }

      const verifier = new RecaptchaVerifier(auth, "send-code-button", {
        size: "invisible",
      });
      recaptchaVerifierRef.current = verifier;

      const result = await signInWithPhoneNumber(auth, fullPhoneNumber, verifier);
      setConfirmationResult(result);
      setStep("verify");
    } catch (err: unknown) {
      // Clean up verifier on error so it can be recreated on retry
      if (recaptchaVerifierRef.current) {
        recaptchaVerifierRef.current.clear();
        recaptchaVerifierRef.current = null;
      }
      const firebaseError = err as { code?: string; message?: string };
      if (firebaseError.code === "auth/invalid-phone-number") {
        setError("Invalid phone number. Please check the number and try again.");
      } else if (firebaseError.code === "auth/too-many-requests") {
        setError("Too many attempts. Please try again later.");
      } else if (firebaseError.code === "auth/quota-exceeded") {
        setError("SMS quota exceeded. Please try again later.");
      } else {
        setError(firebaseError.message || "Failed to send verification code. Please try again.");
      }
    } finally {
      setLoading(false);
    }
  }

  async function handleVerifyAndClaim(e: React.FormEvent) {
    e.preventDefault();
    setError("");

    if (!confirmationResult) {
      setError("Verification session expired. Please go back and try again.");
      return;
    }

    setLoading(true);

    try {
      await confirmationResult.confirm(verificationCode);

      // Phone verified — proceed with claim
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

      const result = await claimCluster(fullPhoneNumber, password, recaptchaToken, fingerprint);

      if (!result.success) {
        setError(result.error);
        return;
      }

      if (result.data.passwordChanged && htpassEnabled) {
        // Password was changed — wait for authentication operator to roll
        const deadline = Date.now() + 60000;
        setPreparingDeadline(deadline);
        setPreparing(true);
        setLoading(false);

        // Wait for the authentication operator to start rolling before polling
        await new Promise((resolve) => setTimeout(resolve, 20000));

        // Poll until the authentication operator is ready or timeout
        while (Date.now() < deadline) {
          const ready = await checkClusterReady(phone);
          if (ready) break;
          await new Promise((resolve) => setTimeout(resolve, 3000));
        }

        setPreparing(false);
      }

      setCluster(result.data);
    } catch (err: unknown) {
      const firebaseError = err as { code?: string; message?: string };
      if (firebaseError.code === "auth/invalid-verification-code") {
        setError("Invalid verification code. Please check and try again.");
      } else if (firebaseError.code === "auth/code-expired") {
        setError("Verification code has expired. Please go back and resend.");
      } else {
        setError(firebaseError.message || "Verification failed. Please try again.");
      }
    } finally {
      setLoading(false);
    }
  }

  function handleBackToInput() {
    setStep("input");
    setVerificationCode("");
    setConfirmationResult(null);
    setError("");
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
            {step === "input" ? (
              <form
                onSubmit={handleSendCode}
                className="animate-fade-in-up"
                style={{ animationDelay: '0.2s' }}
              >
                <div className="flex flex-col gap-4 max-w-xl">
                  <div className="flex flex-col sm:flex-row gap-4">
                    <div className="flex-1 relative flex">
                      <label htmlFor="phone" className="sr-only">Phone number</label>
                      <div className="relative" ref={countryDropdownRef}>
                        <button
                          type="button"
                          onClick={() => setCountryDropdownOpen(!countryDropdownOpen)}
                          className="flex items-center gap-2 px-4 py-4 bg-rh-gray-90 border border-rh-gray-70 border-r-0 text-white font-rh-text text-base hover:bg-rh-gray-80 transition-colors whitespace-nowrap h-full"
                          aria-label="Select country code"
                        >
                          <span className="text-lg leading-none">{selectedCountry.flag}</span>
                          <span className="text-rh-gray-30">{selectedCountry.dial}</span>
                          <ChevronDownIcon />
                        </button>
                        {countryDropdownOpen && (
                          <div className="absolute top-full left-0 mt-1 w-72 bg-rh-gray-90 border border-rh-gray-70 shadow-xl z-50 max-h-64 overflow-hidden flex flex-col">
                            <div className="p-2 border-b border-rh-gray-70">
                              <input
                                ref={countrySearchRef}
                                type="text"
                                value={countrySearch}
                                onChange={(e) => setCountrySearch(e.target.value)}
                                placeholder="Search countries..."
                                className="w-full px-3 py-2 bg-rh-gray-80 border border-rh-gray-70 text-white font-rh-text text-sm placeholder-rh-gray-50 focus:outline-none focus:border-rh-red-50 transition-colors"
                              />
                            </div>
                            <div className="overflow-y-auto">
                              {filteredCountries.map((country) => (
                                <button
                                  key={country.code}
                                  type="button"
                                  onClick={() => {
                                    setSelectedCountry(country);
                                    setCountryDropdownOpen(false);
                                    setCountrySearch("");
                                  }}
                                  className={`w-full flex items-center gap-3 px-4 py-2.5 text-left hover:bg-rh-gray-80 transition-colors ${
                                    selectedCountry.code === country.code ? "bg-rh-gray-80" : ""
                                  }`}
                                >
                                  <span className="text-lg leading-none">{country.flag}</span>
                                  <span className="font-rh-text text-white text-sm flex-1 truncate">{country.name}</span>
                                  <span className="font-rh-text text-rh-gray-40 text-sm">{country.dial}</span>
                                </button>
                              ))}
                              {filteredCountries.length === 0 && (
                                <div className="px-4 py-3 font-rh-text text-rh-gray-50 text-sm">
                                  No countries found
                                </div>
                              )}
                            </div>
                          </div>
                        )}
                      </div>
                      <input
                        id="phone"
                        type="tel"
                        value={phone}
                        onChange={(e) => setPhone(e.target.value.replace(/[^\d\s\-()]/g, ""))}
                        placeholder="Phone number"
                        className="flex-1 min-w-0 px-4 py-4 bg-rh-gray-90 border border-rh-gray-70 text-white font-rh-text text-base placeholder-rh-gray-50 focus:outline-none focus:border-rh-red-50 focus:ring-1 focus:ring-rh-red-50 transition-colors"
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
                    id="send-code-button"
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
                        <span>Sending code</span>
                      </>
                    ) : (
                      <>
                        <span>Send verification code</span>
                        <ArrowIcon />
                      </>
                    )}
                  </button>
                </div>
              </form>
            ) : (
              <form
                onSubmit={handleVerifyAndClaim}
                className="animate-fade-in-up"
                style={{ animationDelay: '0.2s' }}
              >
                <div className="flex flex-col gap-4 max-w-xl">
                  <div className="px-5 py-3 bg-rh-gray-90 border border-rh-gray-70 flex items-center justify-between">
                    <span className="font-rh-text text-white text-base">
                      <span className="text-lg mr-2">{selectedCountry.flag}</span>
                      {selectedCountry.dial} {phone}
                    </span>
                    <button
                      type="button"
                      onClick={handleBackToInput}
                      className="font-rh-text text-rh-red-50 hover:text-rh-red-40 text-sm font-medium transition-colors"
                    >
                      Change number
                    </button>
                  </div>
                  <div className="relative">
                    <label htmlFor="verification-code" className="sr-only">Verification code</label>
                    <input
                      id="verification-code"
                      type="text"
                      inputMode="numeric"
                      autoComplete="one-time-code"
                      value={verificationCode}
                      onChange={(e) => setVerificationCode(e.target.value.replace(/\D/g, "").slice(0, 6))}
                      placeholder="Enter 6-digit code"
                      className="w-full px-5 py-4 bg-rh-gray-90 border border-rh-gray-70 text-white font-rh-text text-base placeholder-rh-gray-50 focus:outline-none focus:border-rh-red-50 focus:ring-1 focus:ring-rh-red-50 transition-colors tracking-widest text-center text-xl"
                      maxLength={6}
                      autoFocus
                      required
                    />
                  </div>
                  <p className="font-rh-text text-rh-gray-50 text-sm">
                    A verification code has been sent to your phone via SMS.
                  </p>
                  <button
                    type="submit"
                    disabled={loading || verificationCode.length !== 6}
                    className="group flex items-center justify-center gap-3 px-8 py-4 bg-rh-red-50 text-white font-rh-text font-bold text-base hover:bg-rh-red-60 disabled:bg-rh-gray-70 disabled:text-rh-gray-50 transition-all duration-200"
                  >
                    {loading ? (
                      <>
                        <svg className="animate-spin h-5 w-5" viewBox="0 0 24 24">
                          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="3" fill="none" />
                          <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                        </svg>
                        <span>Verifying & claiming cluster</span>
                      </>
                    ) : (
                      <>
                        <span>Verify & get cluster</span>
                        <ArrowIcon />
                      </>
                    )}
                  </button>
                </div>
              </form>
            )}

            {/* Error Message */}
            {error && (
              <div className="mt-6 max-w-xl animate-fade-in-up">
                {error === "all_clusters_in_use" ? (
                  <div className="px-6 py-5 bg-rh-gray-80 border border-rh-gray-70 text-center">
                    <p className="font-rh-text text-white text-lg leading-relaxed">
                      Sorry, all of our clusters are in use at the moment, try again later.
                    </p>
                  </div>
                ) : error === "cluster_unavailable" ? (
                  <div className="px-6 py-5 bg-rh-gray-80 border border-rh-gray-70 text-center">
                    <p className="font-rh-text text-white text-lg leading-relaxed">
                      The assigned cluster is no longer available. Please try again to get a new cluster.
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

      {/* ── Preparing Cluster Spinner ── */}
      {preparing && (
        <section className="bg-rh-gray-10">
          <div className="max-w-7xl mx-auto px-6 lg:px-8 py-24 lg:py-32">
            <div className="flex flex-col items-center justify-center text-center animate-fade-in-up">
              <svg className="animate-spin h-12 w-12 text-rh-red-50 mb-6" viewBox="0 0 24 24">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="3" fill="none" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
              </svg>
              <h2 className="font-rh-display text-rh-gray-95 text-2xl sm:text-3xl font-bold tracking-tight mb-3">
                Preparing your cluster
              </h2>
              <p className="font-rh-text text-rh-gray-60 text-base max-w-md mb-4">
                Setting up your Red Hat AI environment.
              </p>
              <PreparingCountdown deadline={preparingDeadline} />
            </div>
          </div>
        </section>
      )}

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
              {!hideKubeconfig && (
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
              )}
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

"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { loginAdmin } from "../../actions";

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

export default function AdminLoginPage() {
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const router = useRouter();

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);

    try {
      const result = await loginAdmin(password);
      if (result.success) {
        router.push("/admin");
      } else {
        setError(result.error);
      }
    } catch {
      setError("An unexpected error occurred");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="min-h-screen">
      {/* Navigation Bar */}
      <nav className="bg-rh-gray-95 border-b border-rh-gray-80">
        <div className="max-w-7xl mx-auto px-6 lg:px-8">
          <div className="flex items-center justify-between h-16">
            <div className="flex items-center gap-3">
              <div className="w-9 h-9 relative flex items-center justify-center">
                <div className="absolute w-full h-1 bg-rh-red-50 top-1.5 rounded-sm" />
                <div className="w-6 h-5 bg-rh-red-50 rounded-sm mt-1" />
              </div>
              <a href="/" className="font-rh-display text-white text-xl font-bold tracking-tight hover:text-rh-gray-30 transition-colors">
                Prelude
              </a>
            </div>
            <div className="flex items-center gap-6">
              <span className="font-rh-text text-rh-gray-40 text-sm">
                Admin Login
              </span>
            </div>
          </div>
        </div>
      </nav>

      {/* Login Section */}
      <section className="bg-rh-gray-95">
        <div className="max-w-7xl mx-auto px-6 lg:px-8 py-20 lg:py-28">
          <div className="max-w-md mx-auto">
            <div className="w-10 h-0.5 bg-rh-red-50 mb-6" />
            <h1 className="font-rh-display text-white text-2xl sm:text-3xl font-bold tracking-tight mb-3">
              Admin Dashboard
            </h1>
            <p className="font-rh-text text-rh-gray-40 text-base mb-8">
              Enter the admin password to access the cluster dashboard.
            </p>

            <form onSubmit={handleSubmit}>
              <div className="flex flex-col gap-4">
                <div className="relative">
                  <label htmlFor="admin-password" className="sr-only">Admin password</label>
                  <input
                    id="admin-password"
                    type={showPassword ? "text" : "password"}
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    placeholder="Admin password"
                    className="w-full px-5 py-4 pr-12 bg-rh-gray-90 border border-rh-gray-70 text-white font-rh-text text-base placeholder-rh-gray-50 focus:outline-none focus:border-rh-red-50 focus:ring-1 focus:ring-rh-red-50 transition-colors"
                    required
                    autoFocus
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
                <button
                  type="submit"
                  disabled={loading}
                  className="flex items-center justify-center gap-3 px-8 py-4 bg-rh-red-50 text-white font-rh-text font-bold text-base hover:bg-rh-red-60 disabled:bg-rh-gray-70 disabled:text-rh-gray-50 transition-all duration-200"
                >
                  {loading ? (
                    <>
                      <svg className="animate-spin h-5 w-5" viewBox="0 0 24 24">
                        <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="3" fill="none" />
                        <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                      </svg>
                      <span>Signing in</span>
                    </>
                  ) : (
                    <span>Sign in</span>
                  )}
                </button>
              </div>
            </form>

            {error && (
              <div className="mt-6">
                <div className="flex items-start gap-3 px-5 py-4 bg-rh-red-80 border border-rh-red-70">
                  <svg width="20" height="20" viewBox="0 0 20 20" fill="none" className="mt-0.5 flex-shrink-0">
                    <circle cx="10" cy="10" r="9" stroke="#F56E6E" strokeWidth="1.5" />
                    <path d="M10 6V11" stroke="#F56E6E" strokeWidth="1.5" strokeLinecap="round" />
                    <circle cx="10" cy="14" r="0.75" fill="#F56E6E" />
                  </svg>
                  <p className="font-rh-text text-rh-red-30 text-sm leading-relaxed">{error}</p>
                </div>
              </div>
            )}
          </div>
        </div>
        <div className="h-px bg-gradient-to-r from-rh-red-50 via-rh-red-50/20 to-transparent" />
      </section>

      {/* Footer */}
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

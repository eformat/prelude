"use client";

import { useState, useEffect, useCallback } from "react";
import {
  getAdminData,
  AdminClaimInfo,
  AdminDeploymentInfo,
} from "../actions";

function RefreshIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M13.5 2.5V6H10" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M2.5 13.5V10H6" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M3.7 5.5A5.5 5.5 0 0 1 13.1 4.1L13.5 6M2.5 10L2.9 11.9A5.5 5.5 0 0 0 12.3 10.5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

export default function AdminPage() {
  const [claims, setClaims] = useState<AdminClaimInfo[]>([]);
  const [deployments, setDeployments] = useState<AdminDeploymentInfo[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [lastRefresh, setLastRefresh] = useState<Date | null>(null);
  const [utcNow, setUtcNow] = useState("");

  const fetchData = useCallback(async () => {
    setLoading(true);
    setError("");
    const result = await getAdminData();
    if (result.success) {
      setClaims(result.data.clusterClaims);
      setDeployments(result.data.clusterDeployments);
      setLastRefresh(new Date());
    } else {
      setError(result.error);
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    fetchData();
    const interval = setInterval(fetchData, 30000);
    return () => clearInterval(interval);
  }, [fetchData]);

  useEffect(() => {
    function updateUTC() {
      setUtcNow(new Date().toISOString().replace("T", " ").substring(0, 19) + " UTC");
    }
    updateUTC();
    const interval = setInterval(updateUTC, 1000);
    return () => clearInterval(interval);
  }, []);

  const readyClaims = claims.filter((c) => c.authenticated);
  const availableClaims = claims.filter((c) => c.authenticated && !c.phone);
  const claimedClaims = claims.filter((c) => c.authenticated && c.phone);

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
                Admin Dashboard
              </span>
            </div>
          </div>
        </div>
      </nav>

      {/* Header Section */}
      <section className="bg-rh-gray-95">
        <div className="max-w-7xl mx-auto px-6 lg:px-8 py-10">
          <div className="flex items-center justify-between">
            <div>
              <div className="w-10 h-0.5 bg-rh-red-50 mb-4" />
              <h1 className="font-rh-display text-white text-2xl sm:text-3xl font-bold tracking-tight">
                Cluster Status
              </h1>
              <p className="font-mono text-rh-gray-40 text-sm mt-1">{utcNow}</p>
              {lastRefresh && (
                <p className="font-rh-text text-rh-gray-50 text-xs mt-0.5">
                  Last updated {lastRefresh.toLocaleTimeString()}
                </p>
              )}
            </div>
            <button
              onClick={fetchData}
              disabled={loading}
              className="flex items-center gap-2 px-4 py-2 bg-rh-gray-80 text-rh-gray-30 font-rh-text text-sm font-medium border border-rh-gray-70 hover:border-rh-gray-50 hover:text-white disabled:opacity-50 transition-colors"
            >
              <span className={loading ? "animate-spin" : ""}>
                <RefreshIcon />
              </span>
              <span>Refresh</span>
            </button>
          </div>

          {/* Summary Tiles */}
          <div className="grid grid-cols-2 sm:grid-cols-5 gap-4 mt-8">
            <div className="bg-rh-gray-90 border border-rh-gray-70 px-5 py-4">
              <p className="font-rh-text text-rh-gray-40 text-xs uppercase tracking-wider">Deployments</p>
              <p className="font-rh-display text-white text-2xl font-bold mt-1">{deployments.length}</p>
            </div>
            <div className="bg-rh-gray-90 border border-rh-gray-70 px-5 py-4">
              <p className="font-rh-text text-rh-gray-40 text-xs uppercase tracking-wider">Claims</p>
              <p className="font-rh-display text-white text-2xl font-bold mt-1">{claims.length}</p>
            </div>
            <div className="bg-rh-gray-90 border border-rh-gray-70 px-5 py-4">
              <p className="font-rh-text text-rh-gray-40 text-xs uppercase tracking-wider">Ready</p>
              <p className="font-rh-display text-white text-2xl font-bold mt-1">{readyClaims.length}</p>
            </div>
            <div className="bg-rh-gray-90 border border-rh-gray-70 px-5 py-4">
              <p className="font-rh-text text-rh-gray-40 text-xs uppercase tracking-wider">Claimed</p>
              <p className="font-rh-display text-white text-2xl font-bold mt-1">{claimedClaims.length}</p>
            </div>
            <div className="bg-rh-gray-90 border border-orange-700/40 px-5 py-4">
              <p className="font-rh-text text-orange-400 text-xs uppercase tracking-wider">Available</p>
              <p className="font-rh-display text-orange-400 text-2xl font-bold mt-1">{availableClaims.length}</p>
            </div>
          </div>
        </div>
        <div className="h-px bg-gradient-to-r from-rh-red-50 via-rh-red-50/20 to-transparent" />
      </section>

      {/* Error */}
      {error && (
        <div className="max-w-7xl mx-auto px-6 lg:px-8 mt-6">
          <div className="flex items-start gap-3 px-5 py-4 bg-rh-red-80 border border-rh-red-70">
            <p className="font-rh-text text-rh-red-30 text-sm">{error}</p>
          </div>
        </div>
      )}

      {/* Tables Section */}
      <section className="bg-rh-gray-10">
        <div className="max-w-7xl mx-auto px-6 lg:px-8 py-10 space-y-10">

          {/* Cluster Claims Table */}
          <div className="bg-white border border-rh-gray-20">
            <div className="border-b border-rh-gray-20 px-6 py-4">
              <h2 className="font-rh-display text-rh-gray-95 text-base font-bold">
                Cluster Claims
              </h2>
              <p className="font-rh-text text-rh-gray-50 text-sm mt-0.5">
                ClusterClaims matching the configured pool
              </p>
            </div>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-rh-gray-20 bg-rh-gray-10">
                    <th className="text-left px-6 py-3 font-rh-text font-semibold text-rh-gray-60 uppercase text-xs tracking-wider">Name</th>
                    <th className="text-left px-6 py-3 font-rh-text font-semibold text-rh-gray-60 uppercase text-xs tracking-wider">Phone</th>
                    <th className="text-left px-6 py-3 font-rh-text font-semibold text-rh-gray-60 uppercase text-xs tracking-wider">Auth</th>
                    <th className="text-left px-6 py-3 font-rh-text font-semibold text-rh-gray-60 uppercase text-xs tracking-wider">Available</th>
                    <th className="text-left px-6 py-3 font-rh-text font-semibold text-rh-gray-60 uppercase text-xs tracking-wider">Namespace</th>
                    <th className="text-left px-6 py-3 font-rh-text font-semibold text-rh-gray-60 uppercase text-xs tracking-wider">Expires</th>
                    <th className="text-left px-6 py-3 font-rh-text font-semibold text-rh-gray-60 uppercase text-xs tracking-wider">Age</th>
                  </tr>
                </thead>
                <tbody>
                  {claims.length === 0 && !loading && (
                    <tr>
                      <td colSpan={7} className="px-6 py-8 text-center font-rh-text text-rh-gray-50">
                        No cluster claims found
                      </td>
                    </tr>
                  )}
                  {claims.map((claim) => (
                    <tr key={claim.name} className="border-b border-rh-gray-20 last:border-b-0 hover:bg-rh-gray-10/50">
                      <td className="px-6 py-3 font-rh-text font-medium text-rh-gray-95">{claim.name}</td>
                      <td className="px-6 py-3 font-mono text-rh-gray-60 text-xs">{claim.phone || "\u2014"}</td>
                      <td className="px-6 py-3">
                        {claim.authenticated ? (
                          <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 text-xs font-rh-text font-medium bg-green-50 text-green-700 border border-green-200">
                            <span className="w-1.5 h-1.5 rounded-full bg-green-500" />
                            done
                          </span>
                        ) : (
                          <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 text-xs font-rh-text font-medium bg-yellow-50 text-yellow-700 border border-yellow-200">
                            <span className="w-1.5 h-1.5 rounded-full bg-yellow-500" />
                            pending
                          </span>
                        )}
                      </td>
                      <td className="px-6 py-3">
                        {claim.authenticated && !claim.phone ? (
                          <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 text-xs font-rh-text font-medium bg-orange-50 text-orange-700 border border-orange-200">
                            <span className="w-1.5 h-1.5 rounded-full bg-orange-500" />
                            available
                          </span>
                        ) : (
                          <span className="font-rh-text text-rh-gray-40">{"\u2014"}</span>
                        )}
                      </td>
                      <td className="px-6 py-3 font-mono text-rh-gray-60 text-xs">{claim.namespace || "\u2014"}</td>
                      <td className="px-6 py-3 font-rh-text text-rh-gray-60 text-xs">
                        {claim.expiresAt
                          ? new Date(claim.expiresAt).toLocaleString(undefined, {
                              month: "short",
                              day: "numeric",
                              hour: "2-digit",
                              minute: "2-digit",
                              timeZone: "UTC",
                              timeZoneName: "short",
                            })
                          : "\u2014"}
                      </td>
                      <td className="px-6 py-3 font-rh-text text-rh-gray-60">{claim.age}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          {/* Cluster Deployments Table */}
          <div className="bg-white border border-rh-gray-20">
            <div className="border-b border-rh-gray-20 px-6 py-4">
              <h2 className="font-rh-display text-rh-gray-95 text-base font-bold">
                Cluster Deployments
              </h2>
              <p className="font-rh-text text-rh-gray-50 text-sm mt-0.5">
                ClusterDeployments for the configured pool
              </p>
            </div>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-rh-gray-20 bg-rh-gray-10">
                    <th className="text-left px-6 py-3 font-rh-text font-semibold text-rh-gray-60 uppercase text-xs tracking-wider">Name</th>
                    <th className="text-left px-6 py-3 font-rh-text font-semibold text-rh-gray-60 uppercase text-xs tracking-wider">Platform</th>
                    <th className="text-left px-6 py-3 font-rh-text font-semibold text-rh-gray-60 uppercase text-xs tracking-wider">Region</th>
                    <th className="text-left px-6 py-3 font-rh-text font-semibold text-rh-gray-60 uppercase text-xs tracking-wider">Version</th>
                    <th className="text-left px-6 py-3 font-rh-text font-semibold text-rh-gray-60 uppercase text-xs tracking-wider">Status</th>
                    <th className="text-left px-6 py-3 font-rh-text font-semibold text-rh-gray-60 uppercase text-xs tracking-wider">Power</th>
                    <th className="text-left px-6 py-3 font-rh-text font-semibold text-rh-gray-60 uppercase text-xs tracking-wider">Age</th>
                  </tr>
                </thead>
                <tbody>
                  {deployments.length === 0 && !loading && (
                    <tr>
                      <td colSpan={7} className="px-6 py-8 text-center font-rh-text text-rh-gray-50">
                        No cluster deployments found
                      </td>
                    </tr>
                  )}
                  {deployments.map((cd) => (
                    <tr key={cd.name} className="border-b border-rh-gray-20 last:border-b-0 hover:bg-rh-gray-10/50">
                      <td className="px-6 py-3 font-rh-text font-medium text-rh-gray-95">{cd.name}</td>
                      <td className="px-6 py-3 font-rh-text text-rh-gray-60">{cd.platform || "\u2014"}</td>
                      <td className="px-6 py-3 font-rh-text text-rh-gray-60">{cd.region || "\u2014"}</td>
                      <td className="px-6 py-3 font-mono text-rh-gray-60 text-xs">{cd.version || "\u2014"}</td>
                      <td className="px-6 py-3">
                        {cd.provisionStatus === "Provisioned" ? (
                          <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 text-xs font-rh-text font-medium bg-green-50 text-green-700 border border-green-200">
                            <span className="w-1.5 h-1.5 rounded-full bg-green-500" />
                            Provisioned
                          </span>
                        ) : cd.provisionStatus === "Provisioning" ? (
                          <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 text-xs font-rh-text font-medium bg-blue-50 text-blue-700 border border-blue-200">
                            <span className="w-1.5 h-1.5 rounded-full bg-blue-500 animate-pulse" />
                            Provisioning
                          </span>
                        ) : (
                          <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 text-xs font-rh-text font-medium bg-rh-gray-10 text-rh-gray-60 border border-rh-gray-20">
                            {cd.provisionStatus || "\u2014"}
                          </span>
                        )}
                      </td>
                      <td className="px-6 py-3">
                        {cd.powerState === "Running" ? (
                          <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 text-xs font-rh-text font-medium bg-green-50 text-green-700 border border-green-200">
                            Running
                          </span>
                        ) : (
                          <span className="font-rh-text text-rh-gray-60">{cd.powerState || "\u2014"}</span>
                        )}
                      </td>
                      <td className="px-6 py-3 font-rh-text text-rh-gray-60">{cd.age}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

        </div>
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

"use server";

const API_URL = process.env.API_URL || "http://0.0.0.0:8080";

interface ClaimResult {
  success: true;
  data: {
    webConsoleURL: string;
    aiConsoleURL: string;
    kubeconfig: string;
    expiresAt: string;
  };
}

interface ClaimError {
  success: false;
  error: string;
}

export interface AdminClaimInfo {
  name: string;
  pool: string;
  phone: string;
  authenticated: boolean;
  namespace: string;
  age: string;
  expiresAt?: string;
}

export interface AdminDeploymentInfo {
  name: string;
  namespace: string;
  platform: string;
  region: string;
  version: string;
  provisionStatus: string;
  powerState: string;
  age: string;
}

export interface AdminData {
  clusterClaims: AdminClaimInfo[];
  clusterDeployments: AdminDeploymentInfo[];
}

interface AdminResult {
  success: true;
  data: AdminData;
}

interface AdminError {
  success: false;
  error: string;
}

export async function getAdminData(): Promise<AdminResult | AdminError> {
  try {
    const res = await fetch(`${API_URL}/api/admin`);
    if (!res.ok) {
      return { success: false, error: "Failed to fetch admin data" };
    }
    const data = await res.json();
    return { success: true, data };
  } catch {
    return { success: false, error: "Failed to connect to cluster service" };
  }
}

export async function claimCluster(
  phone: string,
  password: string,
  recaptchaToken: string,
  fingerprint: string
): Promise<ClaimResult | ClaimError> {
  try {
    const res = await fetch(`${API_URL}/api/claim`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ phone, password, recaptchaToken, fingerprint }),
    });

    if (!res.ok) {
      try {
        const body = await res.json();
        if (body.error === "all_clusters_in_use") {
          return { success: false, error: "all_clusters_in_use" };
        }
        if (body.error === "device_already_claimed") {
          return { success: false, error: "device_already_claimed" };
        }
      } catch {
        // not JSON, fall through
      }
      return { success: false, error: "Failed to claim cluster" };
    }

    const data = await res.json();
    return { success: true, data };
  } catch {
    return { success: false, error: "Failed to connect to cluster service" };
  }
}

"use server";

import { cookies } from "next/headers";
import { redirect } from "next/navigation";

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

interface LoginResult {
  success: true;
}

interface LoginError {
  success: false;
  error: string;
}

export async function loginAdmin(
  password: string
): Promise<LoginResult | LoginError> {
  try {
    const res = await fetch(`${API_URL}/api/admin/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ password }),
    });

    if (!res.ok) {
      return { success: false, error: "Invalid password" };
    }

    const data = await res.json();
    const cookieStore = await cookies();
    cookieStore.set("prelude-admin-session", data.token, {
      httpOnly: true,
      sameSite: "lax",
      path: "/",
      maxAge: 60 * 60 * 24, // 24 hours
    });

    return { success: true };
  } catch {
    return { success: false, error: "Failed to connect to server" };
  }
}

export async function logoutAdmin(): Promise<void> {
  const cookieStore = await cookies();
  cookieStore.delete("prelude-admin-session");
  redirect("/admin/login");
}

export async function getAdminData(): Promise<AdminResult | AdminError> {
  try {
    const cookieStore = await cookies();
    const token = cookieStore.get("prelude-admin-session")?.value || "";
    const headers: Record<string, string> = {};
    if (token) {
      headers["Authorization"] = `Bearer ${token}`;
    }
    const res = await fetch(`${API_URL}/api/admin`, { headers });
    if (res.status === 401) {
      return { success: false, error: "unauthorized" };
    }
    if (!res.ok) {
      return { success: false, error: "Failed to fetch admin data" };
    }
    const data = await res.json();
    return { success: true, data };
  } catch {
    return { success: false, error: "Failed to connect to cluster service" };
  }
}

export async function checkClusterReady(phone: string): Promise<boolean> {
  try {
    const res = await fetch(
      `${API_URL}/api/cluster/ready?phone=${encodeURIComponent(phone)}`
    );
    if (!res.ok) return false;
    const data = await res.json();
    return data.ready === true;
  } catch {
    return false;
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
        if (body.error === "cluster_unavailable") {
          return { success: false, error: "cluster_unavailable" };
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

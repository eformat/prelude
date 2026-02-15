"use server";

const API_URL = process.env.API_URL || "http://0.0.0.0:8080";

interface ClaimResult {
  success: true;
  data: {
    webConsoleURL: string;
    aiConsoleURL: string;
    kubeconfig: string;
  };
}

interface ClaimError {
  success: false;
  error: string;
}

export async function claimCluster(
  phone: string,
  password: string,
  recaptchaToken: string
): Promise<ClaimResult | ClaimError> {
  try {
    const res = await fetch(`${API_URL}/api/claim`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ phone, password, recaptchaToken }),
    });

    if (!res.ok) {
      try {
        const body = await res.json();
        if (body.error === "all_clusters_in_use") {
          return { success: false, error: "all_clusters_in_use" };
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

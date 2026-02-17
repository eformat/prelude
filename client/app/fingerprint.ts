export async function getFingerprint(): Promise<string> {
  const signals: string[] = [];

  // Canvas fingerprint
  try {
    const canvas = document.createElement("canvas");
    canvas.width = 256;
    canvas.height = 128;
    const ctx = canvas.getContext("2d");
    if (ctx) {
      ctx.fillStyle = "#f60";
      ctx.fillRect(10, 10, 100, 50);
      ctx.fillStyle = "#069";
      ctx.font = "14px Arial";
      ctx.fillText("Prelude fingerprint", 2, 90);
      ctx.strokeStyle = "rgba(102, 204, 0, 0.7)";
      ctx.beginPath();
      ctx.arc(50, 50, 30, 0, Math.PI * 2);
      ctx.stroke();
      signals.push(canvas.toDataURL());
    }
  } catch {
    // Canvas not available
  }

  // Stable navigator/screen signals
  signals.push(String(screen.width));
  signals.push(String(screen.height));
  signals.push(String(screen.colorDepth));
  signals.push(navigator.language || "");
  signals.push(String(navigator.hardwareConcurrency || 0));
  signals.push(navigator.platform || "");
  signals.push(Intl.DateTimeFormat().resolvedOptions().timeZone || "");

  const data = new TextEncoder().encode(signals.join("|"));
  const hashBuffer = await crypto.subtle.digest("SHA-256", data);
  const hashArray = Array.from(new Uint8Array(hashBuffer));
  const hashHex = hashArray.map((b) => b.toString(16).padStart(2, "0")).join("");

  return hashHex.slice(0, 16);
}

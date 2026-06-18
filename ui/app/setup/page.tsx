"use client";

import { useEffect, useState } from "react";
import { QRCodeSVG } from "qrcode.react";

const API_BASE =
  process.env.NEXT_PUBLIC_API_URL ||
  (typeof window !== "undefined"
    ? `http://${window.location.hostname}:8080`
    : "http://localhost:8080");

export default function SetupPage() {
  const [qrCode, setQrCode] = useState<string>("");

  useEffect(() => {
    const poll = setInterval(async () => {
      try {
        const res = await fetch(`${API_BASE}/api/pairing`);
        const data = await res.json();
        setQrCode(data.qr_code || "");
      } catch {
        // Backend not ready yet
      }
    }, 2000);
    return () => clearInterval(poll);
  }, []);

  return (
    <main className="min-h-screen p-8 max-w-lg mx-auto text-center">
      <h1 className="text-2xl font-bold mb-6">WhatsApp Pairing</h1>
      {qrCode ? (
        <div>
          <p className="mb-4 text-gray-600">
            Scan this code in WhatsApp &gt; Linked Devices &gt; Link a Device
          </p>
          <div className="inline-block bg-white p-4 rounded shadow">
            <QRCodeSVG value={qrCode} size={256} />
          </div>
        </div>
      ) : (
        <p className="text-gray-500">
          Waiting for QR code... Make sure the backend is running.
        </p>
      )}
    </main>
  );
}

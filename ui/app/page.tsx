"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Conversation, getConversations } from "./api";

export default function Home() {
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getConversations()
      .then(setConversations)
      .catch((e) => setError(e.message));
  }, []);

  return (
    <main className="min-h-screen p-8 max-w-2xl mx-auto">
      <h1 className="text-2xl font-bold mb-6">WhatsApp Sidecar</h1>
      {error && <p className="text-red-500 mb-4">Error: {error}</p>}
      {conversations.length === 0 && !error && (
        <p className="text-gray-500">No conversations yet.</p>
      )}
      <ul className="space-y-2">
        {conversations.map((c) => (
          <li key={c.id}>
            <Link
              href={`/conversations/${encodeURIComponent(c.id)}`}
              className="block p-4 bg-white rounded-lg shadow hover:shadow-md transition-shadow"
            >
              <div className="font-medium">{c.name}</div>
              <div className="text-sm text-gray-500">
                {c.is_group ? "Group" : "Direct"} &middot;{" "}
                {new Date(c.updated_at).toLocaleString()}
              </div>
            </Link>
          </li>
        ))}
      </ul>
    </main>
  );
}

"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { ConversationDetail, getConversation, triggerSummarize } from "../../api";

export default function ConversationPage() {
  const params = useParams();
  const id = params.id as string;
  const [data, setData] = useState<ConversationDetail | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [summarizing, setSummarizing] = useState(false);

  const load = () => {
    getConversation(decodeURIComponent(id))
      .then(setData)
      .catch((e) => setError(e.message));
  };

  useEffect(() => {
    load();
  }, [id]);

  const handleSummarize = async () => {
    setSummarizing(true);
    await triggerSummarize(decodeURIComponent(id));
    load();
    setSummarizing(false);
  };

  if (error) return <p className="p-8 text-red-500">Error: {error}</p>;
  if (!data) return <p className="p-8 text-gray-500">Loading...</p>;

  return (
    <main className="min-h-screen p-8 max-w-4xl mx-auto">
      <Link href="/" className="text-blue-600 hover:underline text-sm">
        &larr; Back
      </Link>

      <h1 className="text-2xl font-bold mt-4 mb-6">
        {data.conversation.name}
      </h1>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="lg:col-span-2 bg-white rounded-lg shadow p-4 max-h-[70vh] overflow-y-auto">
          <h2 className="font-semibold mb-3">Messages</h2>
          <div className="space-y-2">
            {[...data.messages].reverse().map((m) => (
              <div
                key={m.id}
                className={`p-2 rounded ${
                  m.is_from_me
                    ? "bg-blue-50 ml-8 text-right"
                    : "bg-gray-100 mr-8"
                }`}
              >
                <div className="text-xs text-gray-500">
                  {m.is_from_me ? "You" : m.sender} &middot;{" "}
                  {new Date(m.timestamp).toLocaleTimeString()}
                </div>
                <div>{m.content}</div>
              </div>
            ))}
          </div>
        </div>

        <div className="bg-white rounded-lg shadow p-4">
          <div className="flex items-center justify-between mb-3">
            <h2 className="font-semibold">Summary</h2>
            <button
              onClick={handleSummarize}
              disabled={summarizing}
              className="text-sm px-3 py-1 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
            >
              {summarizing ? "Summarizing..." : "Refresh"}
            </button>
          </div>
          {data.summary ? (
            <p className="text-sm text-gray-700">{data.summary.content}</p>
          ) : (
            <p className="text-sm text-gray-400">No summary yet.</p>
          )}
        </div>
      </div>
    </main>
  );
}

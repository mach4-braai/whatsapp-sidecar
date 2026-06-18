const API_BASE =
  process.env.NEXT_PUBLIC_API_URL ||
  (typeof window !== "undefined"
    ? `http://${window.location.hostname}:8080`
    : "http://localhost:8080");

export interface Conversation {
  id: string;
  name: string;
  is_group: boolean;
  updated_at: string;
}

export interface Message {
  id: string;
  conversation_id: string;
  sender: string;
  content: string;
  is_from_me: boolean;
  timestamp: string;
}

export interface Summary {
  id: number;
  conversation_id: string;
  content: string;
  created_at: string;
}

export interface ConversationDetail {
  conversation: Conversation;
  messages: Message[];
  summary: Summary | null;
}

export async function getConversations(): Promise<Conversation[]> {
  const res = await fetch(`${API_BASE}/api/conversations`, {
    cache: "no-store",
  });
  return res.json();
}

export async function getConversation(id: string): Promise<ConversationDetail> {
  const res = await fetch(
    `${API_BASE}/api/conversations/${encodeURIComponent(id)}`,
    { cache: "no-store" }
  );
  return res.json();
}

export async function triggerSummarize(id: string): Promise<void> {
  await fetch(`${API_BASE}/api/summarize/${encodeURIComponent(id)}`, {
    method: "POST",
  });
}

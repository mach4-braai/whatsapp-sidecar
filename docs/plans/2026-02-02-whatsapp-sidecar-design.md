# WhatsApp Sidecar — Design Document

**Date:** 2026-02-02
**Status:** Draft

## Overview

A sidecar app that mirrors WhatsApp conversations in a local web UI,
providing AI-generated summaries of notable events. The system is
read-only with respect to WhatsApp — it observes conversations but
never sends messages on your behalf.

## Architecture

Two services:

- **Go backend** — Connects to WhatsApp via `whatsmeow`, stores
  messages in SQLite, runs periodic summary jobs against a local
  Ollama instance (GLM-4), and exposes a REST API.
- **Next.js frontend** — Web UI that calls the Go backend API. Shows
  conversations and summaries.

### Data Flow

```
WhatsApp ──websocket──> Go backend ──store──> SQLite
                              |
                              |──periodic──> Ollama (GLM-4)
                              |                  |
                              |                  v
                              |              summaries ──> SQLite
                              |
                              └──REST API──> Next.js frontend
```

### Key Decisions

- **SQLite** for storage — single file, no external database, fits a
  personal tool
- **Ollama** for local inference — supports GLM-4, simple HTTP API,
  all data stays local
- **Periodic cron-style job** inside the Go process, no external
  scheduler needed
- **No authentication** — localhost-only personal tool

## Go Backend

Three internal packages:

### `whatsapp`

Wraps `whatsmeow`. Handles QR code pairing, maintains the linked
device session, and writes incoming/outgoing messages to SQLite as
they arrive.

### `store`

SQLite layer using `modernc.org/sqlite` (pure Go, no CGO). Tables:

- `conversations` — contact/group metadata
- `messages` — message content, timestamps, direction
- `summaries` — AI-generated conversation summaries

### `summarizer`

Periodic job (configurable interval, default every 6 hours). For each
conversation with new messages since the last summary:

1. Build a prompt with recent message history
2. Send to Ollama `/api/generate` endpoint
3. Store the resulting summary

### `api`

REST API using `net/http` (standard library):

- `GET /api/conversations` — list with latest summary preview
- `GET /api/conversations/:id` — messages and summaries
- `GET /api/pairing` — QR code data for WhatsApp linking
- `POST /api/summarize/:id` — manually trigger a summary

Binds to `localhost` only.

## Next.js Frontend

### Pages

- **`/`** — Conversation list. Contact name, last message preview,
  badge for unread summaries.
- **`/conversations/[id]`** — Single conversation with two panels:
  - Messages (scrollable history)
  - Summary (latest AI-generated summary)
- **`/setup`** — QR code pairing screen for initial WhatsApp linking.

### Stack

- Next.js with App Router
- Tailwind CSS
- `fetch` to Go backend API, React `useState`/`useEffect` or server
  components
- No state management library
- No authentication

## Project Tooling

Managed entirely via `mise`:

```toml
[tools]
go = "1.23"
node = "22"

[tasks.backend]
description = "Run the Go backend"
run = "cd backend && go run ./cmd/server"

[tasks.ui]
description = "Run the Next.js frontend"
run = "cd ui && npm run dev"

[tasks.dev]
description = "Run both backend and UI"
depends = ["backend", "ui"]

[tasks.setup]
description = "Install all dependencies"
run = """
cd backend && go mod download
cd ../ui && npm install
"""

[tasks.build]
description = "Build both services"
run = """
cd backend && go build -o bin/server ./cmd/server
cd ../ui && npm run build
"""
```

## Directory Structure

```
whatsapp-sidecar/
├── mise.toml
├── backend/
│   ├── go.mod
│   ├── cmd/server/main.go
│   └── internal/
│       ├── whatsapp/
│       ├── store/
│       ├── summarizer/
│       └── api/
└── ui/
    ├── package.json
    └── src/
```


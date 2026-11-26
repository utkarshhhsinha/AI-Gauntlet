# AI Gauntlet — Complete Project Documentation

> **Version:** 1.0.0  
> **Repository:** [utkarshhhsinha/AI-Gauntlet](https://github.com/utkarshhhsinha/AI-Gauntlet)  
> **License:** Apache-2.0 / MIT  

---

## 1. Executive Summary

**AI Gauntlet** is an open-source, self-hostable workspace where software engineering teams assign tasks to AI coding agents the same way they assign them to human colleagues. Agents appear as first-class assignees on Kanban boards and issue tables: they pick up tickets, execute code changes within developer-controlled runtimes, stream terminal execution traces in real time, report progress, raise blockers, and submit pull requests for human review.

The platform natively integrates **Cerebra**, an embedded intelligent LLM routing engine that deterministically classifies tasks across 3 complexity tiers, guarantees tool reliability via Model Context Protocol (MCP) flooring, enforces sticky session affinity, and performs automated zero-downtime failover across 26+ supported agent CLIs and local Ollama runtimes.

---

## 2. High-Level Architecture

The system is structured as a Go backend paired with a TypeScript/React monorepo (pnpm workspaces + Turborepo), backed by PostgreSQL and Redis.

```mermaid
graph TD
    subgraph Clients ["Client Layer (TypeScript / React)"]
        Web["Web App (Next.js App Router)"]
        Desktop["Desktop App (Electron)"]
        Mobile["Mobile App (Expo / React Native)"]
        Docs["Documentation (Fumadocs)"]
    end

    subgraph SharedPackages ["Shared Package Monorepo"]
        Core["@ai-gauntlet/core (Zustand + React Query + Zod)"]
        UI["@ai-gauntlet/ui (Tailwind CSS + Radix UI)"]
        Views["@ai-gauntlet/views (Kanban / Tables / Chat / Realtime)"]
    end

    subgraph Backend ["Backend Core (Go Chi Router)"]
        APIServer["API Server (:8080)"]
        WSHub["WebSocket Hub (gorilla/websocket)"]
        TaskLifecycle["Task Lifecycle & Verification Engine"]
        CerebraRouter["Cerebra Dynamic Tier Router"]
    end

    subgraph DataLayer ["Data & Storage Layer"]
        Postgres[("PostgreSQL 17 + pgvector (sqlc)")]
        RedisStore[("Redis (Stream Relay + Model Cache)")]
    end

    subgraph Workstation ["Developer Workstation / Cloud Box"]
        Daemon["ai-gauntlet CLI Daemon"]
        AgentRuntimes["26+ Agent CLIs (Claude Code, Codex, Cursor, Ollama)"]
        GitRepo["Local Git Working Tree & Sandboxes"]
    end

    Web --> Core
    Desktop --> Core
    Mobile --> Core
    Core --> APIServer
    Views --> Core
    APIServer --> Postgres
    APIServer --> RedisStore
    APIServer <-->|WebSocket Stream| WSHub
    WSHub <--> Clients
    Daemon <-->|WebSocket / RPC| APIServer
    Daemon --> AgentRuntimes
    AgentRuntimes --> GitRepo
```

---

## 3. Technology Stack

| Layer | Technologies | Purpose |
|---|---|---|
| **Backend** | Go (v1.23+), Chi Router, Gorilla WebSocket | Core API server, concurrent background task workers, full-duplex log streaming |
| **Database** | PostgreSQL 17, `pgvector`, `sqlc`, `pgx/v5` | Relational persistence, vector embeddings, type-safe compile-time SQL queries |
| **Caching / PubSub** | Redis | Sharded stream relays, runtime liveness, model catalog cache |
| **AI Routing** | Cerebra (Go Engine + Sidecar) | Sub-millisecond complexity scoring, MCP tool flooring, circuit-breaker failover |
| **Web Frontend** | Next.js (App Router), React 19, TypeScript | Primary browser-based team dashboard and task management interface |
| **Desktop App** | Electron, TypeScript, React | Native macOS, Linux, and Windows client with deep OS and daemon integration |
| **Mobile App** | Expo, React Native (iOS / Android) | Mobile notifications, issue triage, and execution review |
| **Shared State** | TanStack Query v5, Zustand, Zod | Query caching for server state, Zustand for view states, Zod for schema validation |
| **Styling & UI** | Tailwind CSS, Radix UI / Base UI, Lucide React | Atomic design system, accessible UI primitives, dark/light theme cascade |
| **DevOps & Infra** | Docker, Docker Compose, Kubernetes, Helm | Containerization, local zero-config orchestration, enterprise deployment |

---

## 4. Core Concepts & Domain Model

### 4.1 Workspaces & Roles
* **Workspace:** The multi-tenant boundary. Isolates issues, projects, agent definitions, and API keys.
* **Roles:** `owner`, `admin`, and `member`. Permissions govern which human team members can invoke, configure, or inspect specific agent runtimes.

### 4.2 Agents as First-Class Assignees
Unlike conversational chatbots, an Agent in AI Gauntlet possesses a persistent identity:
* Configured with an assigned runtime profile, system instructions, and allowed skills.
* Can be assigned to an issue directly from the Kanban board.
* Comments on issues, posts status transitions (e.g., `In Progress` $\rightarrow$ `Under Review`), and asks clarifying questions via issue mentions.

### 4.3 Squads & Autopilots
* **Squads:** Multi-agent teams containing a leader agent and specialized contributor agents (e.g., an "Architecture Squad" with a Lead Architect, Frontend Bot, and Security Reviewer).
* **Autopilots:** Scheduled cron jobs or webhook-triggered automations that run routine tasks (e.g., daily dependency vulnerability scans, stale issue triage, changelog generation).

### 4.4 Skills & MCP Tooling
* **Skills:** Versioned markdown playbooks that encode domain knowledge and repeatable workflows.
* **Model Context Protocol (MCP):** Connects agents to external tools, databases, and APIs with sandboxed permission boundaries.

---

## 5. Cerebra Intelligent Routing Engine

Cerebra is the built-in request-level model router embedded directly into the daemon and API server. It eliminates the cost-vs-intelligence tradeoff when operating fleets of autonomous agents.

```
                           Task Prompt / Issue Context
                                       │
                                       ▼
                       [Lexical & AST Code Stripper]
                                       │
                                       ▼
                     [Heuristic Complexity Classifier]
                        ├── Keyword Analysis (Refactor, Migrate, Fix)
                        ├── Token Volume Scorer (<500, 500-2000, >2000)
                        └── MCP Tool Intent Detector
                                       │
                     ┌─────────────────┼─────────────────┐
                     ▼                 ▼                 ▼
                TierSimple        TierStandard       TierHeavy
             (Haiku / Flash)    (Sonnet / GPT-4o)  (Opus / o1 / R1)
                     │                 │                 │
                     └─────────────────┼─────────────────┘
                                       │
                         [Policy & Budget Evaluator]
                                       │
                         [Session Affinity Validator]
                                       │
                      [Circuit-Breaker & Unavailability]
                                       │
                          ┌────────────┴────────────┐
                          ▼                         ▼
                    Selected Model           Live Failover (429)
```

### Routing Rules & Invariants
1. **Filter-Before-Select:** Candidates are filtered for unavailability, provider policies, and budget caps *before* selection occurs.
2. **MCP Tool Floor:** Any prompt requiring tool calling, MCP servers, or terminal commands is automatically floored at `TierStandard` to prevent execution failures on underpowered models.
3. **Sticky Session Escalation:** A task session pins its model across turns. If complexity spikes, the tier escalates and stays pinned for up to 5 consecutive turns before decaying.
4. **Self-Healing Failover:** If a model returns an HTTP `429` (rate limit) or API outage, Cerebra marks the model unavailable for a 3600-second TTL and immediately reroutes the execution to the next candidate model in the tier cascade.

---

## 6. Realtime Streaming & Execution Daemon

### 6.1 The Daemon Bridge (`ai-gauntlet`)
The local daemon is a standalone Go binary running on the developer’s workstation. It maintains a secure, bidirectional WebSocket connection to the AI Gauntlet server:
* Receives task claims dispatched by the backend.
* Spawns the appropriate local agent CLI (Claude Code, Codex, Cursor, etc.).
* Operates on the local codebase using Git worktrees.
* **Security Boundary:** Source code remains on the developer’s local machine; only execution logs, diffs, and chat events are transmitted over the wire.

### 6.2 Sharded Stream Relay
`server/internal/realtime/` features a high-throughput stream hub:
* Retains ordered log frames in Redis.
* Distributes token streams and terminal output to connected web and desktop clients with sub-50ms latency.
* Supports client reconnection without log loss via monotonic sequence IDs.

---

## 7. Project Structure

```
multica/
├── server/                          # Go Backend Service
│   ├── cmd/
│   │   ├── ai-gauntlet/             # Main server binary entry point
│   │   ├── cerebra_sidecar/         # Standalone Cerebra HTTP routing sidecar
│   │   └── migrate/                 # Database migration CLI runner
│   ├── internal/
│   │   ├── cerebra/                 # Scorer, Policy, Router, Session Affinity
│   │   ├── daemon/                  # Workstation daemon connection & protocol
│   │   ├── handler/                 # HTTP Chi REST API endpoints
│   │   ├── realtime/                # WebSocket hub, sharded Redis stream relays
│   │   └── service/                 # Core domain business logic
│   └── migrations/                  # Sequential SQL migration scripts (455+ files)
├── apps/
│   ├── web/                         # Next.js App Router web application
│   ├── desktop/                     # Electron desktop application
│   ├── mobile/                      # Expo / React Native mobile app
│   └── docs/                        # Fumadocs documentation platform
├── packages/
│   ├── core/                        # Zustand stores, TanStack Query hooks, API client
│   ├── ui/                          # Atomic Tailwind + Radix UI components
│   └── views/                       # Shared domain views (Kanban, Tables, Chat)
├── plugins/
│   └── cerebra/                     # Cerebra sidecar plugin bundle
├── docker-compose.yml               # Multi-container local deployment spec
├── Makefile                         # Unified development and build pipeline
└── README.md                        # Project landing & quickstart guide
```

---

## 8. Database & Migration Architecture

AI Gauntlet enforces strict database architecture guidelines:
* **No Database Foreign Keys:** Foreign keys (`REFERENCES`) and cascading deletes are prohibited. Entity relationships and dependent cleanups are resolved explicitly in application transactions.
* **Concurrent Index Creation:** Every database index must be constructed using `CREATE INDEX CONCURRENTLY` in dedicated single-statement migration files to prevent blocking production database writes.
* **Type-Safe Queries:** All SQL is compiled via `sqlc` into static Go structs, eliminating runtime SQL injection vulnerabilities and column mismatch bugs.

---

## 9. Quickstart & Local Setup

### Prerequisites
* Go 1.23+
* Node.js 22+ & `pnpm` 9+
* Docker & Docker Compose
* PostgreSQL 17 (with `pgvector`)

### Running Locally
```bash
# 1. Clone repository
git clone https://github.com/utkarshhhsinha/AI-Gauntlet.git
cd AI-Gauntlet

# 2. Automated bootstrap (starts PostgreSQL, runs migrations, installs dependencies)
make dev

# 3. Access web client
# Open http://localhost:3000
```

### Verification Pipeline
```bash
pnpm typecheck       # TypeScript verification across all apps and packages
pnpm test            # Run Vitest unit test suite
make test            # Run Go unit and integration tests
make check           # Full CI verification pipeline
```

---

## 10. Summary & Future Roadmap

AI Gauntlet bridges the critical gap between raw AI model capabilities and production software engineering workflows. By combining task management, human-in-the-loop review gates, multi-agent squads, and Cerebra intelligent model routing, the platform provides an extensible, vendor-agnostic foundation for the next generation of autonomous software development.

<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/logo-dark.svg">
  <source media="(prefers-color-scheme: light)" srcset="docs/assets/logo-light.svg">
  <img alt="AI Gauntlet" src="docs/assets/logo-light.svg" width="50">
</picture>

# AI Gauntlet

**Agents that show up on the board — with intelligent routing built in.**

AI Gauntlet is an open-source workspace where you assign work to AI coding agents the way you'd
assign it to a teammate — they pick up the issue, report progress, raise blockers, and hand it
back for review. Powered by Cerebra, an embedded multi-tier LLM router that dynamically dispatches tasks
to the optimal model with automatic failover. Self-hostable, works with 26 agent CLIs, zero lock-in.

[![CI](https://github.com/utkarshhhsinha/AI-Gauntlet/actions/workflows/ci.yml/badge.svg)](https://github.com/utkarshhhsinha/AI-Gauntlet/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/utkarshhhsinha/AI-Gauntlet?style=flat)](https://github.com/utkarshhhsinha/AI-Gauntlet/releases)
[![GitHub stars](https://img.shields.io/github/stars/utkarshhhsinha/AI-Gauntlet?style=flat)](https://github.com/utkarshhhsinha/AI-Gauntlet/stargazers)
[![Discord](https://img.shields.io/badge/Discord-Join-5865F2?logo=discord&logoColor=white)](https://discord.gg/W8gYBn226t)

[Website](https://github.com/utkarshhhsinha/AI-Gauntlet) · [Docs](https://github.com/utkarshhhsinha/AI-Gauntlet/docs) · [Quickstart](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/cloud-quickstart) · [Download](https://github.com/utkarshhhsinha/AI-Gauntlet/download) · [Vision](VISION.md) · [Self-Hosting](SELF_HOSTING.md) · [Discord](https://discord.gg/W8gYBn226t) · [X](https://x.com/AIGauntletAI)

**English | [简体中文](README.zh.md)**

</div>

<p align="center">
  <sub><em>Your next 10 hires won't be human.</em></sub>
</p>

---

## What is AI Gauntlet?

You already run Claude Code, Codex, and three other agents. Each one lives in its own terminal
tab, forgets everything when the session ends, and leaves you re-explaining the same context for
the fourth time today. The more agents you add, the more of your day goes to babysitting them.

AI Gauntlet puts those agents and your teammates in one workspace. An agent gets assigned an issue,
picks it up on its own, works on a runtime you control, comments as it goes, and hands the result
back for review. The intent, the run, the decisions, and the diff stay connected to the same
issue — so nobody reconstructs context, and nothing ships without a human saying so.

---

## Build the team.

*Claude Code, Codex, Cursor, Kimi — you don't pick one. You hire them all.*

- **[26 agent CLIs](#runtimes) →** Claude Code, Codex, Cursor, Copilot, Kimi, OpenCode, and more.
- **[Agents as teammates](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/agents) →** Give each one a name, a provider, and a runtime — they show up on the board like anyone else.
- **[Cerebra Intelligent Routing](#cerebra-intelligent-routing) →** Dynamically route tasks across Simple, Standard, and Heavy tiers with real-time self-healing model failover.
- **[Squads](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/squads) →** Put agents and people on one team; the leader routes the work.
- **[Skills](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/skills) →** Turn a solved problem into a playbook every agent reuses.
- **[Your own runtime](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/daemon-runtimes) →** Their desk is your machine — a daemon on your laptop or cloud box. Code never leaves it.

## Cerebra Intelligent Routing

*Deterministic tier scoring, sticky session escalation, and self-healing multi-model failover.*

AI Gauntlet embeds **Cerebra**, an intelligent real-time request-level LLM router built directly into the daemon:

- **3 Dynamic Complexity Tiers**:
  - `TierSimple` (Haiku, Gemini Flash, Mini): Lightweight, low-latency execution for summaries, greetings, and status queries.
  - `TierStandard` (Sonnet, GPT-4o, DeepSeek-Coder): High-performance coding, debugging, and MCP tool orchestration.
  - `TierHeavy` (Opus, o1/o3, DeepSeek-R1): Frontier reasoning for system architecture, complex refactoring, and migrations.
- **Deterministic Zero-Latency Dispatch**: Sub-millisecond keyword parsing and token scoring with zero external network overhead on the routing path.
- **MCP Tool Floor**: Automatically floors any task using MCP tools, plugin hooks, or connected apps to `TierStandard` or higher to guarantee tool execution accuracy.
- **Sticky Session Escalation**: Chat and task sessions stay pinned or escalate as complexity grows, keeping context cohesive without wasting tokens.
- **Self-Healing Live Failover**: Automatically detects rate limits (`429`), quota exhaustion, and API outages, seamlessly swapping to candidate fallback models in real-time without failing the parent workflow.

## Hand off the work.

*It starts as three rough sentences in an issue. It ends as a pull request.*

- **[Assign an issue](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/assigning-issues) →** Pick an agent as assignee the way you'd pick a colleague — it takes the work from there.
- **[Autopilots](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/autopilots) →** Run standups, audits, and reports on a cron — nobody to remind.
- **[Chat](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/chat) →** Ask your workspace a question, or start work without filing anything.
- **[Projects](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/projects) →** Group work and attach the repos and docs agents need as context.

## Stay in the loop.

*Which agent touched this? What did it run? What did it cost? Open the run.*

- **[Execution log](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/tasks) →** Replay every tool call, command, and error, timestamped.
- **Token usage →** See what each run cost, per agent and per issue.
- **[Review gates](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/issues) →** Work lands in review, not in main. You decide what ships.
- **[Inbox](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/inbox) →** Get pinged when an agent needs a call, not for every step.
- **[Retries and timeouts](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/tasks#failures-and-automatic-retries) →** Failed runs retry on their own, or stop and tell you why.

## Make it yours.

*Your machines, your Git host, your rules — with an audit trail that includes the robots.*

- **[Self-host everything](SELF_HOSTING.md) →** Docker Compose or Helm, on your own infrastructure.
- **[Any Git host](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/vcs-integration) →** GitHub, GitLab, Gitea, or Forgejo — self-hosted included.
- **[Workspaces](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/workspaces) →** Separate agents, issues, and settings per team.
- **[Roles](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/members-roles) and [access scopes](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/agents#permissions-and-access) →** `owner`, `admin`, and `member` — and exactly which agents each member can run.
- **[Security model](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/security-model) →** What an agent can reach, and what it can't.
- **[Slack, Lark, DingTalk, WeCom, and Telegram](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/channels) →** Trigger and follow agent work where your team already talks. DingTalk, WeCom, and Telegram are [community-maintained](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/community-maintained).
- **[Web, desktop, and mobile](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/desktop-app) →** The same workspace on macOS, Windows, Linux, and iPhone — iOS builds from source today, not yet on the App Store.
- **[CLI and API](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/cli) →** Every surface is scriptable. Agents drive AI Gauntlet through the same CLI you do.

---

## Get started

No terminal required: sign up at **[ai-gauntlet.org](https://github.com/utkarshhhsinha/AI-Gauntlet)**, or download
**[AI Gauntlet Desktop](https://github.com/utkarshhhsinha/AI-Gauntlet/download)** for macOS, Windows, and Linux — it connects
the computer it runs on as a runtime automatically.

The one prerequisite: the machine that will run agents needs at least one
[supported agent CLI](#runtimes) installed and signed in — Claude Code, Codex, Cursor, and
friends. AI Gauntlet drives them; it doesn't ship them.

<details>
<summary><b>Self-hosting the whole thing</b></summary>

<br/>

```bash
curl -fsSL https://raw.githubusercontent.com/utkarshhhsinha/AI-Gauntlet/main/scripts/install.sh | bash -s -- --with-server
ai-gauntlet setup self-host
```

On Windows, set `$env:AI_GAUNTLET_MODE="with-server"`, then run the PowerShell installer:
`irm https://raw.githubusercontent.com/utkarshhhsinha/AI-Gauntlet/main/scripts/install.ps1 | iex`.

This pulls the official images from GHCR and requires Docker. See the
[Self-Hosting Guide](SELF_HOSTING.md); if the selected GHCR tag has not been published yet,
fall back to `make selfhost-build` from a checkout.

</details>

---

## Your first agent in five minutes

**1. Sign in.** [ai-gauntlet.org](https://github.com/utkarshhhsinha/AI-Gauntlet) in the browser, or open
[AI Gauntlet Desktop](https://github.com/utkarshhhsinha/AI-Gauntlet/download).

**2. Connect a computer.** A *runtime* is any machine agents can work on — your laptop, or a
cloud box. Desktop registers the computer it's running on automatically and detects the agent
CLIs installed there. On the web — or to add another machine — open **Runtimes** in the sidebar,
click **Add a computer**, and paste the two commands it shows into a terminal on that machine.

**3. Create an agent.** Open **Agents** in the sidebar and click **New agent**. Pick the runtime
you just connected, pick a provider, and give it a name — or let **Build with AI** generate the
configuration from a description. That name is how it shows up on the board and in comments.

**4. Assign it something.** File an issue and set the agent as assignee. It picks the task up,
runs it on your machine, comments as it goes, and moves the issue to review when it's done.

Full walkthrough: [Quickstart](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/cloud-quickstart) · [Tutorial](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/tutorial)

---

## Runtimes

AI Gauntlet does not ship a model. It drives the agent CLIs you already have installed and
authenticated, so switching providers is a dropdown, not a migration.

| Provider | CLI | Provider | CLI |
| --- | --- | --- | --- |
| Claude Code | `claude` | OpenAI Codex | `codex` |
| Cursor Agent | `cursor-agent` | GitHub Copilot CLI | `copilot` |
| OpenCode | `opencode` | OpenClaw | `openclaw` |
| Hermes | `hermes` | Pi | `pi` |
| Antigravity | `agy` | CodeBuddy | `codebuddy` |
| DevEco Code | `deveco` | Grok | `grok` |
| Kimi | `kimi` | Kiro CLI | `kiro-cli` |
| Qoder CLI | `qodercli` | Qoder CN | `qoderclicn` |
| Qwen Code | `qwen` | QwenPaw | `qwenpaw` |
| Reasonix | `reasonix` | Trae CLI | `traecli` |
| DeepSeek Harness | `dsh` | Oh-My-Pi | `omp` |
| MiniMax Code | `mcode` | Dim | `dim` |
| Huawei Cloud CodeArts | `codearts` | — | — |

Installing and authenticating them: [Install an agent runtime](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/install-agent-runtime) ·
[Providers](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/providers)

---

## Documentation

| I want to… | Start here |
| --- | --- |
| Get an agent doing something today | [Quickstart](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/cloud-quickstart) · [Tutorial](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/tutorial) |
| Understand how the pieces fit | [Core concepts](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/concepts) · [How AI Gauntlet works](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/how-ai-gauntlet-works) |
| Create and configure agents | [Agents](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/agents) · [Create an agent](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/agents-create) · [Skills](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/skills) |
| Get work to an agent | [Triggering agents](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/triggering-agents) · [Assigning issues](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/assigning-issues) · [Mentions](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/mentioning-agents) |
| Connect my machines | [Daemon and runtimes](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/daemon-runtimes) · [Install an agent runtime](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/install-agent-runtime) |
| Connect Git and chat tools | [GitHub](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/github-integration) · [Self-hosted Git](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/vcs-integration) · [Channels](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/channels) |
| Run it on my own infrastructure | [Self-hosting](SELF_HOSTING.md) · [Security model](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/security-model) · [Environment variables](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/environment-variables) |
| Script it | [CLI reference](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/cli) · [CLI and daemon guide](CLI_AND_DAEMON.md) · [Auth tokens](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/auth-tokens) |
| Drive AI Gauntlet from Codex, Claude Code, or Cursor | [AI Gauntlet CLI skill](https://github.com/utkarshhhsinha/AI-Gauntlet-cli) |
| Work out why an agent is stuck | [Tasks](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/tasks) · [Troubleshooting](https://github.com/utkarshhhsinha/AI-Gauntlet/docs/troubleshooting) |

---

## Architecture

```
        Web  ·  Desktop (macOS/Windows/Linux)  ·  iOS
                          │
                          ▼
   ┌──────────────┐   ┌──────────────┐   ┌──────────────────┐
   │   Next.js    │──>│  Go backend  │──>│   PostgreSQL     │
   │   frontend   │<──│  (Chi + WS)  │<──│   (17)           │
   └──────────────┘   └──────┬───────┘   └──────────────────┘
                             │  tasks over WebSocket
                      ┌──────┴───────┐
                      │ Agent daemon │  runs on your machine, next to your code
                      └──────┬───────┘
                             │  spawns
                      ┌──────┴───────────────────────────────┐
                      │  Claude Code · Codex · Cursor · …    │
                      │  (any of the 26 runtimes above)      │
                      └──────────────────────────────────────┘
```

| Layer | Stack |
| --- | --- |
| Web | Next.js 16 (App Router) |
| Desktop | Electron, sharing the web UI packages |
| Mobile | Expo / React Native (iOS) |
| Backend | Go (Chi router, sqlc, gorilla/websocket) |
| Database | PostgreSQL 17 (`pgcrypto` + `pg_trgm`) |
| Agent runtime | Local daemon executing any of the 26 agent CLIs above |

---

## Development

Contributors: start with the [Contributing Guide](CONTRIBUTING.md).

**Prerequisites:** [Node.js](https://nodejs.org/) 22, [pnpm](https://pnpm.io/) 10.28.2, [Go](https://go.dev/) 1.26.6, [Docker](https://www.docker.com/)

```bash
make dev
```

`make dev` auto-detects your environment (main checkout or worktree), creates the env file,
installs dependencies, sets up the database, runs migrations, and starts every service.

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full workflow, worktree support, testing, and
troubleshooting. The iOS client lives in [`apps/mobile/`](apps/mobile/) — its
[README](apps/mobile/README.md) covers building it onto your own iPhone.

We release most weekdays, so `main` moves quickly — pull often.

---

## Why "AI Gauntlet"?

**Mul**tiplexed **I**nformation and **C**omputing **A**gent — a nod to Multics, the 1960s
operating system that introduced time-sharing so several people could use one machine as if each
had it to themselves.

Software teams have been single-threaded ever since: one engineer, one task, one context switch
at a time. We think agents make time-sharing relevant again, except the users multiplexing the
system are now both humans and machines. A small team shouldn't feel small.

The longer argument, and where we think this goes: **[VISION.md](VISION.md)**.

---

## License

[AI Gauntlet License](LICENSE) — the complete Apache License 2.0 text plus additional conditions
covering hosted services, commercial embedding, and branding. Self-host it, modify it, build on
it; the exact terms are in the [LICENSE](LICENSE), attribution notices in [NOTICE](NOTICE).

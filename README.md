<div align="center">

# 🧠 Kernos OS

**The Cognitive Microkernel & Autonomous OS**

[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![React](https://img.shields.io/badge/React-19-61DAFB?style=flat-square&logo=react&logoColor=black)](https://react.dev)
[![License](https://img.shields.io/badge/License-Apache_2.0-orange?style=flat-square)](LICENSE)

*The world's first operating system modeled after biological intelligence.*

</div>

<br />

<div align="center">
  <video src="kernos_linkedin_demo.mp4" width="100%" autoplay loop muted playsinline controls></video>
</div>

<br />

---

## 🧬 The "Unconventional Trinity" Architecture

Kernos OS discards traditional Unix and Windows paradigms. Instead of treating Artificial Intelligence as an external application, Kernos wires cognitive routing directly into the Kernel Space using the **Unconventional Trinity**:

### 1. The Synaptic Vector Graph
A traditional OS organizes memory via a hierarchical file system tree. Kernos OS replaces this with a mathematical **Synaptic Vector Graph**. Every keystroke in the terminal, every code file, and every config change is mapped into a localized Nomic 768-dimensional latent space. You do not search by filename; the kernel retrieves data by *conceptual resonance*, allowing the OS to fetch relevant memories before you even ask.

### 2. Hallucinatory RAG (Retrieval-Augmented Generation)
Kernos thrives on deliberate, controlled hallucination. The "Dispatcher" agent constantly hallucinates potential futures based on your current context. This leads to **Speculative Execution**: the OS forecasts your next 3 terminal commands or code edits, silently pre-executing them in a shadow jail. When you actually type the command, the result returns in 0ms.

### 3. Mutating DAGs (Directed Acyclic Graphs)
Traditional programs run sequentially. Kernos executes tasks via biology-inspired **Mutating DAGs**. The OS generates a dependency graph to accomplish an objective. But as nodes execute and conditions change, the "Architect" agent *mutates the graph in real-time*, rewriting its own dependencies, spawning new parallel nodes, and pruning dead branches to survive volatile software states.

---

## ✨ Features Beyond Next-Gen

| Aspect | Implementation |
|---|---|
| 🤖 **Autonomous Workspaces** | 4 specialized AI agents (Architect, DevOps, Security, Code Review) that debate and write code concurrently within the same window. |
| 🛡️ **Zero-Trust Applets** | Compile React components natively into the WebAssembly/Shadow DOM container by single-clicking them from the UI. |
| 🌐 **P2P Synapse Routing** | Two Kernos instances can bridge their message buses via WebRTC with a 4-digit PIN, seamlessly sharing cognition and files. |
| ⏪ **Temporal Branching** | Every system state mutation and command is logged. The OS features a Timeline Slider to revert to the exact millisecond before a disastrous config change. |
| 🧠 **Nightly Consolidation** | Like human sleep, the kernel performs "RLHF Pruning" overnight, summarizing its past mistakes into permanent behavioral weights to mathematically improve its intelligence. |

---

## 🚀 The Single-Binary Installation

The entire Operating System — the Go microkernel, the React graphical environment, the Nomic vector database, and the Websocket Bus — is statically compiled into a single **19 MB executable.** It has zero external dependencies.

### Standard Boot

```bash
git clone https://github.com/GI-Company/kernos-os.git
cd kernos-os

# Install react deps once
npm install

# Start the full OS environment
make dev
```

Open **http://localhost:3000** in your browser.

> **💡 Cognitive Mode:** To awaken the OS, connect it to a local LLM via [LM Studio](https://lmstudio.ai/):
> ```bash
> go run ./server -lm-url http://localhost:1234/v1/chat/completions
> ```

---

## 🏗️ Deep Microkernel Architecture

Kernos communicates exclusively via typed **Envelope** messages across a high-performance publish-subscribe bus.

```typescript
// The fundamental atomic unit of the OS
interface Envelope {
  topic: string;     // e.g., "vm.spawn", "synapse.mutate", "ai.predict"
  from: string;      // The sender capability hash
  to?: string;       // Direct memory routing
  payload: any;      // The dynamic payload
  time: string;      // Temporal anchor
}
```

By decoupling the UI from the execution layer via the WebSocket Envelope protocol, the OS enforces perfect visual-sandbox isolation.

---

## 📚 Technical & Commercial Documentation

- [The Kernos OS Whitepaper](./KERNOS_OS_WHITEPAPER.md) - Deep dive into Kernel architecture and cognitive systems.
- [The Formal Valuation Paper](./KERNOS_OS_VALUATION.md) - Market analysis, enterprise viability, and intellectual property value.

---

## 📜 License

Apache 2.0 — Open Source Cognitive Computing.

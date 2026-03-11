<div align="center">

# 🧠 Kernos OS

**The Cognitive Microkernel & Autonomous OS**

[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![React](https://img.shields.io/badge/React-19-61DAFB?style=flat-square&logo=react&logoColor=black)](https://react.dev)
[![License](https://img.shields.io/badge/License-Apache_2.0-orange?style=flat-square)](LICENSE)

*The world's first operating system modeled after biological intelligence.*

> **Why "Kernos"?**  
> In ancient Greek antiquity, a *kernos* was a unique pottery vessel featuring a central base with multiple distinct, isolated cups attached to it. It perfectly represents this OS's architecture: a central, high-speed Go **microkernel** acting as the foundation, holding multiple isolated, autonomous AI **agents** and sandboxes that operate together as a single cognitive entity.

</div>

<br />

<div align="center">
  <video src="kernos_linkedin_demo.mp4" width="100%" autoplay loop muted playsinline controls></video>
</div>

<br />

---

## 🧬 The "Unconventional Trinity" Architecture

Kernos OS discards traditional Unix and Windows paradigms. Instead of treating Artificial Intelligence as an external application, Kernos wires cognitive routing directly into the Kernel Space:

### 1. Vector Graph Memory 
A traditional OS organizes memory via a hierarchical file system tree. Kernos OS replaces this with a mathematical **Vector Graph**. Keystrokes, file contents, and system outputs are mapped into a localized Nomic 768-dimensional latent space. The kernel retrieves contextual operating data by semantic resonance, rather than exact path matching.

### 2. Speculative RAG Execution
Kernos utilizes the predictive nature of Local LLMs for **Speculative Execution**. The OS evaluates partial terminal input, predicts the most likely complete command, and silently pre-executes it in an invisible sandbox jail. When the user eventually submits the command, the OS yields the pre-computed `stdout` with perceived zero-latency.

### 3. Concurrent DAG Mutation 
Traditional shell scripts run sequentially and fail abruptly. Kernos executes multi-step objectives using **Directed Acyclic Graphs (DAGs)**. If a node fails during execution, the "Architect" agent synthesizes multiple, divergent recovery paths. The engine executes these alternative branches in parallel; the first to exit successfully collapses the state and is grafted into the DAG, allowing execution to autonomously proceed.

---

## ✨ Integrated Subsystems

| Aspect | Implementation |
|---|---|
| 🤖 **Autonomous Workspaces** | Specialized AI agents (Architect, DevOps, Security, Code Review) dynamically orchestrate tasks within the React environment. |
| 🛡️ **Zero-Trust Applets** | Compile React components natively into the WebAssembly/Shadow DOM container using embedded `esbuild` for strict isolation. |
| 🌐 **WebRTC Subnet Routing** | Two Kernos instances can bridge their message buses via WebRTC, establishing peer-to-peer data channels for distributed execution. |
| ⏪ **Temporal Branching** | Every system state mutation and command is logged to an immutable timeline, enabling rollback capabilities. |
| 🧠 **Contrastive RLHF** | The kernel performs nightly consolidation, contrasting high-reward execution paths against failures to continually update system prompt weights. |

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

## 📚 Technical & Academic Documentation

- [The Kernos OS Research Paper](./KERNOS_OS_RESEARCH_PAPER.md) - Formal academic exploration of the Cognitive Microkernel framework.
- [The Kernos OS Whitepaper](./KERNOS_OS_WHITEPAPER.md) - Deep dive into Kernel architecture and intelligent sub-systems.
- [The Commercial Analysis](./KERNOS_OS_VALUATION.md) - Market viability and technical implementation review.

---

## 📜 License

Apache 2.0 — Open Source Cognitive Computing.

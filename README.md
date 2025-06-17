<div align='center'>

# PoPpOloPOpPo Build System (PPB)

**A distributed, cache-aware, parallel build system for C++ and more, written in Go.**

[![Go](https://github.com/poppolopoppo/ppb/actions/workflows/go.yml/badge.svg)](https://github.com/poppolopoppo/ppb/actions/workflows/go.yml)
[![CodeQL](https://github.com/poppolopoppo/ppb/actions/workflows/github-code-scanning/codeql/badge.svg)](https://github.com/poppolopoppo/ppb/actions/workflows/github-code-scanning/codeql)

</div>

---

## :notebook_with_decorative_cover: Table of Contents

- [About](#about)
- [Architecture](#architecture)
- [Features](#features)
- [Source Control Integration](#source-control-integration)
- [Usage](#usage)
- [Configuration](#configuration)
- [Development History](#development-history)
- [Dependencies](#dependencies)
- [Contributing](#contributing)
- [License](#license)

---

## About

**PPB** is a next-generation build system designed for high-performance, distributed, and cache-efficient builds. It was originally forked from the [PoPpOloPOpPo Engine](https://github.com/poppolopoppo/ppe) and has evolved into a standalone, language-agnostic build orchestrator, with a strong focus on C++ but extensible to other languages and asset pipelines.

PPB is written in Go and leverages modern concurrency, distributed systems, and IO interception techniques to provide:

- **Transparent build artifact tracking** (via IO detouring, e.g., DLL injection on Windows)
- **Deterministic and reproducible builds**
- **Distributed build execution** across a peer-to-peer cluster
- **Aggressive caching** of compilation and linking results

---

## Architecture

PPB is structured as a modular, extensible build graph engine. Key architectural components include:

- **Build Graph Core:**
  Models build actions, dependencies, and artifacts as a directed acyclic graph (DAG). Supports incremental and minimal rebuilds by fingerprinting buildable nodes and their inputs.

- **Action Model:**
  Each build step is an `Action` with explicit inputs, outputs, and rules. Actions can be local or distributed, and are tracked for cacheability and reproducibility.

- **Distributed Cluster:**
  Implements a decentralized peer-to-peer cluster using [QUIC](https://en.wikipedia.org/wiki/QUIC) for secure, low-latency communication. Each worker node advertises its resources (CPU, memory, idle state) and can accept jobs from others.

- **IO Detouring:**
  On Windows, uses DLL injection to intercept file IO, enabling transparent tracking of all files read/written by build actions, without requiring compiler support or source code changes.

- **Caching Layer:**
  Build outputs are fingerprinted and cached using fast hash algorithms and compressed with [zstd](https://github.com/facebook/zstd) or [lz4](https://github.com/lz4/lz4). Cache hits avoid redundant work both locally and across the cluster.

- **Source Control Integration:**
  Integrates with Git to track source file status, branch, and revision, and to optimize incremental builds.

- **Extensible Toolchain Support:**
  Supports multiple compilers (MSVC, Clang, GCC) and can be extended to other languages and asset pipelines.

---

## Features

- **Data-driven build graph:**
  Modules and actions are described in JSON, supporting private, public, and runtime dependencies.

- **Automatic dependency tracking:**
  IO detouring and source control integration ensure all relevant files are tracked for correctness and minimal rebuilds.

- **Distributed builds:**
  Peer-to-peer cluster with decentralized scheduling and resource balancing.

- **Build caching:**
  Deterministic fingerprints for all build actions; cache is compressed and shared across the cluster.

- **Precompiled header (PCH) and C++20 header unit support:**
  Enables caching and distribution of expensive header builds.

- **Compile database and IDE integration:**
  Generates [compile_commands.json](https://clangd.llvm.org/design/compile-commands) and VS Code workspace files for code navigation and tooling.

- **Resource-aware scheduling:**
  Workers advertise and allocate CPU/memory resources dynamically; jobs are distributed accordingly.

- **WebDAV integration:**
  Remote workers can access local filesystems via [webdav](https://pkg.go.dev/golang.org/x/net/webdav).

- **Comprehensive logging and statistics:**
  Detailed logs, build summaries, and critical path analysis.

- **Cross-platform:**
  Runs on Windows, Linux, and macOS (with platform-specific IO tracking).

---

## Source Control Integration

- **Git-aware:**
  Detects modified, added, deleted, and untracked files.
- **Build graph nodes for source control state:**
  Enables commands like `list-modified-files`, `list-artifacts`, and more.
- **Automatic branch and revision tracking:**
  Used for build reproducibility and cache keying.

---

## Usage

### Build the tool

```sh
git clone https://github.com/poppolopoppo/ppb.git
cd ppb/Build
go build
```

### Basic commands

- **Build a project:**
  ```sh
  ./Build [options]
  ```
- **Generate VS Code workspace:**
  ```sh
  ./Build vscode
  ```
- **Debug the build graph:**
  ```sh
  ./Build debug
  ```
- **List all known artifacts:**
  ```sh
  ./Build list-artifacts
  ```
- **List modified files (from source control):**
  ```sh
  ./Build list-modified-files
  ```

Run j for a full list of commands and options.

---

## Configuration

- **Module and action definitions:**
  Place JSON files describing modules and their dependencies in your project, see [compile/Model.go](compile/Model.go).
- **Compiler/toolchain selection:**
  Configurable via JSON and command-line flags, see [compile/compiler.go](compile/compiler.go).
- **Cluster configuration:**
  Workers auto-discover each other via QUIC; resource limits can be set per worker, see [cluster/cluster.go](cluster/cluster.go)..

---

## Development History

- **Forked from PPE:**
  Initial codebase derived from the PoPpOloPOpPo Engine, with a focus on C++.
- **Transition to Go:**
  Rewritten core in Go for concurrency, maintainability, and cross-platform support.
- **IO Detouring:**
  Added DLL-based IO interception for transparent artifact tracking (Windows).
- **Distributed Build and Caching:**
  Implemented decentralized cluster and cache sharing.
- **Source Control Integration:**
  Added Git support for smarter incremental builds.
- **IDE and Tooling Integration:**
  Added compile database and VS Code workspace generation.
- **Ongoing:**
  Refactoring for language/toolchain agnosticism, improved test/distribution setup, and more robust cross-platform support.

---

## Contributing

Contributions are welcome!
Please open issues or pull requests on [GitHub](https://github.com/poppolopoppo/ppb).

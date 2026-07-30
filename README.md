<div align='center'>

# PoPpOloPOpPo Build System (PPB) 🛠️

**A distributed, cache-aware, parallel build system for C++ and more, written in Go.**

[![Go Build](https://github.com/poppolopoppo/ppb/actions/workflows/go.yml/badge.svg)](https://github.com/poppolopoppo/ppb/actions/workflows/go.yml)
[![CodeQL](https://github.com/poppolopoppo/ppb/actions/workflows/codeql.yml/badge.svg)](https://github.com/poppolopoppo/ppb/actions/workflows/codeql.yml)
[![Codacy](https://github.com/poppolopoppo/ppb/actions/workflows/codacy.yml/badge.svg)](https://github.com/poppolopoppo/ppb/actions/workflows/codacy.yml)
[![Go Version](https://img.shields.io/badge/Go-1.25-blue?logo=go)](https://go.dev)

</div>

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

## Prerequisites

- **Go 1.25+** — required for building PPB from source
- **C++ compiler** — MSVC, Clang/ClangCL (Windows), or GCC (Linux/macOS) for C++ toolchain compilation
- **Linux system deps** — `libglfw3-dev` (for Linux builds with GUI support):
  ```sh
  sudo apt-get update && sudo apt-get install -y libglfw3-dev
  ```
- **Network access** — for distributed builds, peers must be able to reach each other over QUIC (UDP port 9085)

---

## Build Outputs

When you run `./ppb build`, PPB produces:

- **Compiled binaries** — native executables and libraries in the build output directory
- **compile_commands.json** — Clang-compatible compilation database (with `-compile-db` flag)
- **Build cache** — fingerprinted and compressed artifacts in the cache directory (zstd/lz4)
- **VS Code workspace** — `.vscode/` files for IDE navigation (with `vscode` command)

---

## Docker

PPB includes a `Dockerfile` for building and running the `ppb_worker` in a container using a multi-stage build:

```dockerfile
# Stage 1: Build the worker binary with Go
FROM golang:1.25 AS builder
WORKDIR /app
COPY . .
RUN go mod download && GOOS=windows GOARCH=amd64 go build -o ppb_worker.exe ./cmd/ppb_worker

# Stage 2: Minimal runtime image
FROM mcr.microsoft.com/windows/servercore:ltsc2022
WORKDIR /app
COPY --from=builder /app/ppb_worker.exe .
EXPOSE 9085
ENTRYPOINT ["C:\\app\\ppb_worker.exe"]
```

To build and run the worker container:

```sh
# Build the Docker image
docker build -t ppb-worker .

# Run the worker (ensure network access for cluster communication)
docker run --rm -it --network host ppb-worker
```

---

## Source Control Integration

- **Git-aware:** Detects modified, added, deleted, and untracked files.
- **Build graph nodes for source control state:** Enables commands like `list-modified-files`, `list-artifacts`, and more.
- **Automatic branch and revision tracking:** Used for build reproducibility and cache keying.

---

## Configuration

- **Module and action definitions:** Place JSON files describing modules and their dependencies in your project, see [compile/Model.go](compile/Model.go).
- **Compiler/toolchain selection:** Configurable via JSON and command-line flags, see [compile/Compiler.go](compile/Compiler.go).
- **Cluster configuration:** Workers auto-discover each other via QUIC; resource limits can be set per worker, see [cluster/cluster.go](cluster/cluster.go).

---

## C++ Toolchain Customization Options

You can customize the following rules per-project or per-archetype to control the C++ toolchain behavior:

### Warning Levels (`Warnings`)

| Field            | Description                                         |
|------------------|-----------------------------------------------------|
| `Default`        | Base warning level for the compiler (e.g., OFF, DEFAULT, HIGH) |
| `Deprecation`    | Controls warnings for deprecated features           |
| `Pedantic`       | Controls warnings for strict standard conformance   |
| `ShadowVariable` | Controls warnings for variable shadowing             |
| `UndefinedMacro` | Controls warnings for use of undefined macros        |
| `UnsafeTypeCast` | Controls warnings for unsafe type casts and promotions |

---

### Build and Compilation Rules

| Field            | Description                                         |
|------------------|-----------------------------------------------------|
| `SizePerUnity`   | Size (in bytes) of each unity file for adaptive unity builds |
| `Instructions`   | Instruction sets to enable (e.g., SSE, AVX2, AVX512) |
| `CppStd`         | The C++ standard to use (e.g., C++11, C++14, C++17, C++20) |
| `CppRtti`        | Enable or disable Run-Time Type Information (RTTI)  |
| `DebugInfo`      | Level of debug information to generate (OFF, DEFAULT, FULL) |
| `Exceptions`     | Enable or disable C++ exceptions                     |
| `FloatingPoint`  | Floating point model (e.g., precise, fast, strict)  |
| `Link`           | Linking options (e.g., static, dynamic)              |
| `Optimize`       | Optimization level (e.g., none, size, speed, full)   |
| `PCH`            | Precompiled header options                           |
| `RuntimeLib`     | Runtime library selection (e.g., static, dynamic, debug, release) |
| `Sanitizer`      | Enable runtime sanitizers (e.g., address, thread, undefined behavior) |
| `Unity`          | Unity build options (enabled, disabled)              |

---

### Boolean Feature Toggles

| Field             | Description                                         |
|-------------------|-----------------------------------------------------|
| `AdaptiveUnity`   | Enable adaptive unity builds                        |
| `Benchmark`       | Enable benchmarking features                        |
| `Deterministic`   | Enable deterministic (reproducible) builds          |
| `DebugFastLink`   | Enable fast linking for debug builds                |
| `Incremental`     | Enable incremental builds                            |
| `LTO`             | Enable Link-Time Optimization                        |
| `RuntimeChecks`   | Enable runtime checks (e.g., stack protection)      |
| `CompilerVerbose` | Enable verbose output from the compiler              |
| `LinkerVerbose`   | Enable verbose output from the linker                |

---

### Example JSON Configuration

```json
{
    "CppStd": "C++20",
    "Optimize": "FULL",
    "DebugInfo": "FULL",
    "Exceptions": "ON",
    "FloatingPoint": "PRECISE",
    "Instructions": ["AVX2", "SSE4.2"],
    "Warnings": {
        "Default": "HIGH",
        "Deprecation": "DEFAULT",
        "Pedantic": "HIGH",
        "ShadowVariable": "HIGH",
        "UndefinedMacro": "DEFAULT",
        "UnsafeTypeCast": "HIGH"
    },
    "LTO": true,
    "Deterministic": true,
    "Incremental": true,
    "CompilerVerbose": false,
    "LinkerVerbose": false
}
```

---

**How to Use:**
- These rules can be set globally, per-project, or per-archetype (template).
- They control the generated compiler and linker flags for each build.
- Boolean toggles (`true`/`false`) enable or disable features.
- Enum/string options select the desired mode or level for each rule.

> **Tip:** Refer to your build system's documentation or schema for the exact allowed values for each field (e.g., which C++ standards, optimization levels, or sanitizer types are supported).

---

## Usage

### 🚀 Quick Start

```sh
git clone https://github.com/poppolopoppo/ppb.git
cd ppb
go build
./ppb configure
./ppb build
```

> **Tip:** For distributed builds, start additional workers with `./ppb worker` on other machines in your network.

### 🧑‍💻 Example Usage

```sh
# Parse json module files and bootstrap the build graph (this is only needed the first time)
./ppb configure [options]

# Build all targets with verbose output and summary
./ppb build -v -Summary

# List all known build artifacts
./ppb list-artifacts

# Generate VS Code workspace
./ppb vscode

# List modified files from source control
./ppb list-modified-files

# Print all available commands and options with detailed descriptions
./ppb help -v

# Show help for a specific command
./ppb help list-artifacts
```

### 📋 Available Commands

Below is a list of the main commands. For each, you can run `./ppb help <command>` for detailed usage.

| Command                | Description                                                      |
|------------------------|------------------------------------------------------------------|
| `help`                 | Print help about command usage                                   |
| `autocomplete`         | Run auto-completion for commands and arguments                   |
| `version`              | Print build version                                              |
| `seed`                 | Print build seed                                                 |
| `worker`               | Start a distributed build worker node                            |
| `configure`            | Parse module JSON files and bootstrap the build graph            |
| `build`                | Build all targets                                                |
| `vscode`               | Generate workspace for Visual Studio Code                        |
| `vcxproj`              | Generate projects and solution for Visual Studio                 |
| `debug`                | Debug the build graph                                            |
| `list-artifacts`       | List all known build artifacts                                   |
| `list-modified-files`  | List modified files from source control                          |
| `list-source-files`    | List all known source files                                      |
| `list-generated-files` | List all known generated files                                   |
| `list-namespaces`      | List all available namespaces                                    |
| `list-environments`    | List all compilation environments                                |
| `list-targets`         | List all translated targets                                      |
| `list-programs`        | List all executable targets                                      |
| `list-persistents`     | List all persistent data                                         |
| `list-commands`        | List all available commands                                      |
| `list-platforms`       | List all available platforms                                     |
| `list-configs`         | List all available configurations                                |
| `list-compilers`       | List all available compilers                                     |
| `list-modules`         | List all available modules                                       |
| `check-build`          | Build graph aliases passed as input parameters                   |
| `check-fingerprint`    | Recompute nodes fingerprint and compare with stored stamp        |
| `import-action`        | Import actions from external JSON file(s)                        |
| `export-action`        | Export selected compilation actions to JSON                      |
| `dist-clean`           | Clean distributed build artifacts across the cluster             |
| `run`                  | Build and run a specific target                                  |

> **Tip:** Many commands accept additional arguments or flags. Use `./ppb help <command>` for details.

---

**Note:**
- You can chain multiple commands using `-and`, e.g. `./ppb configure -and vscode -and build -Summary`
- All commands and flags are case-sensitive.
- Run `make help` for a complete list of build targets and development commands.

---

### 🛠️ Command-Line Options

The following **global flags** can be used with any command:

| Flag             | Description                                                                                  |
|------------------|----------------------------------------------------------------------------------------------|
| `-f`             | Force build even if up-to-date                                                               |
| `-F`             | Force build and ignore cache                                                                 |
| `-j`             | Override number of worker threads (default: numCpu-1)                                        |
| `-q`             | Disable all messages                                                                         |
| `-v`             | Turn on verbose mode                                                                         |
| `-t`             | Print more information about progress                                                        |
| `-V`             | Turn on very verbose mode                                                                    |
| `-d`             | Turn on debug assertions and more log (only if built with debug enabled)                     |
| `-T`             | Turn on timestamp logging                                                                    |
| `-X`             | Turn on diagnostics mode                                                                     |
| `-Color`         | Control ANSI color output in log messages                                                    |
| `-Ide`           | Set output to IDE mode (disable interactive shell)                                           |
| `-LogAll`        | Output all messages for given log categories                                                 |
| `-LogMute`       | Mute all messages for given log categories                                                   |
| `-LogImmediate`  | Disable buffering of log messages                                                            |
| `-LogFile`       | Output log to specified file (default: stdout)                                               |
| `-OutputDir`     | Override default output directory                                                            |
| `-RootDir`       | Override root directory                                                                      |
| `-StopOnError`   | Interrupt build process immediately when an error occurred                                   |
| `-Summary`       | Print build graph execution summary when build finished                                      |
| `-WX`            | Consider warnings as errors                                                                  |
| `-EX`            | Consider errors as panics                                                                    |

---

## Distributed Builds

PPB supports peer-to-peer distributed builds. To use distributed builds:

1. **Start a worker** on each machine that should participate in the cluster:
   ```sh
   # On each worker machine
   ./ppb worker -port=9085
   ```
2. **Start the coordinator** (or use any node as coordinator):
   ```sh
   ./ppb build -v -Summary
   ```
3. Workers auto-discover each other via QUIC. The cluster automatically balances workloads based on available CPU and memory.

---

## Troubleshooting

### `go build` fails on Windows with DataDog/zstd errors

The `DataDog/zstd` dependency requires CGO and a C compiler on Windows. If you're building on Windows without a C compiler:

- Install [MinGW-w64](https://www.mingw-w64.org/) and add it to your `PATH`
- Or build on Linux/macOS where CGO works by default
- For CI (Linux runners), this is not an issue

### Worker can't connect to other peers

- Ensure UDP port 9085 is open in your firewall
- Verify workers are on the same network or VPN
- Check that QUIC is not blocked by your network infrastructure

### Cache is not working as expected

- Fingerprint mismatches between builds will cause cache misses
- Ensure `Deterministic` is enabled in your JSON configuration for reproducible cache keys
- Clear the cache with `./ppb dist-clean` if needed

---

## Security

PPB uses CodeQL and Codacy for automated security scanning on every push and pull request. Key security practices:

- **Dependency updates** are managed through [Dependabot](https://github.com/poppolopoppo/ppb/settings/dependabot) with weekly checks
- **Code scanning** via CodeQL (`security-extended` and `security-and-quality` query packs)
- **Secret scanning** is enabled for the repository
- **Vulnerability reporting:** Please report security issues privately via GitHub's security advisory feature or by contacting the maintainers

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

We welcome contributions! Please follow these guidelines:

### Prerequisites
- Go 1.25+ installed
- C++ compiler for your platform (MSVC, Clang, or GCC)

### Setup
```sh
git clone https://github.com/poppolopoppo/ppb.git
cd ppb
go mod download
go build
```

### Running Tests
```sh
go test -v ./...
```

### Code Style
- Run `gofmt -w .` before committing to ensure consistent formatting
- Run `go vet ./...` to check for issues
- Use `// #nosec` comments to suppress gosec findings that are false positives

### Submitting Changes
1. Fork the repository and create a feature branch
2. Make your changes with appropriate tests
3. Ensure all checks pass (`go build`, `go vet`, `go test`)
4. Open a pull request with a clear description of the change

### Reporting Issues
Please open issues on [GitHub Issues](https://github.com/poppolopoppo/ppb/issues) with:
- A clear description of the problem
- Steps to reproduce
- Your environment (OS, Go version, PPB version)
- Any relevant logs or error messages

---

## License

See the project license for details.
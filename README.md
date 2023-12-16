<div align='center'>

<h1>PoPpOloPOpPo Build System</h1>
<p>Build executor with caching and distribution without relying on compiler support thanks to IO detouring</p>

<h4> <span> · </span> <a href="https://github.com/poppolopoppo/ppb/blob/master/README.md"> Documentation </a> <span> · </span> <a href="https://github.com/poppolopoppo/ppb/issues"> Report Bug </a> <span> · </span> <a href="https://github.com/poppolopoppo/ppb/issues"> Request Feature </a> </h4>

</div>

# :notebook_with_decorative_cover: Table of Contents

- [About the Project](#suspect-about-the-project)
- [Features](#dart-features)
- [Todo](#soon-dependencies)
- [Dependencies](#heartpulse-dependencies)

## :star2: About the Project

PPB is a parallelised build system written in Go.
It supports caching of compiled artifacts, as well as tasks distribution in a cluster of workers over network.
Caching and distribution both rely on [IO detouring using DLL injection on Windows](https://github.com/microsoft/Detours/blob/main/samples/tracebld/tracebld.cpp).

This is an initial release forked from [PoPploPopPo Engine](https://github.com/poppolopoppo/ppe), and code still needs to be cleaned out of some old assumptions specific to [PPE](https://github.com/poppolopoppo/ppe/).
Sources for IO detouring DLL library are still hosted on the [original repository](https://github.com/poppolopoppo/ppe/tree/master/Source/Tools) and you will only find prebuilt binaries for Windows here.

The current version of the toolchain is stronlgy assuming C++ is the main language, but the goal is to provide a language/tool agnostic toolchain supporting painless caching and distribution (could run a DCC to generate assets for instance).

## :dart: Features
- Data-driven C++ module declaration using JSON
- Modules can have private, public or runtime dependencies
- Generate [compile_commands.json](https://clangd.llvm.org/design/compile-commands) and Visual Studio Code workspace
- Build execution is parallelized and dependencies are tracked to guarantee mininal rebuilds
- Supports compilation and link caching with deterministic builds
- Cached compilation results are compressed using [facebook zstd](https://github.com/facebook/zstd) or [lz4](https://github.com/lz4/lz4)
- Actions can be distributed over a cluster of workers using [google QUIC](https://en.wikipedia.org/wiki/QUIC) peer-to-peer protocol
- Distribution is decentralized and workload is balanced according to resources available on peers
- Remote workers can directly access local filesystem using a [webdav server](https://pkg.go.dev/golang.org/x/net/webdav) and IO detouring
- Resources allocated to job distribution can be customized (work idle, N threads, max memory usage)

## :soon: Todo

- [ ] Add sources for IOWrapper & IODetouring helpers (only prebuilt binaries)
- [ ] Find a better setup for testing distribution (current DockerFile on Windows is very limiting)
- [ ] Provide a way to import actions without describing modules with data-driver layer
- [x] Implement Visual Studio solution and project generator
- [ ] Test distribution and caching with [UnrealBuildTool](https://docs.unrealengine.com/5.3/en-US/unreal-build-tool-in-unreal-engine/)

## :heartpulse: Dependencies
- [github.com/DataDog/zstd](github.com/DataDog/zstd)
- [github.com/Showmax/go-fqdn](github.com/Showmax/go-fqdn)
- [github.com/djherbis/times](github.com/djherbis/times)
- [github.com/goccy/go-json](github.com/goccy/go-json)
- [github.com/klauspost/compress](github.com/klauspost/compress)
- [github.com/mholt/archiver/v3](github.com/mholt/archiver/v3)
- [github.com/minio/sha256-simd](github.com/minio/sha256-simd)
- [github.com/pierrec/lz4/v4](github.com/pierrec/lz4/v4)
- [github.com/pkg/profile](github.com/pkg/profile)
- [github.com/quic-go/quic-go](github.com/quic-go/quic-go)
- [github.com/shirou/gopsutil](github.com/shirou/gopsutil)
- [golang.org/x/exp](golang.org/x/exp)
- [golang.org/x/net](golang.org/x/net)
- [golang.org/x/sys](golang.org/x/sys)
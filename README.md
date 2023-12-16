<div align='center'>

<h1>PoPpOloPOpPo Build System</h1>
<p>Build executor support caching and distribution in a compiler-agnostic way thanks to IO detouring</p>

<h4> <span> · </span> <a href="https://github.com/poppolopoppo/ppb/blob/master/README.md"> Documentation </a> <span> · </span> <a href="https://github.com/poppolopoppo/ppb/issues"> Report Bug </a> <span> · </span> <a href="https://github.com/poppolopoppo/ppb/issues"> Request Feature </a> </h4>

</div>

# :notebook_with_decorative_cover: Table of Contents

- [About the Project](#star2-about-the-project)
- [Current state](#calendar-current-state)

## :star2: About the Project

Parallelised build system written in Go.<br/>
Support caching of compiled artifacts as well as tasks distribution in a cluster of workers over network.<br/>
Caching and distribution both rely on [IO detouring using DLL injection on Windows](https://github.com/microsoft/Detours/blob/main/samples/tracebld/tracebld.cpp).

### :dart: Features
- Data-driven C++ module declaration using JSON
- Modules can have private, public or runtime dependencies
- Build execution is parallelized and dependencies are track for each action to guarantee mininal rebuilds
- Supports compilation and link caching with deterministic builds
- Cached compilation results are store compressed with their dependencies with automatic invalidation
- Actions can be distributed over a cluster of worker using a secure peer-to-peer protocol based on quic
- Distribution is decentralized and workload is balanced according to resources available on peers
- Resources allocated to job distribution on peers can be customnized (work idle, N threads, max memory usage)
- Workers can directly access host filesystem thanks to a webdav server and IO detouring

## :calendar: Current state
This is an initial release forked from [PoPploPopPo Engine](https://github.com/poppolopoppo/ppe), and code still needs to be cleaned out of some assumptions.
Sources for IO detouring DLL library are still hosted on the original repository and this repository only contains a prebuilt version for Windows.
The current version of the toolchain is stronlgy assuming C++ is the main language, but the goal is to provide a language agnostic toolchain supporting painless caching and distribution.

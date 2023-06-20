# ppb
_PoPploPopPo Build System_

Parallelised build system written in Go.<br/>
Support caching of compiled artifacts as well as tasks distribution in a cluster of workers over network.<br/>
Caching and distribution both rely on [IO detouring using DLL injection on Windows](https://github.com/microsoft/Detours/blob/main/samples/tracebld/tracebld.cpp).

## Notes about current state
This is an initial release forked from [PoPploPopPo Engine](https://github.com/poppolopoppo/ppe), and code still needs to be cleaned out of some assumptions.
Sources for IO detouring DLL library are still hosted on the original repository and this repository only contains a prebuilt version for Windows.
The current version of the toolchain is stronlgy assuming C++ is the main language, but the goal is to provide a language agnostic toolchain supporting painless caching and distribution.

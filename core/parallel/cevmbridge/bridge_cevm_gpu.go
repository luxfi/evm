// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build cgo && cevm && cevmgpu

package cevmbridge

// GPU-linked overlay for the cevm bridge (FRONT 1 of the parallel GPU effort).
//
// This file is ADDITIVE to bridge_cevm.go. Under `-tags "cevm cevmgpu"` BOTH
// files compile, so cgo concatenates their `#cgo LDFLAGS`: bridge_cevm.go
// contributes the CPU state libs (build-mpt — libevm-state.a + libevm dylib +
// cevm_precompiles + conan crypto), and this file adds the GPU host archives
// (evm-gpu / evm-metal-hosts / evm-kernel-metal / evm-gpu-state) plus the
// Apple Metal + Foundation frameworks. The default `-tags cevm` build is
// UNCHANGED and stays CPU-only — this file is gated behind the extra `cevmgpu`
// tag, so it never affects the CPU-only bridge.
//
// The GPU archives are the build-mpt-gpu Metal-ON build (LUX_CEVM_ENABLE_METAL
// =ON) — CO-BUILT from the SAME current cevm source tree AND the SAME Conan
// deps as build-mpt's state libs (build-mpt-gpu references build-mpt's own
// generators/conan_toolchain.cmake, so crypto/bls/intx/zap-core/nlohmann_json
// are byte-identical; the GPU backend luxgpu_core_static is compiled from the
// in-tree luxcpp/gpu at project version 0.5.2 — the same version build-mpt
// reports). This CLOSES the prior ABI-layout risk: the earlier linkage used
// build-phase5b (Apr/May, project version 0.2.0) archives against Jul
// build-mpt (0.5.2) state libs — nm-clean but era-skewed. The two are now the
// same era by construction. A link-level symbol audit (nm) confirms every
// evm:: core/state/precompile symbol the GPU archives reference is satisfied
// by build-mpt's libs, and the evm::gpu::* symbols resolve among the four
// archives themselves. The only externally-provided symbols are
// Metal/Foundation framework symbols (MTLCreateSystemDefaultDevice,
// MTLCompileOptions, CFRetain/CFRelease, …) — hence exactly two `-framework`
// flags; no MetalPerformanceShaders and no luxgpu are referenced by these
// archives.
//
// Rebuild recipe (mac, Metal toolchain-free — ObjC++ host libs link without
// `xcrun metal`; shaders compile at runtime):
//   export LUXCPP_PREFIX=/opt/homebrew PKG_CONFIG_PATH=/opt/homebrew/lib/pkgconfig \
//          DYLD_LIBRARY_PATH=/opt/homebrew/lib SDKROOT=$(xcrun --show-sdk-path)
//   cmake -S /Users/z/work/luxcpp/cevm -B /Users/z/work/luxcpp/cevm/build-mpt-gpu \
//     -G "Unix Makefiles" -DCMAKE_BUILD_TYPE=Release -DLUX_CEVM_ENABLE_METAL=ON \
//     -DCMAKE_TOOLCHAIN_FILE=/Users/z/work/luxcpp/cevm/build-mpt/build/Release/generators/conan_toolchain.cmake
//   cmake --build /Users/z/work/luxcpp/cevm/build-mpt-gpu \
//     --target evm-gpu evm-metal-hosts evm-kernel-metal evm-gpu-state -j8
//
// SCOPE — BUILD + LINK ONLY. Metal shaders compile at RUNTIME
// (newLibraryWithSource), and MTLCreateSystemDefaultDevice() returns nil in a
// headless/CLI session with no window-server, so actual GPU DISPATCH cannot be
// verified in this environment — it is CI-gated on a Metal-capable interactive
// runner. GPUABIVersion / GPUBackendName below are device-FREE queries (a
// compiled-in constant and a static string table); they anchor the GPU
// dispatch objects into the link without touching a device, and MUST NOT be
// read as evidence that dispatch works.

/*
#cgo CFLAGS: -I/Users/z/work/luxcpp/cevm/lib/evm/gpu
#cgo LDFLAGS: /Users/z/work/luxcpp/cevm/build-mpt-gpu/lib/evm/libevm-gpu.a /Users/z/work/luxcpp/cevm/build-mpt-gpu/lib/evm/libevm-metal-hosts.a /Users/z/work/luxcpp/cevm/build-mpt-gpu/lib/evm/libevm-kernel-metal.a /Users/z/work/luxcpp/cevm/build-mpt-gpu/lib/evm/libevm-gpu-state.a -framework Metal -framework Foundation
#include "go_bridge.h"
*/
import "C"

// GPULinked reports that the cevm GPU host archives are compiled into this
// build. It is only defined under `-tags "cevm cevmgpu"`; the CPU-only `-tags
// cevm` build has no such symbol (nothing references it unconditionally, so no
// stub is required).
const GPULinked = true

// GPUABIVersion returns the evm::gpu C-ABI version compiled into the linked GPU
// archives (EVM_GPU_ABI_VERSION in go_bridge.h). Device-free: a compiled-in
// constant, no Metal device is created.
func GPUABIVersion() uint32 { return uint32(C.gpu_abi_version()) }

// GPUBackendName maps an evm::gpu backend id (EVM_GPU_BACKEND_CPU_SEQUENTIAL /
// _CPU_PARALLEL / _METAL / _CUDA) to its static name string. Device-free: a
// static lookup, no Metal device is touched. This does NOT probe or dispatch.
func GPUBackendName(backend uint8) string {
	return C.GoString(C.gpu_backend_name(C.uint8_t(backend)))
}

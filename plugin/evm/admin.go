// (c) 2020-2020, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"fmt"
	"net/http"

	"github.com/luxfi/evm/v2/plugin/evm/client"
	"github.com/luxfi/log"
	"github.com/luxfi/node/api"
	"github.com/luxfi/node/utils/profiler"
)

// Admin is the API service for admin API calls
type Admin struct {
	vm       *VM
	profiler profiler.Profiler
}

func NewAdminService(vm *VM, performanceDir string) *Admin {
	return &Admin{
		vm:       vm,
		profiler: profiler.New(performanceDir),
	}
}

// StartCPUProfiler starts a cpu profile writing to the specified file
func (p *Admin) StartCPUProfiler(_ *http.Request, _ *struct{}, _ *api.EmptyReply) error {
	log.Info("Admin: StartCPUProfiler called")

	p.vm.vmLock.Lock()
	defer p.vm.vmLock.Unlock()

	return p.profiler.StartCPUProfiler()
}

// StopCPUProfiler stops the cpu profile
func (p *Admin) StopCPUProfiler(r *http.Request, _ *struct{}, _ *api.EmptyReply) error {
	log.Info("Admin: StopCPUProfiler called")

	p.vm.vmLock.Lock()
	defer p.vm.vmLock.Unlock()

	return p.profiler.StopCPUProfiler()
}

// MemoryProfile runs a memory profile writing to the specified file
func (p *Admin) MemoryProfile(_ *http.Request, _ *struct{}, _ *api.EmptyReply) error {
	log.Info("Admin: MemoryProfile called")

	p.vm.vmLock.Lock()
	defer p.vm.vmLock.Unlock()

	return p.profiler.MemoryProfile()
}

// LockProfile runs a mutex profile writing to the specified file
func (p *Admin) LockProfile(_ *http.Request, _ *struct{}, _ *api.EmptyReply) error {
	log.Info("Admin: LockProfile called")

	p.vm.vmLock.Lock()
	defer p.vm.vmLock.Unlock()

	return p.profiler.LockProfile()
}

func (p *Admin) SetLogLevel(_ *http.Request, args *client.SetLogLevelArgs, reply *api.EmptyReply) error {
	log.Info("EVM: SetLogLevel called", "logLevel", args.Level)

	p.vm.vmLock.Lock()
	defer p.vm.vmLock.Unlock()

	// TODO: Implement SetLogLevel when luxfi/log supports dynamic level changes
	// For now, log level changes are not supported
	return fmt.Errorf("dynamic log level changes are not currently supported")
}

func (p *Admin) GetVMConfig(_ *http.Request, _ *struct{}, reply *client.ConfigReply) error {
	// Convert VM config to the expected type
	// For now, return nil as the config types don't match
	reply.Config = nil
	return nil
}

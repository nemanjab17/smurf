package vm_test

import (
	"testing"

	"github.com/nemanjab17/smurf/internal/vm"
)

func TestCreateOpts_ApplyDefaults(t *testing.T) {
	opts := vm.CreateOpts{Name: "dev", PapaID: "base"}
	// applyDefaults is unexported; test via manager behaviour indirectly.
	// Here we just verify the exported constants are sane.
	if vm.DefaultVCPUs <= 0 {
		t.Error("DefaultVCPUs must be positive")
	}
	if vm.DefaultMemoryMB < 512 {
		t.Error("DefaultMemoryMB seems too small")
	}
	if vm.DefaultDiskSizeMB < 1024 {
		t.Error("DefaultDiskSizeMB seems too small")
	}
	_ = opts
}

func TestNewRunningVM(t *testing.T) {
	rvm := vm.NewRunningVM("id-1", "/tmp/sock", "10.0.0.2", 9999)
	if rvm.ID != "id-1" {
		t.Errorf("ID: got %q", rvm.ID)
	}
	if rvm.IP != "10.0.0.2" {
		t.Errorf("IP: got %q", rvm.IP)
	}
	if rvm.PID != 9999 {
		t.Errorf("PID: got %d", rvm.PID)
	}
	// Stop on a mock VM (machine == nil) must not panic
	if err := rvm.Stop(nil); err != nil {
		t.Errorf("Stop on mock VM: %v", err)
	}
}

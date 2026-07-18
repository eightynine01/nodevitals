package collector

import "context"

// gpuDevice is a neutral snapshot of one GPU's polled telemetry — keeps
// go-nvml's cgo-bound types out of the collector surface, mirroring how
// smartDevice keeps anatol/smart.go's ioctl-bound types out (see smart.go).
// Unused this task; the GPU collector that maps this into model.Sample
// arrives in a later task.
type gpuDevice struct {
	Index                                               int
	UUID, Name                                          string
	UtilGPU, MemUsedBytes, MemTotalBytes, TempC, PowerW float64
	ThrottleReasons                                     uint64
	EccUncorrected                                      float64
}

// xidRaw is one raw XID event as delivered by the NVML EventSet subscription
// goroutine (added in a later task, gpu-tagged/cgo). Classification of the
// Xid field happens in xid.go (untagged, pure Go).
type xidRaw struct {
	DeviceIndex int
	UUID        string
	Xid         uint64
}

// gpuReader is production code's seam onto go-nvml: NVML has no pure-Go
// interface package (even pkg/nvml/mock imports the cgo-bound pkg/nvml), so
// CGO_ENABLED=0 builds and tests can never import go-nvml. All GPU collector
// logic (a later task) is tested against a fake gpuReader instead — the same
// pattern smartProbe uses for anatol/smart.go. The gpu-tagged NVML
// implementation lives behind this interface in a later task.
type gpuReader interface {
	Read(ctx context.Context) ([]gpuDevice, error) // polled snapshot
	XidEvents() <-chan xidRaw                      // async XID feed
	Close() error
}

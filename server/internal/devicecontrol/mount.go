package devicecontrol

import (
	"net/http"

	"connectrpc.com/connect"

	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
)

// MountLiveControl registers live device-control procedures.
func (h *Handlers) MountLiveControl(mux *http.ServeMux, opts ...connect.HandlerOption) []string {
	if mux == nil {
		panic("devicecontrol: mux is required")
	}
	mounted := make([]string, 0, 2)
	register := func(procedure string, handler http.Handler) {
		mux.Handle(procedure, handler)
		mounted = append(mounted, procedure)
	}
	register(cadestrov1connect.ControlServiceSyncDeviceProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceSyncDeviceProcedure, h.SyncDevice, opts...))
	register(cadestrov1connect.ControlServiceRebootDeviceProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceRebootDeviceProcedure, h.RebootDevice, opts...))
	return mounted
}

// LiveControlProcedures is the exact audited live-control surface implemented here.
func LiveControlProcedures() []string {
	return []string{
		cadestrov1connect.ControlServiceSyncDeviceProcedure,
		cadestrov1connect.ControlServiceRebootDeviceProcedure,
	}
}

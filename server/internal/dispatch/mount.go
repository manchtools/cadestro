package dispatch

import (
	"net/http"

	"connectrpc.com/connect"

	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
)

// MountActions registers live device-control procedures.
func (h *Handlers) MountActions(mux *http.ServeMux, opts ...connect.HandlerOption) []string {
	if mux == nil {
		panic("dispatch: mux is required")
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

// MutationProcedures is the exact audited dispatch surface implemented here.
func MutationProcedures() []string {
	return []string{
		cadestrov1connect.ControlServiceSyncDeviceProcedure,
		cadestrov1connect.ControlServiceRebootDeviceProcedure,
	}
}

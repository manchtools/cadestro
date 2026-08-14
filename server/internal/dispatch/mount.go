package dispatch

import (
	"net/http"

	"connectrpc.com/connect"

	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
)

// MountActions registers the direct singleton dispatch procedures.
func (h *Handlers) MountActions(mux *http.ServeMux, opts ...connect.HandlerOption) []string {
	if mux == nil {
		panic("dispatch: mux is required")
	}
	mounted := make([]string, 0, 7)
	register := func(procedure string, handler http.Handler) {
		mux.Handle(procedure, handler)
		mounted = append(mounted, procedure)
	}
	register(cadestrov1connect.ControlServiceDispatchActionProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceDispatchActionProcedure, h.DispatchAction, opts...))
	register(cadestrov1connect.ControlServiceDispatchInstantActionProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceDispatchInstantActionProcedure, h.DispatchInstantAction, opts...))
	register(cadestrov1connect.ControlServiceDispatchActionSetProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceDispatchActionSetProcedure, h.DispatchActionSet, opts...))
	register(cadestrov1connect.ControlServiceDispatchDefinitionProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceDispatchDefinitionProcedure, h.DispatchDefinition, opts...))
	register(cadestrov1connect.ControlServiceDispatchToMultipleProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceDispatchToMultipleProcedure, h.DispatchToMultiple, opts...))
	register(cadestrov1connect.ControlServiceDispatchToGroupProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceDispatchToGroupProcedure, h.DispatchToGroup, opts...))
	register(cadestrov1connect.ControlServiceDispatchAssignedActionsProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceDispatchAssignedActionsProcedure, h.DispatchAssignedActions, opts...))
	return mounted
}

// MutationProcedures is the exact audited dispatch surface implemented here.
func MutationProcedures() []string {
	return []string{
		cadestrov1connect.ControlServiceDispatchActionProcedure,
		cadestrov1connect.ControlServiceDispatchInstantActionProcedure,
		cadestrov1connect.ControlServiceDispatchActionSetProcedure,
		cadestrov1connect.ControlServiceDispatchDefinitionProcedure,
		cadestrov1connect.ControlServiceDispatchToMultipleProcedure,
		cadestrov1connect.ControlServiceDispatchToGroupProcedure,
		cadestrov1connect.ControlServiceDispatchAssignedActionsProcedure,
	}
}

package registrationtoken

import (
	"net/http"

	"connectrpc.com/connect"

	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
)

// Mount registers exactly the explicit registration-token procedures.
func (h *Handlers) Mount(mux *http.ServeMux, opts ...connect.HandlerOption) []string {
	if mux == nil {
		panic("registrationtoken: mux is required")
	}
	mounted := make([]string, 0, 5)
	register := func(procedure string, handler http.Handler) {
		mux.Handle(procedure, handler)
		mounted = append(mounted, procedure)
	}
	register(cadestrov1connect.ControlServiceCreateTokenProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceCreateTokenProcedure, h.CreateToken, opts...))
	register(cadestrov1connect.ControlServiceListTokensProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceListTokensProcedure, h.ListTokens, opts...))
	register(cadestrov1connect.ControlServiceRenameTokenProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceRenameTokenProcedure, h.RenameToken, opts...))
	register(cadestrov1connect.ControlServiceSetTokenDisabledProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceSetTokenDisabledProcedure, h.SetTokenDisabled, opts...))
	register(cadestrov1connect.ControlServiceDeleteTokenProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceDeleteTokenProcedure, h.DeleteToken, opts...))
	return mounted
}

// MutationProcedures is the exact audited registration-token mutation set.
func MutationProcedures() []string {
	return []string{
		cadestrov1connect.ControlServiceCreateTokenProcedure,
		cadestrov1connect.ControlServiceRenameTokenProcedure,
		cadestrov1connect.ControlServiceSetTokenDisabledProcedure,
		cadestrov1connect.ControlServiceDeleteTokenProcedure,
	}
}

// ReadProcedures is the exact non-mutating registration-token set.
func ReadProcedures() []string {
	return []string{
		cadestrov1connect.ControlServiceListTokensProcedure,
	}
}

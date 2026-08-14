package searchrpc

import (
	"net/http"

	"connectrpc.com/connect"

	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
)

// Mount registers exactly the SQLite FTS5 search procedures.
func (h *Handlers) Mount(mux *http.ServeMux, opts ...connect.HandlerOption) []string {
	if mux == nil {
		panic("search: mux is required")
	}
	mounted := make([]string, 0, 2)
	register := func(procedure string, handler http.Handler) {
		mux.Handle(procedure, handler)
		mounted = append(mounted, procedure)
	}
	register(cadestrov1connect.ControlServiceSearchProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceSearchProcedure, h.Search, opts...))
	register(cadestrov1connect.ControlServiceRebuildSearchIndexProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceRebuildSearchIndexProcedure, h.RebuildSearchIndex, opts...))
	return mounted
}

// ReadProcedures is the exact non-mutating search surface.
func ReadProcedures() []string {
	return []string{cadestrov1connect.ControlServiceSearchProcedure}
}

// MutationProcedures is the exact audited search-maintenance surface.
func MutationProcedures() []string {
	return []string{cadestrov1connect.ControlServiceRebuildSearchIndexProcedure}
}

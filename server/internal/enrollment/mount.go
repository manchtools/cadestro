package enrollment

import (
	"net/http"

	"connectrpc.com/connect"

	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
)

// MountRegister mounts the token-authenticated enrollment procedure. Renewal
// is deliberately mounted on the mTLS agent listener instead.
func (h *Handlers) MountRegister(mux *http.ServeMux, opts ...connect.HandlerOption) []string {
	if mux == nil {
		panic("enrollment: mux is required")
	}
	mux.Handle(cadestrov1connect.ControlServiceRegisterProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceRegisterProcedure, h.Register, opts...))
	return []string{cadestrov1connect.ControlServiceRegisterProcedure}
}

// MountRenewal mounts certificate renewal behind the direct mTLS listener.
func (h *Handlers) MountRenewal(mux *http.ServeMux, opts ...connect.HandlerOption) []string {
	if mux == nil {
		panic("enrollment: mux is required")
	}
	mux.Handle(cadestrov1connect.ControlServiceRenewCertificateProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceRenewCertificateProcedure, h.RenewCertificate, opts...))
	return []string{cadestrov1connect.ControlServiceRenewCertificateProcedure}
}

// MutationProcedures is the exact audited enrollment surface.
func MutationProcedures() []string {
	return []string{
		cadestrov1connect.ControlServiceRegisterProcedure,
		cadestrov1connect.ControlServiceRenewCertificateProcedure,
	}
}

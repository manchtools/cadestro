package controlrpc

import (
	"net/http"

	"connectrpc.com/connect"

	"github.com/manchtools/cadestro/server/internal/assignment"
	"github.com/manchtools/cadestro/server/internal/authoring"
	"github.com/manchtools/cadestro/server/internal/compliance"
	"github.com/manchtools/cadestro/server/internal/device"
	"github.com/manchtools/cadestro/server/internal/devicecontrol"
	"github.com/manchtools/cadestro/server/internal/devicegroup"
	"github.com/manchtools/cadestro/server/internal/enrollment"
	"github.com/manchtools/cadestro/server/internal/identity"
	"github.com/manchtools/cadestro/server/internal/registrationtoken"
	"github.com/manchtools/cadestro/server/internal/searchrpc"
)

type Handlers struct {
	Identity           *identity.Handlers
	Enrollment         *enrollment.Handlers
	Authoring          *authoring.Handlers
	Assignments        *assignment.Handlers
	DeviceGroups       *devicegroup.Handlers
	Devices            *device.Handlers
	RegistrationTokens *registrationtoken.Handlers
	Compliance         *compliance.Handlers
	DeviceControl      *devicecontrol.Handlers
	Search             *searchrpc.Handlers
}

func (h Handlers) Mount(mux *http.ServeMux, opts ...connect.HandlerOption) []string {
	if mux == nil || h.Identity == nil || h.Enrollment == nil || h.Authoring == nil ||
		h.Assignments == nil || h.DeviceGroups == nil || h.Devices == nil ||
		h.RegistrationTokens == nil || h.Compliance == nil || h.DeviceControl == nil || h.Search == nil {
		panic("controlrpc: complete handler wiring is required")
	}
	var mounted []string
	mounted = append(mounted, h.Identity.Mount(mux, opts...)...)
	mounted = append(mounted, h.Enrollment.MountRegister(mux, opts...)...)
	mounted = append(mounted, h.Authoring.MountActions(mux, opts...)...)
	mounted = append(mounted, h.Authoring.MountActionSets(mux, opts...)...)
	mounted = append(mounted, h.Authoring.MountDefinitions(mux, opts...)...)
	mounted = append(mounted, h.Assignments.Mount(mux, opts...)...)
	mounted = append(mounted, h.DeviceGroups.Mount(mux, opts...)...)
	mounted = append(mounted, h.Devices.Mount(mux, opts...)...)
	mounted = append(mounted, h.RegistrationTokens.Mount(mux, opts...)...)
	mounted = append(mounted, h.Compliance.MountPolicies(mux, opts...)...)
	mounted = append(mounted, h.DeviceControl.MountLiveControl(mux, opts...)...)
	mounted = append(mounted, h.Search.Mount(mux, opts...)...)
	return mounted
}

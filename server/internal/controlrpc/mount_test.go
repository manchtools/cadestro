package controlrpc

import (
	"fmt"
	"net/http"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/assignment"
	"github.com/manchtools/cadestro/server/internal/authoring"
	"github.com/manchtools/cadestro/server/internal/compliance"
	"github.com/manchtools/cadestro/server/internal/device"
	"github.com/manchtools/cadestro/server/internal/devicegroup"
	"github.com/manchtools/cadestro/server/internal/dispatch"
	"github.com/manchtools/cadestro/server/internal/enrollment"
	"github.com/manchtools/cadestro/server/internal/identity"
	"github.com/manchtools/cadestro/server/internal/registrationtoken"
	"github.com/manchtools/cadestro/server/internal/searchrpc"
)

func TestMountIsExactControlServiceDescriptorSet(t *testing.T) {
	handlers := Handlers{
		Identity: &identity.Handlers{}, Enrollment: &enrollment.Handlers{},
		Authoring: &authoring.Handlers{}, Assignments: &assignment.Handlers{},
		DeviceGroups: &devicegroup.Handlers{}, Devices: &device.Handlers{},
		RegistrationTokens: &registrationtoken.Handlers{}, Compliance: &compliance.Handlers{},
		Dispatch: &dispatch.Handlers{}, Search: &searchrpc.Handlers{},
	}
	mounted := handlers.Mount(http.NewServeMux())
	got := make(map[string]struct{}, len(mounted))
	var duplicates []string
	for _, procedure := range mounted {
		if _, exists := got[procedure]; exists {
			duplicates = append(duplicates, procedure)
		}
		got[procedure] = struct{}{}
	}
	assert.Empty(t, duplicates, "one procedure must have one direct owner")

	service := cadestrov1.File_cadestro_v1_control_proto.Services().ByName("ControlService")
	require.NotNil(t, service)
	want := make(map[string]struct{}, service.Methods().Len())
	for i := 0; i < service.Methods().Len(); i++ {
		method := service.Methods().Get(i)
		want[fmt.Sprintf("/%s/%s", service.FullName(), method.Name())] = struct{}{}
	}
	// Renewal is mounted only on the authenticated agent listener; the public
	// control mux owns registration and the operator-facing surface.
	delete(want, "/cadestro.v1.ControlService/RenewCertificate")

	var missing, extra []string
	for procedure := range want {
		if _, exists := got[procedure]; !exists {
			missing = append(missing, procedure)
		}
	}
	for procedure := range got {
		if _, exists := want[procedure]; !exists {
			extra = append(extra, procedure)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	assert.Empty(t, missing)
	assert.Empty(t, extra)
	assert.Len(t, mounted, len(want))
}

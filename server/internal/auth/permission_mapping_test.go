package auth

import (
	"fmt"
	"testing"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
)

func TestControlServicePermissionMappingMatchesDescriptor(t *testing.T) {
	public := map[string]bool{
		cadestrov1connect.ControlServiceRefreshTokenProcedure:     true,
		cadestrov1connect.ControlServiceLogoutProcedure:           true,
		cadestrov1connect.ControlServiceListAuthMethodsProcedure:  true,
		cadestrov1connect.ControlServiceGetSSOLoginURLProcedure:   true,
		cadestrov1connect.ControlServiceSSOCallbackProcedure:      true,
		cadestrov1connect.ControlServiceRegisterProcedure:         true,
		cadestrov1connect.ControlServiceRenewCertificateProcedure: true,
	}
	service := cadestrov1.File_cadestro_v1_control_proto.Services().ByName("ControlService")
	counts := make(map[cadestrov1.Permission]int)
	for i := 0; i < service.Methods().Len(); i++ {
		procedure := fmt.Sprintf("/cadestro.v1.ControlService/%s", service.Methods().Get(i).Name())
		permission, gated := PermissionForProcedure(procedure)
		if public[procedure] {
			if gated {
				t.Fatalf("public procedure %s is permission-gated", procedure)
			}
			continue
		}
		if !gated {
			t.Fatalf("protected procedure %s is unmapped", procedure)
		}
		counts[permission]++
	}
	for permission := cadestrov1.Permission_PERMISSION_GET_CURRENT_USER; permission <= cadestrov1.Permission_PERMISSION_REVOKE_USER_SESSIONS; permission++ {
		if counts[permission] == 0 {
			t.Fatalf("permission %s is unused", permission)
		}
	}
}

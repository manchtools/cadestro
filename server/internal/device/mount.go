package device

import (
	"net/http"

	"connectrpc.com/connect"

	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
)

func (h *Handlers) Mount(mux *http.ServeMux, opts ...connect.HandlerOption) []string {
	if mux == nil {
		panic("device: mux is required")
	}
	mounted := make([]string, 0, 31)
	register := func(procedure string, handler http.Handler) {
		mux.Handle(procedure, handler)
		mounted = append(mounted, procedure)
	}
	register(cadestrov1connect.ControlServiceListDevicesProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceListDevicesProcedure, h.ListDevices, opts...))
	register(cadestrov1connect.ControlServiceGetDeviceProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceGetDeviceProcedure, h.GetDevice, opts...))
	register(cadestrov1connect.ControlServiceGetDeviceInventoryProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceGetDeviceInventoryProcedure, h.GetDeviceInventory, opts...))
	register(cadestrov1connect.ControlServiceGetOSQueryResultProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceGetOSQueryResultProcedure, h.GetOSQueryResult, opts...))
	register(cadestrov1connect.ControlServiceGetDeviceLogResultProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceGetDeviceLogResultProcedure, h.GetDeviceLogResult, opts...))
	register(cadestrov1connect.ControlServiceGetDeviceComplianceProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceGetDeviceComplianceProcedure, h.GetDeviceCompliance, opts...))
	register(cadestrov1connect.ControlServiceGetDeviceCompliancePolicyStatusProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceGetDeviceCompliancePolicyStatusProcedure, h.GetDeviceCompliancePolicyStatus, opts...))
	register(cadestrov1connect.ControlServiceListLpsPasswordsProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceListLpsPasswordsProcedure, h.ListLpsPasswords, opts...))
	register(cadestrov1connect.ControlServiceRevealLpsPasswordProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceRevealLpsPasswordProcedure, h.RevealLpsPassword, opts...))
	register(cadestrov1connect.ControlServiceListLuksKeysProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceListLuksKeysProcedure, h.ListLuksKeys, opts...))
	register(cadestrov1connect.ControlServiceRevealLuksKeyProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceRevealLuksKeyProcedure, h.RevealLuksKey, opts...))
	register(cadestrov1connect.ControlServiceCreateLuksTokenProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceCreateLuksTokenProcedure, h.CreateLuksToken, opts...))
	register(cadestrov1connect.ControlServiceRevokeLuksDeviceKeyProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceRevokeLuksDeviceKeyProcedure, h.RevokeLuksDeviceKey, opts...))
	register(cadestrov1connect.ControlServiceDispatchOSQueryProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceDispatchOSQueryProcedure, h.DispatchOSQuery, opts...))
	register(cadestrov1connect.ControlServiceRefreshDeviceInventoryProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceRefreshDeviceInventoryProcedure, h.RefreshDeviceInventory, opts...))
	register(cadestrov1connect.ControlServiceQueryDeviceLogsProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceQueryDeviceLogsProcedure, h.QueryDeviceLogs, opts...))
	register(cadestrov1connect.ControlServiceStartTerminalProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceStartTerminalProcedure, h.StartTerminal, opts...))
	register(cadestrov1connect.ControlServiceStopTerminalProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceStopTerminalProcedure, h.StopTerminal, opts...))
	register(cadestrov1connect.ControlServiceListActiveTerminalSessionsProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceListActiveTerminalSessionsProcedure, h.ListActiveTerminalSessions, opts...))
	register(cadestrov1connect.ControlServiceTerminateTerminalSessionProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceTerminateTerminalSessionProcedure, h.TerminateTerminalSession, opts...))
	register(cadestrov1connect.ControlServiceSetDeviceLabelProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceSetDeviceLabelProcedure, h.SetDeviceLabel, opts...))
	register(cadestrov1connect.ControlServiceRemoveDeviceLabelProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceRemoveDeviceLabelProcedure, h.RemoveDeviceLabel, opts...))
	register(cadestrov1connect.ControlServiceAssignDeviceProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceAssignDeviceProcedure, h.AssignDevice, opts...))
	register(cadestrov1connect.ControlServiceUnassignDeviceProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceUnassignDeviceProcedure, h.UnassignDevice, opts...))
	register(cadestrov1connect.ControlServiceListDeviceAssigneesProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceListDeviceAssigneesProcedure, h.ListDeviceAssignees, opts...))
	register(cadestrov1connect.ControlServiceSetDeviceSyncIntervalProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceSetDeviceSyncIntervalProcedure, h.SetDeviceSyncInterval, opts...))
	register(cadestrov1connect.ControlServiceSetDeviceInventoryIntervalProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceSetDeviceInventoryIntervalProcedure, h.SetDeviceInventoryInterval, opts...))
	register(cadestrov1connect.ControlServiceDeleteDeviceProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceDeleteDeviceProcedure, h.DeleteDevice, opts...))
	return mounted
}

func MutationProcedures() []string {
	return []string{
		cadestrov1connect.ControlServiceSetDeviceLabelProcedure,
		cadestrov1connect.ControlServiceRemoveDeviceLabelProcedure,
		cadestrov1connect.ControlServiceAssignDeviceProcedure,
		cadestrov1connect.ControlServiceUnassignDeviceProcedure,
		cadestrov1connect.ControlServiceSetDeviceSyncIntervalProcedure,
		cadestrov1connect.ControlServiceSetDeviceInventoryIntervalProcedure,
		cadestrov1connect.ControlServiceDeleteDeviceProcedure,
		cadestrov1connect.ControlServiceCreateLuksTokenProcedure,
		cadestrov1connect.ControlServiceRevokeLuksDeviceKeyProcedure,
		cadestrov1connect.ControlServiceDispatchOSQueryProcedure,
		cadestrov1connect.ControlServiceRefreshDeviceInventoryProcedure,
		cadestrov1connect.ControlServiceQueryDeviceLogsProcedure,
		cadestrov1connect.ControlServiceStartTerminalProcedure,
		cadestrov1connect.ControlServiceStopTerminalProcedure,
		cadestrov1connect.ControlServiceTerminateTerminalSessionProcedure,
	}
}

func ReadProcedures() []string {
	return []string{
		cadestrov1connect.ControlServiceListDevicesProcedure,
		cadestrov1connect.ControlServiceGetDeviceProcedure,
		cadestrov1connect.ControlServiceListDeviceAssigneesProcedure,
	}
}

func SensitiveReadProcedures() []string {
	return []string{
		cadestrov1connect.ControlServiceGetDeviceInventoryProcedure,
		cadestrov1connect.ControlServiceGetOSQueryResultProcedure,
		cadestrov1connect.ControlServiceGetDeviceLogResultProcedure,
		cadestrov1connect.ControlServiceGetDeviceComplianceProcedure,
		cadestrov1connect.ControlServiceGetDeviceCompliancePolicyStatusProcedure,
		cadestrov1connect.ControlServiceListLpsPasswordsProcedure,
		cadestrov1connect.ControlServiceRevealLpsPasswordProcedure,
		cadestrov1connect.ControlServiceListLuksKeysProcedure,
		cadestrov1connect.ControlServiceRevealLuksKeyProcedure,
		cadestrov1connect.ControlServiceListActiveTerminalSessionsProcedure,
	}
}

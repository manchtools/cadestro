package authoring

import (
	"net/http"

	"connectrpc.com/connect"

	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
)

// MountActions registers exactly the explicit Action CRUD procedures.
func (h *Handlers) MountActions(mux *http.ServeMux, opts ...connect.HandlerOption) []string {
	if mux == nil {
		panic("authoring: mux is required")
	}
	mounted := make([]string, 0, 7)
	register := func(procedure string, handler http.Handler) {
		mux.Handle(procedure, handler)
		mounted = append(mounted, procedure)
	}
	register(cadestrov1connect.ControlServiceCreateActionProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceCreateActionProcedure, h.CreateAction, opts...))
	register(cadestrov1connect.ControlServiceGetActionProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceGetActionProcedure, h.GetAction, opts...))
	register(cadestrov1connect.ControlServiceListActionsProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceListActionsProcedure, h.ListActions, opts...))
	register(cadestrov1connect.ControlServiceRenameActionProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceRenameActionProcedure, h.RenameAction, opts...))
	register(cadestrov1connect.ControlServiceUpdateActionDescriptionProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceUpdateActionDescriptionProcedure, h.UpdateActionDescription, opts...))
	register(cadestrov1connect.ControlServiceUpdateActionParamsProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceUpdateActionParamsProcedure, h.UpdateActionParams, opts...))
	register(cadestrov1connect.ControlServiceDeleteActionProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceDeleteActionProcedure, h.DeleteAction, opts...))
	return mounted
}

// ActionMutationProcedures is the exact audited Action mutation surface.
func ActionMutationProcedures() []string {
	return []string{
		cadestrov1connect.ControlServiceCreateActionProcedure,
		cadestrov1connect.ControlServiceRenameActionProcedure,
		cadestrov1connect.ControlServiceUpdateActionDescriptionProcedure,
		cadestrov1connect.ControlServiceUpdateActionParamsProcedure,
		cadestrov1connect.ControlServiceDeleteActionProcedure,
	}
}

// ActionReadProcedures is the exact non-mutating Action surface.
func ActionReadProcedures() []string {
	return []string{
		cadestrov1connect.ControlServiceGetActionProcedure,
		cadestrov1connect.ControlServiceListActionsProcedure,
	}
}

// MountActionSets registers exactly the explicit ActionSet CRUD procedures.
func (h *Handlers) MountActionSets(mux *http.ServeMux, opts ...connect.HandlerOption) []string {
	if mux == nil {
		panic("authoring: mux is required")
	}
	mounted := make([]string, 0, 10)
	register := func(procedure string, handler http.Handler) {
		mux.Handle(procedure, handler)
		mounted = append(mounted, procedure)
	}
	register(cadestrov1connect.ControlServiceCreateActionSetProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceCreateActionSetProcedure, h.CreateActionSet, opts...))
	register(cadestrov1connect.ControlServiceGetActionSetProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceGetActionSetProcedure, h.GetActionSet, opts...))
	register(cadestrov1connect.ControlServiceListActionSetsProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceListActionSetsProcedure, h.ListActionSets, opts...))
	register(cadestrov1connect.ControlServiceRenameActionSetProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceRenameActionSetProcedure, h.RenameActionSet, opts...))
	register(cadestrov1connect.ControlServiceUpdateActionSetDescriptionProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceUpdateActionSetDescriptionProcedure, h.UpdateActionSetDescription, opts...))
	register(cadestrov1connect.ControlServiceUpdateActionSetScheduleProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceUpdateActionSetScheduleProcedure, h.UpdateActionSetSchedule, opts...))
	register(cadestrov1connect.ControlServiceDeleteActionSetProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceDeleteActionSetProcedure, h.DeleteActionSet, opts...))
	register(cadestrov1connect.ControlServiceAddActionToSetProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceAddActionToSetProcedure, h.AddActionToSet, opts...))
	register(cadestrov1connect.ControlServiceRemoveActionFromSetProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceRemoveActionFromSetProcedure, h.RemoveActionFromSet, opts...))
	register(cadestrov1connect.ControlServiceReorderActionInSetProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceReorderActionInSetProcedure, h.ReorderActionInSet, opts...))
	return mounted
}

// ActionSetMutationProcedures is the exact audited ActionSet mutation surface.
func ActionSetMutationProcedures() []string {
	return []string{
		cadestrov1connect.ControlServiceCreateActionSetProcedure,
		cadestrov1connect.ControlServiceRenameActionSetProcedure,
		cadestrov1connect.ControlServiceUpdateActionSetDescriptionProcedure,
		cadestrov1connect.ControlServiceUpdateActionSetScheduleProcedure,
		cadestrov1connect.ControlServiceDeleteActionSetProcedure,
		cadestrov1connect.ControlServiceAddActionToSetProcedure,
		cadestrov1connect.ControlServiceRemoveActionFromSetProcedure,
		cadestrov1connect.ControlServiceReorderActionInSetProcedure,
	}
}

// ActionSetReadProcedures is the exact non-mutating ActionSet surface.
func ActionSetReadProcedures() []string {
	return []string{
		cadestrov1connect.ControlServiceGetActionSetProcedure,
		cadestrov1connect.ControlServiceListActionSetsProcedure,
	}
}

// MountDefinitions registers exactly the explicit Definition CRUD procedures.
func (h *Handlers) MountDefinitions(mux *http.ServeMux, opts ...connect.HandlerOption) []string {
	if mux == nil {
		panic("authoring: mux is required")
	}
	mounted := make([]string, 0, 10)
	register := func(procedure string, handler http.Handler) {
		mux.Handle(procedure, handler)
		mounted = append(mounted, procedure)
	}
	register(cadestrov1connect.ControlServiceCreateDefinitionProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceCreateDefinitionProcedure, h.CreateDefinition, opts...))
	register(cadestrov1connect.ControlServiceGetDefinitionProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceGetDefinitionProcedure, h.GetDefinition, opts...))
	register(cadestrov1connect.ControlServiceListDefinitionsProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceListDefinitionsProcedure, h.ListDefinitions, opts...))
	register(cadestrov1connect.ControlServiceRenameDefinitionProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceRenameDefinitionProcedure, h.RenameDefinition, opts...))
	register(cadestrov1connect.ControlServiceUpdateDefinitionDescriptionProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceUpdateDefinitionDescriptionProcedure, h.UpdateDefinitionDescription, opts...))
	register(cadestrov1connect.ControlServiceUpdateDefinitionScheduleProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceUpdateDefinitionScheduleProcedure, h.UpdateDefinitionSchedule, opts...))
	register(cadestrov1connect.ControlServiceDeleteDefinitionProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceDeleteDefinitionProcedure, h.DeleteDefinition, opts...))
	register(cadestrov1connect.ControlServiceAddActionSetToDefinitionProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceAddActionSetToDefinitionProcedure, h.AddActionSetToDefinition, opts...))
	register(cadestrov1connect.ControlServiceRemoveActionSetFromDefinitionProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceRemoveActionSetFromDefinitionProcedure, h.RemoveActionSetFromDefinition, opts...))
	register(cadestrov1connect.ControlServiceReorderActionSetInDefinitionProcedure,
		connect.NewUnaryHandler(cadestrov1connect.ControlServiceReorderActionSetInDefinitionProcedure, h.ReorderActionSetInDefinition, opts...))
	return mounted
}

// DefinitionMutationProcedures is the exact audited Definition mutation
// surface.
func DefinitionMutationProcedures() []string {
	return []string{
		cadestrov1connect.ControlServiceCreateDefinitionProcedure,
		cadestrov1connect.ControlServiceRenameDefinitionProcedure,
		cadestrov1connect.ControlServiceUpdateDefinitionDescriptionProcedure,
		cadestrov1connect.ControlServiceUpdateDefinitionScheduleProcedure,
		cadestrov1connect.ControlServiceDeleteDefinitionProcedure,
		cadestrov1connect.ControlServiceAddActionSetToDefinitionProcedure,
		cadestrov1connect.ControlServiceRemoveActionSetFromDefinitionProcedure,
		cadestrov1connect.ControlServiceReorderActionSetInDefinitionProcedure,
	}
}

// DefinitionReadProcedures is the exact non-mutating Definition surface.
func DefinitionReadProcedures() []string {
	return []string{
		cadestrov1connect.ControlServiceGetDefinitionProcedure,
		cadestrov1connect.ControlServiceListDefinitionsProcedure,
	}
}

package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

func (service *Service) deviceStatus(device *db.Device) cadestrov1.DeviceStatus {
	if device.LastSeenAt != nil && service.now().UTC().Sub(device.LastSeenAt.UTC()) <= 2*service.heartbeatInterval {
		return cadestrov1.DeviceStatus_DEVICE_STATUS_ONLINE
	}
	return cadestrov1.DeviceStatus_DEVICE_STATUS_OFFLINE
}

func (service *Service) deviceProto(ctx context.Context, device *db.Device) (*cadestrov1.Device, error) {
	checks, err := service.store.Queries().ListComplianceResults(ctx, device.ID)
	if err != nil {
		return nil, err
	}
	status := cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_UNSPECIFIED
	passing := int32(0)
	for _, check := range checks {
		if check.Compliant {
			passing++
		}
	}
	if len(checks) > 0 {
		status = cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_NON_COMPLIANT
		if passing == int32(len(checks)) {
			status = cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_COMPLIANT
		}
	}
	mapped := &cadestrov1.Device{
		Id: &cadestrov1.DeviceId{Value: device.ID}, Hostname: device.Hostname, AgentVersion: device.AgentVersion,
		Status: service.deviceStatus(device), RegisteredAt: timestamppb.New(device.RegisteredAt),
		CertExpiresAt: timestamppb.New(device.CertExpiresAt), ComplianceStatus: status,
		ComplianceTotal: int32(len(checks)), CompliancePassing: passing,
	}
	if device.LastSeenAt != nil {
		mapped.LastSeenAt = timestamppb.New(*device.LastSeenAt)
	}
	return mapped, nil
}

func (service *Service) ListDevices(ctx context.Context, request *connect.Request[cadestrov1.ListDevicesRequest]) (*connect.Response[cadestrov1.ListDevicesResponse], error) {
	limit := pageSize(request.Msg.GetPageSize())
	devices, err := service.store.Queries().ListDevices(ctx, db.ListDevicesParams{ID: request.Msg.GetPageToken(), Limit: limit})
	if err != nil {
		return nil, service.internal("list devices", err)
	}
	total, err := service.store.Queries().CountDevices(ctx)
	if err != nil {
		return nil, service.internal("count devices", err)
	}
	response := &cadestrov1.ListDevicesResponse{TotalCount: int32(total), NextPageToken: nextPageToken(devices, limit, func(device *db.Device) string { return device.ID })}
	for _, device := range devices {
		mapped, err := service.deviceProto(ctx, device)
		if err != nil {
			return nil, service.internal("map device", err)
		}
		response.Devices = append(response.Devices, mapped)
	}
	return connect.NewResponse(response), nil
}

func (service *Service) GetDevice(ctx context.Context, request *connect.Request[cadestrov1.GetDeviceRequest]) (*connect.Response[cadestrov1.GetDeviceResponse], error) {
	device, err := service.store.Queries().GetDevice(ctx, request.Msg.GetId().GetValue())
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("device")
		}
		return nil, service.internal("get device", err)
	}
	mapped, err := service.deviceProto(ctx, device)
	if err != nil {
		return nil, service.internal("map device", err)
	}
	return connect.NewResponse(&cadestrov1.GetDeviceResponse{Device: mapped}), nil
}

func (service *Service) DeleteDevice(ctx context.Context, request *connect.Request[cadestrov1.DeleteDeviceRequest]) (*connect.Response[cadestrov1.DeleteDeviceResponse], error) {
	id := request.Msg.GetId().GetValue()
	rows, err := service.store.Queries().DeleteDevice(ctx, id)
	if err != nil {
		return nil, service.internal("delete device", err)
	}
	if rows == 0 {
		return nil, rpcNotFound("device")
	}
	if err := service.audit(ctx, "device.deleted", "device", id, "user", ""); err != nil {
		return nil, service.internal("audit device deletion", err)
	}
	return connect.NewResponse(&cadestrov1.DeleteDeviceResponse{}), nil
}

func registrationToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	value := base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256([]byte(value))
	return value, hex.EncodeToString(digest[:]), nil
}

func (service *Service) CreateToken(ctx context.Context, request *connect.Request[cadestrov1.CreateTokenRequest]) (*connect.Response[cadestrov1.CreateTokenResponse], error) {
	if request.Msg.GetExpiresAt() == nil || !request.Msg.GetExpiresAt().AsTime().After(service.now()) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("token expiration must be in the future"))
	}
	value, digest, err := registrationToken()
	if err != nil {
		return nil, service.internal("generate registration token", err)
	}
	token, err := service.store.Queries().CreateRegistrationToken(ctx, db.CreateRegistrationTokenParams{
		ID: ulid.Make().String(), ValueHash: digest, Name: request.Msg.GetName(), MaxUses: int64(request.Msg.GetMaxUses()),
		ExpiresAt: request.Msg.GetExpiresAt().AsTime(), CreatedAt: service.now().UTC(),
	})
	if err != nil {
		return nil, service.internal("create registration token", err)
	}
	if err := service.audit(ctx, "registration_token.created", "registration_token", token.ID, "user", ""); err != nil {
		return nil, service.internal("audit registration token creation", err)
	}
	mapped := registrationTokenProto(token)
	mapped.Value = value
	return connect.NewResponse(&cadestrov1.CreateTokenResponse{Token: mapped, CaFingerprintPin: service.caFingerprint}), nil
}

func (service *Service) ListTokens(ctx context.Context, request *connect.Request[cadestrov1.ListTokensRequest]) (*connect.Response[cadestrov1.ListTokensResponse], error) {
	limit := pageSize(request.Msg.GetPageSize())
	tokens, err := service.store.Queries().ListRegistrationTokens(ctx, db.ListRegistrationTokensParams{
		AfterID: request.Msg.GetPageToken(), PageLimit: limit,
	})
	if err != nil {
		return nil, service.internal("list registration tokens", err)
	}
	total, err := service.store.Queries().CountRegistrationTokens(ctx)
	if err != nil {
		return nil, service.internal("count registration tokens", err)
	}
	response := &cadestrov1.ListTokensResponse{TotalCount: int32(total), NextPageToken: nextPageToken(tokens, limit, func(token *db.RegistrationToken) string { return token.ID })}
	for _, token := range tokens {
		response.Tokens = append(response.Tokens, registrationTokenProto(token))
	}
	return connect.NewResponse(response), nil
}

func (service *Service) RenameToken(ctx context.Context, request *connect.Request[cadestrov1.RenameTokenRequest]) (*connect.Response[cadestrov1.UpdateTokenResponse], error) {
	token, err := service.store.Queries().RenameRegistrationToken(ctx, db.RenameRegistrationTokenParams{Name: request.Msg.GetName(), ID: request.Msg.GetId().GetValue()})
	return service.tokenUpdateResponse(ctx, "rename registration token", token, err)
}

func (service *Service) tokenUpdateResponse(ctx context.Context, operation string, token *db.RegistrationToken, err error) (*connect.Response[cadestrov1.UpdateTokenResponse], error) {
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("registration token")
		}
		return nil, service.internal(operation, err)
	}
	if err := service.audit(ctx, "registration_token.updated", "registration_token", token.ID, "user", ""); err != nil {
		return nil, service.internal("audit registration token update", err)
	}
	return connect.NewResponse(&cadestrov1.UpdateTokenResponse{Token: registrationTokenProto(token)}), nil
}

func (service *Service) DeleteToken(ctx context.Context, request *connect.Request[cadestrov1.DeleteTokenRequest]) (*connect.Response[cadestrov1.DeleteTokenResponse], error) {
	id := request.Msg.GetId().GetValue()
	rows, err := service.store.Queries().DeleteRegistrationToken(ctx, id)
	if err != nil {
		return nil, service.internal("delete registration token", err)
	}
	if rows == 0 {
		return nil, rpcNotFound("registration token")
	}
	if err := service.audit(ctx, "registration_token.deleted", "registration_token", id, "user", ""); err != nil {
		return nil, service.internal("audit registration token deletion", err)
	}
	return connect.NewResponse(&cadestrov1.DeleteTokenResponse{}), nil
}

func groupProto(id, name, description string, memberCount int64, createdAt time.Time) *cadestrov1.DeviceGroup {
	return &cadestrov1.DeviceGroup{
		Id: &cadestrov1.DeviceGroupId{Value: id}, Name: name, Description: description,
		MemberCount: int32(memberCount), CreatedAt: timestamppb.New(createdAt),
	}
}

func (service *Service) CreateDeviceGroup(ctx context.Context, request *connect.Request[cadestrov1.CreateDeviceGroupRequest]) (*connect.Response[cadestrov1.CreateDeviceGroupResponse], error) {
	group, err := service.store.Queries().CreateDeviceGroup(ctx, db.CreateDeviceGroupParams{ID: ulid.Make().String(), Name: request.Msg.GetName(), Description: request.Msg.GetDescription(), CreatedAt: service.now().UTC()})
	if err != nil {
		if store.IsConflict(err) {
			return nil, rpcConflict("device group")
		}
		return nil, service.internal("create device group", err)
	}
	if err := service.audit(ctx, "device_group.created", "device_group", group.ID, "user", ""); err != nil {
		return nil, service.internal("audit device group creation", err)
	}
	return connect.NewResponse(&cadestrov1.CreateDeviceGroupResponse{Group: groupProto(group.ID, group.Name, group.Description, 0, group.CreatedAt)}), nil
}

func (service *Service) GetDeviceGroup(ctx context.Context, request *connect.Request[cadestrov1.GetDeviceGroupRequest]) (*connect.Response[cadestrov1.GetDeviceGroupResponse], error) {
	id := request.Msg.GetId().GetValue()
	group, err := service.store.Queries().GetDeviceGroup(ctx, id)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("device group")
		}
		return nil, service.internal("get device group", err)
	}
	members, err := service.store.Queries().ListDeviceGroupMembers(ctx, id)
	if err != nil {
		return nil, service.internal("list device group members", err)
	}
	response := &cadestrov1.GetDeviceGroupResponse{Group: groupProto(group.ID, group.Name, group.Description, group.MemberCount, group.CreatedAt)}
	for _, member := range members {
		mapped := &cadestrov1.DeviceGroupMember{DeviceId: &cadestrov1.DeviceId{Value: member.ID}, Hostname: member.Hostname, AgentVersion: member.AgentVersion}
		if member.LastSeenAt != nil {
			mapped.LastSeenAt = timestamppb.New(*member.LastSeenAt)
		}
		response.Devices = append(response.Devices, mapped)
	}
	return connect.NewResponse(response), nil
}

func (service *Service) ListDeviceGroups(ctx context.Context, request *connect.Request[cadestrov1.ListDeviceGroupsRequest]) (*connect.Response[cadestrov1.ListDeviceGroupsResponse], error) {
	limit := pageSize(request.Msg.GetPageSize())
	groups, err := service.store.Queries().ListDeviceGroups(ctx, db.ListDeviceGroupsParams{ID: request.Msg.GetPageToken(), Limit: limit})
	if err != nil {
		return nil, service.internal("list device groups", err)
	}
	total, err := service.store.Queries().CountDeviceGroups(ctx)
	if err != nil {
		return nil, service.internal("count device groups", err)
	}
	response := &cadestrov1.ListDeviceGroupsResponse{TotalCount: int32(total), NextPageToken: nextPageToken(groups, limit, func(group *db.ListDeviceGroupsRow) string { return group.ID })}
	for _, group := range groups {
		response.Groups = append(response.Groups, groupProto(group.ID, group.Name, group.Description, group.MemberCount, group.CreatedAt))
	}
	return connect.NewResponse(response), nil
}

func (service *Service) ListDeviceGroupsForDevice(ctx context.Context, request *connect.Request[cadestrov1.ListDeviceGroupsForDeviceRequest]) (*connect.Response[cadestrov1.ListDeviceGroupsForDeviceResponse], error) {
	groups, err := service.store.Queries().ListDeviceGroupsForDevice(ctx, request.Msg.GetDeviceId().GetValue())
	if err != nil {
		return nil, service.internal("list groups for device", err)
	}
	response := &cadestrov1.ListDeviceGroupsForDeviceResponse{}
	for _, group := range groups {
		response.Groups = append(response.Groups, groupProto(group.ID, group.Name, group.Description, group.MemberCount, group.CreatedAt))
	}
	return connect.NewResponse(response), nil
}

func (service *Service) RenameDeviceGroup(ctx context.Context, request *connect.Request[cadestrov1.RenameDeviceGroupRequest]) (*connect.Response[cadestrov1.UpdateDeviceGroupResponse], error) {
	group, err := service.store.Queries().RenameDeviceGroup(ctx, db.RenameDeviceGroupParams{Name: request.Msg.GetName(), ID: request.Msg.GetId().GetValue()})
	return service.groupUpdateResponse(ctx, "rename device group", group, err)
}

func (service *Service) UpdateDeviceGroupDescription(ctx context.Context, request *connect.Request[cadestrov1.UpdateDeviceGroupDescriptionRequest]) (*connect.Response[cadestrov1.UpdateDeviceGroupResponse], error) {
	group, err := service.store.Queries().UpdateDeviceGroupDescription(ctx, db.UpdateDeviceGroupDescriptionParams{Description: request.Msg.GetDescription(), ID: request.Msg.GetId().GetValue()})
	return service.groupUpdateResponse(ctx, "update device group description", group, err)
}

func (service *Service) groupUpdateResponse(ctx context.Context, operation string, group *db.DeviceGroup, err error) (*connect.Response[cadestrov1.UpdateDeviceGroupResponse], error) {
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("device group")
		}
		if store.IsConflict(err) {
			return nil, rpcConflict("device group")
		}
		return nil, service.internal(operation, err)
	}
	current, err := service.store.Queries().GetDeviceGroup(ctx, group.ID)
	if err != nil {
		return nil, service.internal("get updated device group", err)
	}
	if err := service.audit(ctx, "device_group.updated", "device_group", group.ID, "user", ""); err != nil {
		return nil, service.internal("audit device group update", err)
	}
	return connect.NewResponse(&cadestrov1.UpdateDeviceGroupResponse{Group: groupProto(current.ID, current.Name, current.Description, current.MemberCount, current.CreatedAt)}), nil
}

func (service *Service) DeleteDeviceGroup(ctx context.Context, request *connect.Request[cadestrov1.DeleteDeviceGroupRequest]) (*connect.Response[cadestrov1.DeleteDeviceGroupResponse], error) {
	id := request.Msg.GetId().GetValue()
	rows, err := service.store.Queries().DeleteDeviceGroup(ctx, id)
	if err != nil {
		return nil, service.internal("delete device group", err)
	}
	if rows == 0 {
		return nil, rpcNotFound("device group")
	}
	if err := service.audit(ctx, "device_group.deleted", "device_group", id, "user", ""); err != nil {
		return nil, service.internal("audit device group deletion", err)
	}
	return connect.NewResponse(&cadestrov1.DeleteDeviceGroupResponse{}), nil
}

func (service *Service) requireDeviceAndGroup(ctx context.Context, deviceID, groupID string) error {
	if _, err := service.store.Queries().GetDeviceGroup(ctx, groupID); err != nil {
		if store.IsNotFound(err) {
			return rpcNotFound("device group")
		}
		return service.internal("get device group", err)
	}
	if _, err := service.store.Queries().GetDevice(ctx, deviceID); err != nil {
		if store.IsNotFound(err) {
			return rpcNotFound("device")
		}
		return service.internal("get device", err)
	}
	return nil
}

func (service *Service) AddDeviceToGroup(ctx context.Context, request *connect.Request[cadestrov1.AddDeviceToGroupRequest]) (*connect.Response[cadestrov1.AddDeviceToGroupResponse], error) {
	groupID := request.Msg.GetGroupId().GetValue()
	deviceID := request.Msg.GetDeviceId().GetValue()
	if err := service.requireDeviceAndGroup(ctx, deviceID, groupID); err != nil {
		return nil, err
	}
	if err := service.store.Queries().AddDeviceToGroup(ctx, db.AddDeviceToGroupParams{GroupID: groupID, DeviceID: deviceID}); err != nil {
		if store.IsConflict(err) {
			return nil, rpcConflict("device group member")
		}
		return nil, service.internal("add device to group", err)
	}
	group, err := service.store.Queries().GetDeviceGroup(ctx, groupID)
	if err != nil {
		return nil, service.internal("get updated device group", err)
	}
	if err := service.audit(ctx, "device_group.member_added", "device_group", groupID, "user", ""); err != nil {
		return nil, service.internal("audit device group membership", err)
	}
	return connect.NewResponse(&cadestrov1.AddDeviceToGroupResponse{Group: groupProto(group.ID, group.Name, group.Description, group.MemberCount, group.CreatedAt)}), nil
}

func (service *Service) RemoveDeviceFromGroup(ctx context.Context, request *connect.Request[cadestrov1.RemoveDeviceFromGroupRequest]) (*connect.Response[cadestrov1.RemoveDeviceFromGroupResponse], error) {
	groupID := request.Msg.GetGroupId().GetValue()
	rows, err := service.store.Queries().RemoveDeviceFromGroup(ctx, db.RemoveDeviceFromGroupParams{GroupID: groupID, DeviceID: request.Msg.GetDeviceId().GetValue()})
	if err != nil {
		return nil, service.internal("remove device from group", err)
	}
	if rows == 0 {
		return nil, rpcNotFound("device group member")
	}
	group, err := service.store.Queries().GetDeviceGroup(ctx, groupID)
	if err != nil {
		return nil, service.internal("get updated device group", err)
	}
	if err := service.audit(ctx, "device_group.member_removed", "device_group", groupID, "user", ""); err != nil {
		return nil, service.internal("audit device group membership", err)
	}
	return connect.NewResponse(&cadestrov1.RemoveDeviceFromGroupResponse{Group: groupProto(group.ID, group.Name, group.Description, group.MemberCount, group.CreatedAt)}), nil
}

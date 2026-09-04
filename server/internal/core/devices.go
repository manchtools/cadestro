package core

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
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
	compliance, err := service.deviceCompliance(ctx, device.ID)
	if err != nil {
		return nil, err
	}
	mapped := &cadestrov1.Device{
		Id: &cadestrov1.DeviceId{Value: device.ID}, Hostname: device.Hostname, AgentVersion: device.AgentVersion,
		Status: service.deviceStatus(device), RegisteredAt: timestamppb.New(device.RegisteredAt),
		CertExpiresAt: timestamppb.New(device.CertExpiresAt), ComplianceStatus: compliance.status,
		ComplianceTotal: int32(len(compliance.checks)), CompliancePassing: compliance.passing,
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
	err := service.store.Transaction(ctx, func(queries *db.Queries) error {
		if err := queries.DeleteAssignmentsForTarget(ctx, db.DeleteAssignmentsForTargetParams{TargetType: cadestrov1.AssignmentTargetType_ASSIGNMENT_TARGET_TYPE_DEVICE, TargetID: id}); err != nil {
			return err
		}
		rows, err := queries.DeleteDevice(ctx, id)
		if err != nil {
			return err
		}
		if rows != 1 {
			return sql.ErrNoRows
		}
		return service.audit(ctx, queries, cadestrov1.AuditEventType_AUDIT_EVENT_TYPE_DEVICE_DELETED, cadestrov1.AuditStreamType_AUDIT_STREAM_TYPE_DEVICE, id, cadestrov1.AuditActorType_AUDIT_ACTOR_TYPE_USER, "")
	})
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("device")
		}
		return nil, service.internal("delete device", err)
	}
	return connect.NewResponse(&cadestrov1.DeleteDeviceResponse{}), nil
}

func registrationToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	value := base64.RawURLEncoding.EncodeToString(raw)
	return value, tokenHash(value), nil
}

func (service *Service) CreateToken(ctx context.Context, request *connect.Request[cadestrov1.CreateTokenRequest]) (*connect.Response[cadestrov1.CreateTokenResponse], error) {
	if request.Msg.GetExpiresAt() == nil || !request.Msg.GetExpiresAt().AsTime().After(service.now()) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("token expiration must be in the future"))
	}
	value, digest, err := registrationToken()
	if err != nil {
		return nil, service.internal("generate registration token", err)
	}
	var mapped *cadestrov1.RegistrationToken
	err = service.store.Transaction(ctx, func(queries *db.Queries) error {
		token, err := queries.CreateRegistrationToken(ctx, db.CreateRegistrationTokenParams{
			ID: ulid.Make().String(), ValueHash: digest, Name: request.Msg.GetName(), MaxUses: request.Msg.GetMaxUses(),
			ExpiresAt: request.Msg.GetExpiresAt().AsTime(),
		})
		if err != nil {
			return err
		}
		if err := service.audit(ctx, queries, cadestrov1.AuditEventType_AUDIT_EVENT_TYPE_REGISTRATION_TOKEN_CREATED, cadestrov1.AuditStreamType_AUDIT_STREAM_TYPE_REGISTRATION_TOKEN, token.ID, cadestrov1.AuditActorType_AUDIT_ACTOR_TYPE_USER, ""); err != nil {
			return err
		}
		mapped = registrationTokenProto(token)
		mapped.Value = value
		return nil
	})
	if err != nil {
		return nil, service.internal("create registration token", err)
	}
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

func (service *Service) RenameToken(ctx context.Context, request *connect.Request[cadestrov1.RenameTokenRequest]) (*connect.Response[cadestrov1.RenameTokenResponse], error) {
	var mapped *cadestrov1.RegistrationToken
	err := service.store.Transaction(ctx, func(queries *db.Queries) error {
		token, err := queries.RenameRegistrationToken(ctx, db.RenameRegistrationTokenParams{Name: request.Msg.GetName(), ID: request.Msg.GetId().GetValue()})
		if err != nil {
			return err
		}
		if err := service.audit(ctx, queries, cadestrov1.AuditEventType_AUDIT_EVENT_TYPE_REGISTRATION_TOKEN_UPDATED, cadestrov1.AuditStreamType_AUDIT_STREAM_TYPE_REGISTRATION_TOKEN, token.ID, cadestrov1.AuditActorType_AUDIT_ACTOR_TYPE_USER, ""); err != nil {
			return err
		}
		mapped = registrationTokenProto(token)
		return nil
	})
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("registration token")
		}
		return nil, service.internal("rename registration token", err)
	}
	return connect.NewResponse(&cadestrov1.RenameTokenResponse{Token: mapped}), nil
}

func (service *Service) DeleteToken(ctx context.Context, request *connect.Request[cadestrov1.DeleteTokenRequest]) (*connect.Response[cadestrov1.DeleteTokenResponse], error) {
	id := request.Msg.GetId().GetValue()
	err := service.store.Transaction(ctx, func(queries *db.Queries) error {
		rows, err := queries.DeleteRegistrationToken(ctx, id)
		if err != nil {
			return err
		}
		if rows != 1 {
			return sql.ErrNoRows
		}
		return service.audit(ctx, queries, cadestrov1.AuditEventType_AUDIT_EVENT_TYPE_REGISTRATION_TOKEN_DELETED, cadestrov1.AuditStreamType_AUDIT_STREAM_TYPE_REGISTRATION_TOKEN, id, cadestrov1.AuditActorType_AUDIT_ACTOR_TYPE_USER, "")
	})
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("registration token")
		}
		return nil, service.internal("delete registration token", err)
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
	var mapped *cadestrov1.DeviceGroup
	err := service.store.Transaction(ctx, func(queries *db.Queries) error {
		group, err := queries.CreateDeviceGroup(ctx, db.CreateDeviceGroupParams{ID: ulid.Make().String(), Name: request.Msg.GetName(), Description: request.Msg.GetDescription()})
		if err != nil {
			return err
		}
		if err := service.audit(ctx, queries, cadestrov1.AuditEventType_AUDIT_EVENT_TYPE_DEVICE_GROUP_CREATED, cadestrov1.AuditStreamType_AUDIT_STREAM_TYPE_DEVICE_GROUP, group.ID, cadestrov1.AuditActorType_AUDIT_ACTOR_TYPE_USER, ""); err != nil {
			return err
		}
		mapped = groupProto(group.ID, group.Name, group.Description, 0, group.CreatedAt)
		return nil
	})
	if err != nil {
		if store.IsConflict(err) {
			return nil, rpcConflict("device group")
		}
		return nil, service.internal("create device group", err)
	}
	return connect.NewResponse(&cadestrov1.CreateDeviceGroupResponse{Group: mapped}), nil
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

func (service *Service) RenameDeviceGroup(ctx context.Context, request *connect.Request[cadestrov1.RenameDeviceGroupRequest]) (*connect.Response[cadestrov1.RenameDeviceGroupResponse], error) {
	id := request.Msg.GetId().GetValue()
	current, err := service.groupMutation(ctx, id, "rename device group", func(queries *db.Queries) error {
		_, err := queries.RenameDeviceGroup(ctx, db.RenameDeviceGroupParams{Name: request.Msg.GetName(), ID: id})
		return err
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.RenameDeviceGroupResponse{Group: current}), nil
}

func (service *Service) SetDeviceGroupDescription(ctx context.Context, request *connect.Request[cadestrov1.SetDeviceGroupDescriptionRequest]) (*connect.Response[cadestrov1.SetDeviceGroupDescriptionResponse], error) {
	id := request.Msg.GetId().GetValue()
	current, err := service.groupMutation(ctx, id, "set device group description", func(queries *db.Queries) error {
		_, err := queries.SetDeviceGroupDescription(ctx, db.SetDeviceGroupDescriptionParams{Description: request.Msg.GetDescription(), ID: id})
		return err
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.SetDeviceGroupDescriptionResponse{Group: current}), nil
}

func (service *Service) groupMutation(ctx context.Context, id, operation string, mutate func(*db.Queries) error) (*cadestrov1.DeviceGroup, error) {
	var mapped *cadestrov1.DeviceGroup
	err := service.store.Transaction(ctx, func(queries *db.Queries) error {
		if err := mutate(queries); err != nil {
			return err
		}
		group, err := queries.GetDeviceGroup(ctx, id)
		if err != nil {
			return err
		}
		if err := service.audit(ctx, queries, cadestrov1.AuditEventType_AUDIT_EVENT_TYPE_DEVICE_GROUP_UPDATED, cadestrov1.AuditStreamType_AUDIT_STREAM_TYPE_DEVICE_GROUP, id, cadestrov1.AuditActorType_AUDIT_ACTOR_TYPE_USER, ""); err != nil {
			return err
		}
		mapped = groupProto(group.ID, group.Name, group.Description, group.MemberCount, group.CreatedAt)
		return nil
	})
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("device group")
		}
		if store.IsConflict(err) {
			return nil, rpcConflict("device group")
		}
		return nil, service.internal(operation, err)
	}
	return mapped, nil
}

func (service *Service) DeleteDeviceGroup(ctx context.Context, request *connect.Request[cadestrov1.DeleteDeviceGroupRequest]) (*connect.Response[cadestrov1.DeleteDeviceGroupResponse], error) {
	id := request.Msg.GetId().GetValue()
	err := service.store.Transaction(ctx, func(queries *db.Queries) error {
		if err := queries.DeleteAssignmentsForTarget(ctx, db.DeleteAssignmentsForTargetParams{TargetType: cadestrov1.AssignmentTargetType_ASSIGNMENT_TARGET_TYPE_DEVICE_GROUP, TargetID: id}); err != nil {
			return err
		}
		rows, err := queries.DeleteDeviceGroup(ctx, id)
		if err != nil {
			return err
		}
		if rows != 1 {
			return sql.ErrNoRows
		}
		return service.audit(ctx, queries, cadestrov1.AuditEventType_AUDIT_EVENT_TYPE_DEVICE_GROUP_DELETED, cadestrov1.AuditStreamType_AUDIT_STREAM_TYPE_DEVICE_GROUP, id, cadestrov1.AuditActorType_AUDIT_ACTOR_TYPE_USER, "")
	})
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("device group")
		}
		return nil, service.internal("delete device group", err)
	}
	return connect.NewResponse(&cadestrov1.DeleteDeviceGroupResponse{}), nil
}

func (service *Service) requireDeviceAndGroup(ctx context.Context, queries *db.Queries, deviceID, groupID string) error {
	if _, err := queries.GetDeviceGroup(ctx, groupID); err != nil {
		if store.IsNotFound(err) {
			return rpcNotFound("device group")
		}
		return service.internal("get device group", err)
	}
	if _, err := queries.GetDevice(ctx, deviceID); err != nil {
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
	var mapped *cadestrov1.DeviceGroup
	err := service.store.Transaction(ctx, func(queries *db.Queries) error {
		if err := service.requireDeviceAndGroup(ctx, queries, deviceID, groupID); err != nil {
			return err
		}
		if err := queries.AddDeviceToGroup(ctx, db.AddDeviceToGroupParams{GroupID: groupID, DeviceID: deviceID}); err != nil {
			return err
		}
		if err := queries.TouchDeviceGroup(ctx, groupID); err != nil {
			return err
		}
		group, err := queries.GetDeviceGroup(ctx, groupID)
		if err != nil {
			return err
		}
		if err := service.audit(ctx, queries, cadestrov1.AuditEventType_AUDIT_EVENT_TYPE_DEVICE_GROUP_MEMBER_ADDED, cadestrov1.AuditStreamType_AUDIT_STREAM_TYPE_DEVICE_GROUP, groupID, cadestrov1.AuditActorType_AUDIT_ACTOR_TYPE_USER, ""); err != nil {
			return err
		}
		mapped = groupProto(group.ID, group.Name, group.Description, group.MemberCount, group.CreatedAt)
		return nil
	})
	if err != nil {
		var connectErr *connect.Error
		if errors.As(err, &connectErr) {
			return nil, connectErr
		}
		if store.IsConflict(err) {
			return nil, rpcConflict("device group member")
		}
		return nil, service.internal("add device to group", err)
	}
	return connect.NewResponse(&cadestrov1.AddDeviceToGroupResponse{Group: mapped}), nil
}

func (service *Service) RemoveDeviceFromGroup(ctx context.Context, request *connect.Request[cadestrov1.RemoveDeviceFromGroupRequest]) (*connect.Response[cadestrov1.RemoveDeviceFromGroupResponse], error) {
	groupID := request.Msg.GetGroupId().GetValue()
	var mapped *cadestrov1.DeviceGroup
	err := service.store.Transaction(ctx, func(queries *db.Queries) error {
		rows, err := queries.RemoveDeviceFromGroup(ctx, db.RemoveDeviceFromGroupParams{GroupID: groupID, DeviceID: request.Msg.GetDeviceId().GetValue()})
		if err != nil {
			return err
		}
		if rows != 1 {
			return sql.ErrNoRows
		}
		if err := queries.TouchDeviceGroup(ctx, groupID); err != nil {
			return err
		}
		group, err := queries.GetDeviceGroup(ctx, groupID)
		if err != nil {
			return err
		}
		if err := service.audit(ctx, queries, cadestrov1.AuditEventType_AUDIT_EVENT_TYPE_DEVICE_GROUP_MEMBER_REMOVED, cadestrov1.AuditStreamType_AUDIT_STREAM_TYPE_DEVICE_GROUP, groupID, cadestrov1.AuditActorType_AUDIT_ACTOR_TYPE_USER, ""); err != nil {
			return err
		}
		mapped = groupProto(group.ID, group.Name, group.Description, group.MemberCount, group.CreatedAt)
		return nil
	})
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("device group member")
		}
		return nil, service.internal("remove device from group", err)
	}
	return connect.NewResponse(&cadestrov1.RemoveDeviceFromGroupResponse{Group: mapped}), nil
}

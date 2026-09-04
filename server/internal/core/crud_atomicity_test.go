package core

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/ca"
	"github.com/manchtools/cadestro/server/internal/mtls"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

func TestDeletingTargetsRemovesTheirAssignments(t *testing.T) {
	service, ctx, now, _ := testService(t)
	action := createResultTestAction(t, service, "01K00000000000000000000081", "target cleanup", "true", false)
	deviceID := "01K00000000000000000000082"
	groupID := "01K00000000000000000000083"
	createResultTestDevice(t, service, deviceID, now)
	_, err := service.store.Queries().CreateDeviceGroup(ctx, db.CreateDeviceGroupParams{ID: groupID, Name: "target cleanup"})
	require.NoError(t, err)
	assignments := []db.CreateAssignmentParams{
		{ID: "01K00000000000000000000084", ActionID: action.GetId().GetValue(), TargetType: cadestrov1.AssignmentTargetType_ASSIGNMENT_TARGET_TYPE_DEVICE, TargetID: deviceID},
		{ID: "01K00000000000000000000085", ActionID: action.GetId().GetValue(), TargetType: cadestrov1.AssignmentTargetType_ASSIGNMENT_TARGET_TYPE_DEVICE_GROUP, TargetID: groupID},
	}
	for _, assignment := range assignments {
		_, err := service.store.Queries().CreateAssignment(ctx, assignment)
		require.NoError(t, err)
	}
	_, err = service.DeleteDevice(ctx, connect.NewRequest(&cadestrov1.DeleteDeviceRequest{Id: &cadestrov1.DeviceId{Value: deviceID}}))
	require.NoError(t, err)
	_, err = service.DeleteDeviceGroup(ctx, connect.NewRequest(&cadestrov1.DeleteDeviceGroupRequest{Id: &cadestrov1.DeviceGroupId{Value: groupID}}))
	require.NoError(t, err)
	for _, assignment := range assignments {
		_, err := service.store.Queries().GetAssignment(ctx, assignment.ID)
		require.ErrorIs(t, err, sql.ErrNoRows)
	}
}

func TestMembershipMutationsAdvanceGroupTimestamp(t *testing.T) {
	service, ctx, now, _ := testService(t)
	deviceID := "01K00000000000000000000086"
	groupID := "01K00000000000000000000087"
	createResultTestDevice(t, service, deviceID, now)
	created, err := service.store.Queries().CreateDeviceGroup(ctx, db.CreateDeviceGroupParams{ID: groupID, Name: "membership timestamp"})
	require.NoError(t, err)
	time.Sleep(1100 * time.Millisecond)
	_, err = service.AddDeviceToGroup(ctx, connect.NewRequest(&cadestrov1.AddDeviceToGroupRequest{GroupId: &cadestrov1.DeviceGroupId{Value: groupID}, DeviceId: &cadestrov1.DeviceId{Value: deviceID}}))
	require.NoError(t, err)
	afterAdd, err := service.store.Queries().GetDeviceGroup(ctx, groupID)
	require.NoError(t, err)
	require.True(t, afterAdd.UpdatedAt.After(created.UpdatedAt))
	time.Sleep(1100 * time.Millisecond)
	_, err = service.RemoveDeviceFromGroup(ctx, connect.NewRequest(&cadestrov1.RemoveDeviceFromGroupRequest{GroupId: &cadestrov1.DeviceGroupId{Value: groupID}, DeviceId: &cadestrov1.DeviceId{Value: deviceID}}))
	require.NoError(t, err)
	afterRemove, err := service.store.Queries().GetDeviceGroup(ctx, groupID)
	require.NoError(t, err)
	require.True(t, afterRemove.UpdatedAt.After(afterAdd.UpdatedAt))
}

func TestAbsentRoleRelationsReturnNotFound(t *testing.T) {
	service, ctx, _, _ := testService(t)
	_, err := service.GrantRolePermission(ctx, connect.NewRequest(&cadestrov1.GrantRolePermissionRequest{Id: &cadestrov1.RoleId{Value: "01K00000000000000000000091"}, Permission: cadestrov1.Permission_PERMISSION_GET_CURRENT_USER}))
	require.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	_, err = service.AssignRoleToUser(ctx, connect.NewRequest(&cadestrov1.AssignRoleToUserRequest{UserId: &cadestrov1.UserId{Value: "01K00000000000000000000092"}, RoleId: &cadestrov1.RoleId{Value: administratorsRoleID}}))
	require.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestCreateAssignmentRequiresExistingActionAndTarget(t *testing.T) {
	service, ctx, _, _ := testService(t)
	missingAction := &cadestrov1.ActionId{Value: "01K00000000000000000000095"}
	missingDevice := &cadestrov1.AssignmentTargetId{Value: "01K00000000000000000000096"}
	_, err := service.CreateAssignment(ctx, connect.NewRequest(&cadestrov1.CreateAssignmentRequest{
		ActionId: missingAction, TargetType: cadestrov1.AssignmentTargetType_ASSIGNMENT_TARGET_TYPE_DEVICE, TargetId: missingDevice,
	}))
	require.Equal(t, connect.CodeNotFound, connect.CodeOf(err))

	action := createResultTestAction(t, service, "01K00000000000000000000097", "assignment validation", "true", false)
	_, err = service.CreateAssignment(ctx, connect.NewRequest(&cadestrov1.CreateAssignmentRequest{
		ActionId: action.GetId(), TargetType: cadestrov1.AssignmentTargetType_ASSIGNMENT_TARGET_TYPE_DEVICE, TargetId: missingDevice,
	}))
	require.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	assignments, err := service.store.Queries().ListAssignments(ctx, db.ListAssignmentsParams{})
	require.NoError(t, err)
	require.Empty(t, assignments)
}

func TestRenameActionRollsBackWhenResponseProjectionFails(t *testing.T) {
	service, ctx, _, _ := testService(t)
	action := createResultTestAction(t, service, "01K00000000000000000000098", "original", "true", false)
	_, err := service.store.Queries().ConfigureAction(ctx, db.ConfigureActionParams{ActionBlob: []byte{0xff}, ID: action.GetId().GetValue()})
	require.NoError(t, err)
	_, err = service.RenameAction(ctx, connect.NewRequest(&cadestrov1.RenameActionRequest{
		Id: action.GetId(), Name: "changed",
	}))
	require.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	stored, err := service.store.Queries().GetAction(ctx, action.GetId().GetValue())
	require.NoError(t, err)
	require.Equal(t, "original", stored.Name)
	events, err := service.store.Queries().ListAuditEvents(ctx, db.ListAuditEventsParams{ID: "~", Limit: 10})
	require.NoError(t, err)
	require.Empty(t, events)
}

func TestConcurrentRenewCertificateKeepsSinglePendingCertificate(t *testing.T) {
	service, ctx, now, _ := testService(t)
	service.ca = testEnrollmentCA(t, now)
	deviceID := "01K00000000000000000000093"
	csr, peer := renewalIdentity(t, service, deviceID, now)
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	runCtx = mtls.WithPeerCertificate(mtls.WithDeviceID(runCtx, deviceID), peer)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseCalls := func() { releaseOnce.Do(func() { close(release) }) }
	workersDone := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(2)
	t.Cleanup(func() {
		releaseCalls()
		cancel()
		select {
		case <-workersDone:
		case <-time.After(time.Second):
			t.Error("certificate renewal workers did not stop")
		}
	})
	ca.WithClock(func() time.Time {
		select {
		case entered <- struct{}{}:
		case <-runCtx.Done():
			return now
		}
		select {
		case <-release:
		case <-runCtx.Done():
		}
		return now
	})(service.ca)

	type renewalOutcome struct {
		response *connect.Response[cadestrov1.RenewCertificateResponse]
		err      error
	}
	outcomes := make(chan renewalOutcome, 2)
	for range 2 {
		go func() {
			defer workers.Done()
			response, err := service.RenewCertificate(runCtx, connect.NewRequest(&cadestrov1.RenewCertificateRequest{Csr: csr}))
			outcomes <- renewalOutcome{response: response, err: err}
		}()
	}
	go func() {
		workers.Wait()
		close(workersDone)
	}()
	for range 2 {
		select {
		case <-entered:
		case <-runCtx.Done():
			t.Fatal("certificate renewal calls did not reach the signing barrier")
		}
	}
	releaseCalls()

	certificates := make([][]byte, 0, 3)
	for range 2 {
		select {
		case outcome := <-outcomes:
			if outcome.err != nil {
				require.Equal(t, connect.CodeInternal, connect.CodeOf(outcome.err))
				continue
			}
			certificates = append(certificates, outcome.response.Msg.GetCertificate())
		case <-runCtx.Done():
			t.Fatal("certificate renewal calls did not finish")
		}
	}
	require.NotEmpty(t, certificates)
	retry, err := service.RenewCertificate(runCtx, connect.NewRequest(&cadestrov1.RenewCertificateRequest{Csr: csr}))
	require.NoError(t, err)
	certificates = append(certificates, retry.Msg.GetCertificate())
	for _, certificate := range certificates[1:] {
		require.Equal(t, certificates[0], certificate)
	}
	events, err := service.store.Queries().ListAuditEvents(runCtx, db.ListAuditEventsParams{ID: "~", Limit: 10})
	require.NoError(t, err)
	require.Len(t, events, 1)
}

func TestRenewCertificateMapsStorageFailureToInternal(t *testing.T) {
	service, ctx, now, _ := testService(t)
	service.ca = testEnrollmentCA(t, now)
	deviceID := "01K00000000000000000000094"
	csr, peer := renewalIdentity(t, service, deviceID, now)
	ctx = mtls.WithPeerCertificate(mtls.WithDeviceID(ctx, deviceID), peer)
	require.NoError(t, service.store.Close())
	_, err := service.RenewCertificate(ctx, connect.NewRequest(&cadestrov1.RenewCertificateRequest{Csr: csr}))
	require.Equal(t, connect.CodeInternal, connect.CodeOf(err))
}

func renewalIdentity(t *testing.T, service *Service, deviceID string, now time.Time) ([]byte, *x509.Certificate) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	requestDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "agent"}}, privateKey)
	require.NoError(t, err)
	csr := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: requestDER})
	issued, err := service.ca.IssueCertificateFromCSR(deviceID, csr)
	require.NoError(t, err)
	block, _ := pem.Decode(issued.CertPEM)
	require.NotNil(t, block)
	peer, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	serial, err := ca.SerialFromCert(peer)
	require.NoError(t, err)
	_, err = service.store.Queries().CreateDevice(context.Background(), db.CreateDeviceParams{ID: deviceID, Hostname: "host", AgentVersion: "test", IdentityPublicKey: publicKey, ActiveCertificatePem: issued.CertPEM, ActiveCertSerial: serial, CertExpiresAt: peer.NotAfter, RegisteredAt: now})
	require.NoError(t, err)
	return csr, peer
}

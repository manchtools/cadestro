package executor

import (
	"context"
	"errors"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

// copySecret keeps the plaintext lifetime explicit at the authenticated mTLS
// boundary. At-rest encryption belongs to the control sink.
func copySecret(value []byte) ([]byte, error) {
	if len(value) == 0 {
		return nil, errors.New("secret is empty")
	}
	return append([]byte(nil), value...), nil
}

func (e *Executor) executeLuksAction(ctx context.Context, params *pb.EncryptionParams, state pb.DesiredState, actionID string) (*pb.CommandOutput, bool, map[string]string, error) {
	if state == pb.DesiredState_DESIRED_STATE_ABSENT {
		return e.executeLuks(ctx, params, state, actionID, nil)
	}
	openPresharedKey := func() ([]byte, error) {
		return copySecret(params.GetPresharedKey())
	}
	return e.executeLuks(ctx, params, state, actionID, openPresharedKey)
}

func (e *Executor) executeWifiAction(ctx context.Context, params *pb.WifiParams, state pb.DesiredState, actionID string) (*pb.CommandOutput, bool, error) {
	if err := validateActionIDForFilesystem(actionID); err != nil {
		return nil, false, err
	}
	if state == pb.DesiredState_DESIRED_STATE_ABSENT {
		return e.executeWifi(ctx, params, state, actionID, "", "")
	}
	if params == nil {
		return nil, false, errors.New("wifi params required")
	}
	var psk, clientKey []byte
	var err error
	switch params.AuthType {
	case pb.WifiAuthType_WIFI_AUTH_TYPE_PSK:
		psk, err = copySecret(params.GetPsk())
	case pb.WifiAuthType_WIFI_AUTH_TYPE_EAP_TLS:
		clientKey, err = copySecret(params.GetClientKey())
	default:
		err = errors.New("unsupported WiFi authentication type")
	}
	if err != nil {
		return nil, false, err
	}
	defer clear(psk)
	defer clear(clientKey)
	return e.executeWifi(ctx, params, state, actionID, string(psk), string(clientKey))
}

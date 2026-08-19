package executor

import (
	"context"
	"errors"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

// sealToControl is intentionally a copy boundary, not a cryptographic
// envelope. AgentService runs only on the authenticated mTLS stream; at-rest
// encryption belongs to the control sink.
func (e *Executor) sealToControl(plaintext []byte, _ ...string) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, errors.New("secret is empty")
	}
	return append([]byte(nil), plaintext...), nil
}

func openFromControl(value []byte) ([]byte, error) {
	if len(value) == 0 {
		return nil, errors.New("opened secret is empty")
	}
	return append([]byte(nil), value...), nil
}

func (e *Executor) SealLuksPassphrase(_ string, passphrase string) ([]byte, error) {
	return e.sealToControl([]byte(passphrase))
}

func (e *Executor) OpenLuksPassphrase(_ string, value []byte) (string, error) {
	plaintext, err := openFromControl(value)
	if err != nil {
		return "", err
	}
	defer clear(plaintext)
	return string(plaintext), nil
}

func (e *Executor) executeSealedLuks(ctx context.Context, params *pb.EncryptionParams, state pb.DesiredState, actionID string) (*pb.CommandOutput, bool, map[string]string, error) {
	if state == pb.DesiredState_DESIRED_STATE_ABSENT {
		return e.executeLuks(ctx, params, state, actionID, nil)
	}
	openPresharedKey := func() ([]byte, error) {
		plaintext, err := openFromControl(params.GetPresharedKey())
		if err != nil {
			return nil, err
		}
		return plaintext, nil
	}
	return e.executeLuks(ctx, params, state, actionID, openPresharedKey)
}

func (e *Executor) executeSealedWifi(ctx context.Context, params *pb.WifiParams, state pb.DesiredState, actionID string) (*pb.CommandOutput, bool, error) {
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
		psk, err = openFromControl(params.GetPsk())
	case pb.WifiAuthType_WIFI_AUTH_TYPE_EAP_TLS:
		clientKey, err = openFromControl(params.GetClientKey())
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

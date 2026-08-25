package executor

import (
	"context"
	"fmt"
	"path/filepath"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/network"
)

func wifiConnectionName(actionID string) string {
	return "cadestro-wifi-" + actionID
}

func wifiCertPath(actionID string) string {
	return filepath.Join(network.CertBaseDir, actionID)
}

func (e *Executor) executeWifi(ctx context.Context, params *pb.WifiParams, state pb.DesiredState, actionID, psk, clientKey string) (*pb.CommandOutput, bool, error) {
	e.ensureDeps()
	if params == nil {
		return nil, false, fmt.Errorf("wifi params required")
	}

	if err := validateActionIDForFilesystem(actionID); err != nil {
		return nil, false, err
	}

	conName := wifiConnectionName(actionID)
	certDir := wifiCertPath(actionID)

	if state == pb.DesiredState_DESIRED_STATE_ABSENT {

		existed, existsErr := e.deps.network.ConnectionExists(ctx, conName)
		if existsErr != nil {
			e.logger.Warn("wifi ABSENT: ConnectionExists failed; conservatively reporting changed=true",
				"connection", conName, "error", existsErr)
			existed = true
		}
		if err := e.deps.network.Delete(ctx, conName, network.DeleteOptions{CertDir: certDir}); err != nil {
			return nil, false, fmt.Errorf("delete connection: %w", err)
		}
		stdout := fmt.Sprintf("connection %s already absent\n", conName)
		if existed {
			stdout = fmt.Sprintf("removed connection %s\n", conName)
		}
		return &pb.CommandOutput{
			ExitCode: 0,
			Stdout:   stdout,
		}, existed, nil
	}

	profile := network.Profile{
		Name:        conName,
		SSID:        params.Ssid,
		AuthType:    wifiAuthFromProto(params.AuthType),
		PSK:         sysexec.NewMultilineSecret(psk),
		CACert:      params.CaCert,
		ClientCert:  params.ClientCert,
		ClientKey:   sysexec.NewMultilineSecret(clientKey),
		Identity:    params.Identity,
		AutoConnect: params.AutoConnect,
		Hidden:      params.Hidden,
		Priority:    int(params.Priority),
		CertDir:     certDir,
	}

	changed, err := e.deps.network.Apply(ctx, profile)
	if err != nil {
		return nil, false, err
	}

	stdout := fmt.Sprintf("connection %s already configured correctly\n", conName)
	if changed {
		stdout = fmt.Sprintf("configured connection %s for SSID %s\n", conName, params.Ssid)
	}
	return &pb.CommandOutput{ExitCode: 0, Stdout: stdout}, changed, nil
}

func wifiAuthFromProto(t pb.WifiAuthType) network.AuthType {
	switch t {
	case pb.WifiAuthType_WIFI_AUTH_TYPE_PSK:
		return network.AuthPSK
	case pb.WifiAuthType_WIFI_AUTH_TYPE_EAP_TLS:
		return network.AuthEAPTLS
	}
	return 0
}

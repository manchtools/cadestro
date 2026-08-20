#!/usr/bin/env python3
import tempfile
import unittest
from pathlib import Path

import cutover_judge as judge


def write(root: Path, name: str, content: str) -> None:
    path = root / name
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


def clean_fixture(root: Path) -> None:
    write(root, "contract/proto/cadestro/v1/control.proto", """
service ControlService {
  rpc SyncDevice(SyncDeviceRequest) returns (SyncDeviceResponse);
  rpc RebootDevice(RebootDeviceRequest) returns (RebootDeviceResponse);
}
message SyncDeviceRequest {}
message SyncDeviceResponse {}
message RebootDeviceRequest {}
message RebootDeviceResponse {}
message Manifest {}
message ManifestDelivery {}
message ManifestResult {}
message SyncState { repeated ManifestDelivery deliveries = 1; }
""")
    write(root, "contract/proto/cadestro/v1/actions.proto", "enum ActionType { ACTION_TYPE_PACKAGE = 1; }\n")


class CutoverJudgeTest(unittest.TestCase):
    def test_clean_tree_passes_current_invariants(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            clean_fixture(root)
            result = judge.report(root)
            self.assertTrue(result["pass"], result)

    def test_retired_rpc_and_authored_values_fail(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            clean_fixture(root)
            write(root, "contract/proto/cadestro/v1/actions.proto", "enum ActionType {\n ACTION_TYPE_REBOOT = 500;\n ACTION_TYPE_SYNC = 501;\n}\n")
            write(root, "contract/proto/cadestro/v1/control.proto", "service ControlService {\n rpc DispatchInstantAction(A) returns (B);\n rpc DispatchAssignedActions(A) returns (B);\n}\n")
            metrics = judge.metric_matches(root)
            self.assertGreater(len(metrics["authored_sync_reboot_action_values"]), 0)
            self.assertGreater(len(metrics["legacy_live_control_rpc_apis"]), 0)
            self.assertFalse(judge.report(root)["pass"])

    def test_live_policy_push_and_coupling_fail(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            clean_fixture(root)
            write(root, "server/internal/device/live.go", """
package device
func (h *Handlers) SyncDevice() {
  h.router.SendAtEpoch(deviceID, epoch, &ServerMessage_ManifestDelivery{})
  h.scheduler.Schedule(run_at)
}
""")
            metrics = judge.metric_matches(root)
            self.assertGreater(len(metrics["live_policy_manifest_push_paths"]), 0)
            self.assertGreater(len(metrics["live_control_scheduler_coupling"]), 0)

    def test_pull_sync_allows_agent_sync_reads(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            clean_fixture(root)
            write(root, "server/internal/agentsync/service.go", """
package agentsync
func (s *Service) Sync() { return s.store.ListDueDeviceDeliveries(ctx) }
""")
            self.assertEqual(judge.live_control_coupling_matches(root), [])

    def test_protocol_residue_identity_sealing_and_cert_state_fail(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            clean_fixture(root)
            write(root, "contract/proto/cadestro/v1/legacy.proto", "message DeliveryReceipt {}\nmessage Old { string fieldSealVersion = 1; }\n")
            write(root, "server/internal/store/sqliteschema/schema.sql", "CREATE TABLE devices (\n cert_fingerprint text,\n cert_not_after timestamp\n);\n")
            write(root, "server/internal/ca/ca.go", "package ca\nfunc SetTrustBundle() {}\n")
            metrics = judge.metric_matches(root)
            for name in ("policy_receipt_protocol_types", "stale_field_sealing_protocol_machinery", "legacy_device_identity_columns", "certificate_lifecycle_junk"):
                self.assertGreater(len(metrics[name]), 0, name)

    def test_missing_current_structure_fails(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            write(root, "contract/proto/cadestro/v1/control.proto", "service ControlService { rpc SyncDevice(A) returns (B); }\n")
            result = judge.report(root)
            self.assertFalse(result["pass"])
            self.assertGreater(result["metrics"]["missing_pull_sync_structures"]["count"], 0)


if __name__ == "__main__":
    unittest.main()

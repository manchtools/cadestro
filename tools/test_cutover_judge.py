#!/usr/bin/env python3
import json
import tempfile
import unittest
from pathlib import Path

import cutover_judge as judge


def write(root: Path, name: str, content: str) -> None:
    path = root / name
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


def feature_fixture(root: Path, junk: bool = False) -> None:
    write(root, "contract/proto/cadestro/v1/control.proto", """service ControlService {
  rpc Keep(KeepRequest) returns (KeepResponse);
  rpc Removed(RemovedRequest) returns (RemovedResponse);
}
message RegistrationToken {
  int32 max_uses = 1;
  int32 current_uses = 2;
  google.protobuf.Timestamp expires_at = 3;
  bool disabled = 4;
}
message CreateTokenRequest {
  int32 max_uses = 1;
  google.protobuf.Timestamp expires_at = 2;
}
""")
    write(root, "contract/proto/cadestro/v1/agent.proto", "service AgentService { rpc Stream(A) returns (B); }\n")
    write(root, "contract/proto/cadestro/v1/device_auth.proto", "service DeviceAuthService { rpc Enroll(A) returns (B); }\n")
    write(root, "contract/proto/cadestro/v1/actions.proto", """enum ActionType { ACTION_TYPE_KEEP = 1; }
message Action { oneof params { KeepParams keep = 1; } }
message KeepParams { string value = 1; }
""")
    write(root, "web/src/routes/(app)/devices/+page.svelte", "<h1>devices</h1>\n")
    write(root, "sdk/pkg/pkg.go", "package pkg\n")
    write(root, "sdk/crypto/crypto.go", "package crypto\n")
    write(root, "sdk/docs/03-capabilities/01-network.md", "# network\n")
    write(root, "server/internal/device/handlers.go", "package device\n")
    write(root, "agent/internal/executor/executor.go", "package executor\n")
    write(root, "server/internal/controlruntime/runtime.go", "package controlruntime\n")
    if junk:
        write(root, "agent/internal/store/migrations/001.sql", """-- +goose Up
CREATE TABLE manifest_deliveries (delivery_id TEXT PRIMARY KEY, state TEXT NOT NULL);
CREATE TABLE manifest_occurrences (delivery_id TEXT NOT NULL, occurrence_id TEXT NOT NULL);
CREATE TABLE reboot_markers (delivery_id TEXT NOT NULL);
-- +goose Down
DROP TABLE manifest_deliveries;
""")
        write(root, "server/internal/registrationtoken/token.go", """package registrationtoken
type RegistrationToken struct { OneTime bool; CurrentUses int; OwnerID string }
""")
        write(root, "agent/internal/executor/legacy.go", """package executor
var old = 1
var (
    executorRunner = old
    desktopMgr = old
)
type PolicyPushState string
type deviceIdentityKey struct{}
""")
        write(root, "server/internal/policy.go", """package policy
func ResolvePolicy() {}
func DispatchPolicy() {}
func EnforceDeviceScope() {}
""")
        (root / "server/internal/controlruntime/runtime.go").write_text("""package controlruntime
import _ \"github.com/example/cadestro/server/internal/old\"
""", encoding="utf-8")
        write(root, "server/internal/store/sqliteschema/schema.sql", "CREATE TABLE deliveries (delivery_id text, state text);\n")
        write(root, "contract/proto/cadestro/v1/delivery.proto", """message ManifestDelivery {
  string delivery_id = 1;
  string occurrence_id = 2;
}
message DeliveryReceipt { string delivery_id = 1; }
""")
        write(root, "sdk/crypto/sealing.go", "package crypto\nfunc FieldSealContext() {}\n")


class CutoverJudgeTest(unittest.TestCase):
    def test_real_baseline_discovery_is_non_vacuous(self) -> None:
        root = Path(__file__).resolve().parents[1]
        inventory = judge.feature_inventory(root)
        self.assertTrue(all(inventory.values()), inventory)
        domains = judge.implementation_inventory(root, require_nonempty=True)
        self.assertTrue(all(domains.values()), domains)

    def test_removed_feature_is_unexplained_failure(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            baseline = Path(directory) / "baseline"
            candidate = Path(directory) / "candidate"
            feature_fixture(baseline)
            feature_fixture(candidate)
            control = candidate / "contract/proto/cadestro/v1/control.proto"
            control.write_text(control.read_text(encoding="utf-8").replace("  rpc Removed(RemovedRequest) returns (RemovedResponse);\n", ""), encoding="utf-8")
            result = judge.compare_features(judge.feature_inventory(baseline), judge.feature_inventory(candidate), [])
            self.assertFalse(result["pass"])
            self.assertIn({"category": "rpc", "feature": "ControlService.Removed"}, result["unexplained_missing"])

    def test_removed_token_posture_is_unexplained_failure(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            baseline = Path(directory) / "baseline"
            candidate = Path(directory) / "candidate"
            feature_fixture(baseline)
            feature_fixture(candidate)
            control = candidate / "contract/proto/cadestro/v1/control.proto"
            control.write_text(control.read_text(encoding="utf-8").replace("  int32 current_uses = 2;\n", ""), encoding="utf-8")
            result = judge.compare_features(judge.feature_inventory(baseline), judge.feature_inventory(candidate), [])
            self.assertFalse(result["pass"])
            self.assertIn(
                {"category": "registration_token_posture", "feature": "RegistrationToken.current_uses"},
                result["unexplained_missing"],
            )

    def test_removed_machinery_retains_feature_and_passes(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            baseline = Path(directory) / "baseline"
            candidate = Path(directory) / "candidate"
            feature_fixture(baseline, junk=True)
            feature_fixture(candidate, junk=False)
            runtime = candidate / "server/internal/controlruntime"
            (runtime / "runtime.go").unlink()
            runtime.rmdir()
            baseline_features = judge.feature_inventory(baseline)
            candidate_features = judge.feature_inventory(candidate)
            feature = judge.compare_features(baseline_features, candidate_features, [])
            domains = judge.implementation_inventory(baseline)
            candidate_domains = judge.implementation_inventory(candidate)
            counts = {name: len(items) for name, items in judge.metric_matches(baseline).items()}
            simplify = judge.simplification_report(candidate, counts)
            self.assertTrue(feature["pass"], feature)
            self.assertNotIn("server:controlruntime", sum(candidate_domains.values(), []))
            self.assertIn("server:controlruntime", sum(domains.values(), []))
            self.assertTrue(simplify["improved"], simplify)
            self.assertTrue(simplify["pass"], simplify)
            self.assertGreater(counts["manifest_delivery_protocol_types_fields"], 0)
            self.assertGreater(counts["legacy_registration_token_counter_owner_state"], 0)
            self.assertGreater(counts["agent_scheduled_work_tables"], 0)
            self.assertEqual(len(judge.executor_global_matches(baseline)), 2)

    def test_effective_agent_schema_rejects_parallel_cutover_tables(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            baseline = Path(directory) / "baseline"
            candidate = Path(directory) / "candidate"
            for root in (baseline, candidate):
                feature_fixture(root)
                write(root, "agent/internal/store/migrations/001.sql", """-- +goose Up
CREATE TABLE manifest_deliveries (delivery_id TEXT PRIMARY KEY);
CREATE TABLE manifest_occurrences (delivery_id TEXT NOT NULL);
CREATE TABLE reboot_markers (delivery_id TEXT NOT NULL);
-- +goose Down
""")
            write(candidate, "agent/internal/store/migrations/002.sql", """-- +goose Up
CREATE TABLE work_items (work_id TEXT PRIMARY KEY);
CREATE TABLE work_occurrences (work_id TEXT NOT NULL);
CREATE TABLE work_reboot_markers (work_id TEXT NOT NULL);
-- +goose Down
""")
            baseline_counts = {name: len(items) for name, items in judge.metric_matches(baseline).items()}
            result = judge.simplification_report(candidate, baseline_counts)
            metric = result["metrics"]["agent_scheduled_work_tables"]
            self.assertFalse(metric["pass"], metric)
            self.assertGreater(metric["candidate"], metric["baseline"])

    def test_assigned_policy_pull_removes_submission_coupling(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            baseline = Path(directory) / "baseline"
            candidate = Path(directory) / "candidate"
            for root in (baseline, candidate):
                feature_fixture(root)
            write(baseline, "server/internal/dispatch/handlers.go", """package dispatch
func (h *Handlers) DispatchAssignedActions() {
    inputs := h.assignedManifests()
    h.submitter.Submit(inputs)
}
""")
            write(candidate, "server/internal/dispatch/handlers.go", """package dispatch
func (h *Handlers) DispatchAssignedActions() { h.signalSync() }
""")
            before = judge.assigned_policy_push_matches(baseline)
            after = judge.assigned_policy_push_matches(candidate)
            self.assertGreater(len(before), 0)
            self.assertEqual(after, [])

    def test_legacy_definition_compile_metric_is_exact_and_hard_zero(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            baseline = Path(directory) / "baseline"
            candidate = Path(directory) / "candidate"
            for root in (baseline, candidate):
                feature_fixture(root)
            write(baseline, "server/internal/manifest/compiler.go", """package manifest
func (c *Compiler) definition(ctx context.Context, deviceID, id string) ([]*Manifest, error) { return nil, nil }
func (c *Compiler) DefinitionRunbookForDevice(ctx context.Context, deviceID, id string) (*Manifest, error) { return nil, nil }
""")
            write(candidate, "server/internal/manifest/compiler.go", """package manifest
func (c *Compiler) definitionRunbook(ctx context.Context, deviceID, id string) (*Manifest, error) { return nil, nil }
""")
            before = judge.legacy_definition_compile_matches(baseline)
            after = judge.legacy_definition_compile_matches(candidate)
            self.assertEqual(len(before), 2)
            self.assertEqual(after, [])
            counts = {name: len(items) for name, items in judge.metric_matches(baseline).items()}
            result = judge.simplification_report(candidate, counts)
            metric = result["metrics"]["legacy_definition_compile_paths"]
            self.assertEqual(metric["baseline"], 2)
            self.assertEqual(metric["candidate"], 0)
            self.assertTrue(metric["zero_invariant"], metric)
            self.assertTrue(metric["pass"], metric)

    def test_completed_cutovers_are_hard_zeroes(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            baseline = Path(directory) / "baseline"
            candidate = Path(directory) / "candidate"
            feature_fixture(baseline, junk=True)
            feature_fixture(candidate)
            write(candidate, "server/internal/dispatch/handlers.go", """package dispatch
func (h *Handlers) DispatchAssignedActions() { h.submitter.Submit(h.assignedManifests()) }
""")
            write(candidate, "server/internal/registrationtoken/token.go", """package registrationtoken
type RegistrationToken struct { OneTime bool }
""")
            counts = {name: len(items) for name, items in judge.metric_matches(baseline).items()}
            result = judge.simplification_report(candidate, counts)
            for name in (
                "assigned_policy_push_submission_coupling",
                "legacy_registration_token_counter_owner_state",
            ):
                metric = result["metrics"][name]
                self.assertTrue(metric["zero_invariant"], metric)
                self.assertFalse(metric["pass"], metric)

    def test_field_sealing_machinery_is_non_vacuous_and_hard_zero(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            baseline = Path(directory) / "baseline"
            candidate = Path(directory) / "candidate"
            feature_fixture(baseline)
            feature_fixture(candidate)
            write(baseline, "contract/proto/cadestro/v1/legacy.proto", "message Legacy { string fieldSealVersion = 1; }\n")
            counts = {name: len(items) for name, items in judge.metric_matches(baseline).items()}
            self.assertGreater(counts["stale_field_sealing_protocol_machinery"], 0)
            result = judge.simplification_report(candidate, counts)
            metric = result["metrics"]["stale_field_sealing_protocol_machinery"]
            self.assertTrue(metric["zero_invariant"], metric)
            self.assertTrue(metric["pass"], metric)
            write(candidate, "contract/proto/cadestro/v1/legacy.proto", "message Legacy { string fieldSealVersion = 1; }\n")
            result = judge.simplification_report(candidate, counts)
            self.assertFalse(result["metrics"]["stale_field_sealing_protocol_machinery"]["pass"])

    def test_field_sealing_judge_detects_live_wiring_but_ignores_sdk_and_tests(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            feature_fixture(root)
            write(root, "sdk/crypto/field.go", "package crypto\nfunc FieldSealContext() {}\n")
            write(root, "server/internal/crypto/field_test.go", "package crypto\nvar _ = SealedValue{}\n")
            self.assertEqual(judge.live_field_sealing_matches(root), [])
            for name, token in {
                "agent.go": "agent_sealing_public_key",
                "control.go": "control_sealing_public_key",
                "config.go": "CADESTRO_SEALING_KEY",
                "runtime.go": "ConfigureSealing",
                "seal.go": "SealToPublicKey",
                "open.go": "OpenWithPrivateKey",
                "proto.proto": "SealedValue",
            }.items():
                write(root, "server/internal/live/" + name, token + "\n")
                self.assertGreater(len(judge.live_field_sealing_matches(root)), 0, token)

    def test_device_specific_manifest_compiler_is_a_hard_zero(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            baseline = Path(directory) / "baseline"
            candidate = Path(directory) / "candidate"
            feature_fixture(baseline)
            feature_fixture(candidate)
            write(
                baseline,
                "server/internal/manifest/compiler.go",
                "package manifest\nfunc (c *Compiler) ActionForDevice() {}\n",
            )
            counts = {name: len(items) for name, items in judge.metric_matches(baseline).items()}
            self.assertGreater(counts["device_specific_manifest_compile_paths"], 0)
            self.assertTrue(judge.simplification_report(candidate, counts)["pass"])
            write(
                candidate,
                "server/internal/dispatch/handlers.go",
                "package dispatch\nfunc use() { compiler.DefinitionForDevice() }\n",
            )
            result = judge.simplification_report(candidate, counts)
            self.assertFalse(result["metrics"]["device_specific_manifest_compile_paths"]["pass"])

    def test_device_identity_metric_counts_schema_not_references(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            write(root, "server/internal/store/sqliteschema/schema.sql", """CREATE TABLE devices (
    id text PRIMARY KEY,
    agent_sealing_public_key blob,
    cert_fingerprint text,
    cert_not_after timestamp,
    enrollment_identity_public_key blob
);
""")
            write(root, "server/internal/enrollment/handlers.go", """package enrollment
func retry() { _ = \"cert_fingerprint cert_not_after device_identity\" }
""")
            found = judge.legacy_device_identity_columns(root)
            self.assertEqual([item.text.strip().split()[0] for item in found], [
                "agent_sealing_public_key", "cert_fingerprint", "cert_not_after",
            ])

    def test_policy_result_transport_must_reuse_the_existing_path(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            baseline = Path(directory) / "baseline"
            candidate = Path(directory) / "candidate"
            for root in (baseline, candidate):
                feature_fixture(root)
            write(candidate, "contract/client.go", "package contract\nfunc SendPolicyActionResult() {}\n")
            counts = {name: len(items) for name, items in judge.metric_matches(baseline).items()}
            result = judge.simplification_report(candidate, counts)
            metric = result["metrics"]["policy_specific_result_transport_paths"]
            self.assertFalse(metric["pass"], metric)
            self.assertTrue(metric["zero_invariant"], metric)

    def test_removed_token_field_reservations_are_not_live_state(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            write(root, "contract/proto/cadestro/v1/control.proto", """message RegistrationToken {
  reserved 4, 11;
  reserved "one_time", "owner_id";
  int32 current_uses = 6;
}
message CreateTokenRequest {
  reserved 2, 5;
  reserved "one_time", "owner_id";
}
""")
            self.assertEqual(judge.legacy_registration_token_matches(root), [])

    def test_exception_requires_reason_and_present_merge_target(self) -> None:
        baseline = {"rpc": ["ControlService.Old"]}
        candidate = {"rpc": ["ControlService.New"]}
        mapping = [{"category": "rpc", "from": "ControlService.Old", "to": ["ControlService.New"], "reason": "merged into the single cutover endpoint"}]
        result = judge.compare_features(baseline, candidate, mapping)
        self.assertTrue(result["pass"], json.dumps(result))
        with self.assertRaises(judge.JudgeError):
            judge.compare_features(baseline, candidate, [{"category": "rpc", "from": "ControlService.Old", "to": [], "reason": ""}])

    def test_certificate_junk_metric_ignores_reusable_sdk_capabilities(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            write(root, "sdk/crypto/cert.go", "package crypto\nfunc VerifyCAContinuity() {}\n")
            write(root, "contract/client.go", "package contract\nfunc WithMTLSFromPEMAndSystemRoots() {}\n")
            self.assertEqual(judge.certificate_lifecycle_junk_matches(root), [])

            write(root, "server/internal/enrollment/handlers.go", "package enrollment\nfunc legacy() { _ = AssertCSRMatchesCertKey }\n")
            self.assertGreater(len(judge.certificate_lifecycle_junk_matches(root)), 0)

    def test_certificate_junk_metric_ignores_test_sources(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            write(root, "server/deploy/setup_test.sh", "CA_TRUST_BUNDLE_FILE=/run/certs/ca-trust-bundle.crt\n")
            write(root, "server/internal/ca/ca_test.go", "package ca\nfunc SetTrustBundle() {}\n")
            self.assertEqual(judge.certificate_lifecycle_junk_matches(root), [])

    def test_certificate_junk_metric_detects_live_server_rotation(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            write(root, "server/internal/ca/ca.go", "package ca\nfunc SetTrustBundle() {}\n")
            write(root, "server/deploy/setup.sh", "CADESTRO_CA_TRUST_BUNDLE_FILE=/run/certs/ca-trust-bundle.crt\n")
            found = judge.certificate_lifecycle_junk_matches(root)
            self.assertEqual(len(found), 2, found)

    def test_certificate_junk_metric_requires_hard_zero(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            baseline = Path(directory) / "baseline"
            candidate = Path(directory) / "candidate"
            write(baseline, "server/internal/ca/ca.go", "package ca\nfunc SetTrustBundle() {}\n")
            write(baseline, "server/deploy/setup.sh", "CA_TRUST_BUNDLE_FILE=/run/certs/ca-trust-bundle.crt\n")
            write(candidate, "server/internal/ca/ca.go", "package ca\nfunc SetTrustBundle() {}\n")
            counts = {name: len(items) for name, items in judge.metric_matches(baseline).items()}
            report = judge.simplification_report(candidate, counts)
            metric = report["metrics"]["certificate_lifecycle_junk"]
            self.assertFalse(metric["pass"], metric)
            self.assertGreater(metric["baseline"], metric["candidate"])
            self.assertGreater(metric["candidate"], 0)
            self.assertFalse(report["pass"], report)


if __name__ == "__main__":
    unittest.main()

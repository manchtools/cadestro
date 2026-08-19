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
            self.assertEqual(len(judge.executor_global_matches(baseline)), 2)

    def test_exception_requires_reason_and_present_merge_target(self) -> None:
        baseline = {"rpc": ["ControlService.Old"]}
        candidate = {"rpc": ["ControlService.New"]}
        mapping = [{"category": "rpc", "from": "ControlService.Old", "to": ["ControlService.New"], "reason": "merged into the single cutover endpoint"}]
        result = judge.compare_features(baseline, candidate, mapping)
        self.assertTrue(result["pass"], json.dumps(result))
        with self.assertRaises(judge.JudgeError):
            judge.compare_features(baseline, candidate, [{"category": "rpc", "from": "ControlService.Old", "to": [], "reason": ""}])


if __name__ == "__main__":
    unittest.main()

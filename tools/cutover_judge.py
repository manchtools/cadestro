#!/usr/bin/env python3
"""Check the live cutover contract and architecture invariants.

The judge reads the candidate tree directly.  It intentionally has no
historical checkout, golden diff, alias map, or exception file: the current
architecture is the oracle.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable


SKIP_PARTS = {"node_modules", ".git", "dist", "build", "archive"}
GENERATED_PARTS = {"gen", "generated"}


class JudgeError(RuntimeError):
    pass


@dataclass(frozen=True)
class Match:
    path: str
    line: int
    text: str

    def as_dict(self) -> dict[str, object]:
        return {"path": self.path, "line": self.line, "text": self.text.strip()[:180]}


def files(root: Path, suffixes: set[str] | None = None) -> Iterable[Path]:
    for path in root.rglob("*"):
        if not path.is_file() or any(part in SKIP_PARTS for part in path.parts):
            continue
        if suffixes is None or path.suffix in suffixes:
            yield path


def rel(root: Path, path: Path) -> str:
    return path.relative_to(root).as_posix()


def is_generated(root: Path, path: Path) -> bool:
    return any(part in GENERATED_PARTS for part in path.relative_to(root).parts)


def source(path: Path) -> str:
    return path.read_text(encoding="utf-8", errors="replace")


def matches(root: Path, pattern: str, suffixes: set[str] | None = None) -> list[Match]:
    rx = re.compile(pattern, re.IGNORECASE)
    found: list[Match] = []
    for path in files(root, suffixes):
        if path.name.endswith("_test.go") or ".test." in path.name or is_generated(root, path):
            continue
        for number, line in enumerate(source(path).splitlines(), 1):
            if rx.search(line):
                found.append(Match(rel(root, path), number, line))
    return found


def proto_service_methods(root: Path, service: str) -> dict[str, Match]:
    result: dict[str, Match] = {}
    pattern = re.compile(rf"^\s*rpc\s+(\w+)\s*\(", re.MULTILINE)
    service_pattern = re.compile(rf"\bservice\s+{re.escape(service)}\s*\{{(.*?)\}}", re.DOTALL)
    for path in files(root, {".proto"}):
        for block in service_pattern.findall(source(path)):
            for match in pattern.finditer(block):
                line = source(path)[: match.start()].count("\n") + 1
                result[match.group(1)] = Match(rel(root, path), line, match.group(0))
    return result


def authored_action_value_matches(root: Path) -> list[Match]:
    """ACTION_TYPE_SYNC/REBOOT are retired authored action values."""
    result: list[Match] = []
    token = re.compile(r"^\s*ACTION_TYPE_(?:SYNC|REBOOT)\s*=", re.IGNORECASE)
    for path in files(root, {".proto"}):
        for number, line in enumerate(source(path).splitlines(), 1):
            if token.search(line):
                result.append(Match(rel(root, path), number, line))
    return result


def legacy_live_rpc_matches(root: Path) -> list[Match]:
    """The retired unary aliases must not be in source API or production wiring."""
    token = re.compile(r"\b(?:DispatchInstantAction|DispatchAssignedActions)\b")
    found: list[Match] = []
    for path in files(root, {".go", ".proto"}):
        relative = rel(root, path)
        if path.name.endswith("_test.go") or is_generated(root, path):
            continue
        for number, line in enumerate(source(path).splitlines(), 1):
            if token.search(line.split("//", 1)[0]):
                found.append(Match(relative, number, line))
    return found


def delivery_protocol_matches(root: Path) -> list[Match]:
    """Receipt/transport-delivery protocol residue is not part of pull sync."""
    token = re.compile(r"^\s*message\s+DeliveryReceipt\b|\bdelivery_receipt\s*=\s*\d+\s*;")
    found: list[Match] = []
    for path in files(root, {".go", ".proto"}):
        relative = rel(root, path)
        if path.name.endswith("_test.go") or is_generated(root, path):
            continue
        for number, line in enumerate(source(path).splitlines(), 1):
            if token.search(line.split("//", 1)[0]):
                found.append(Match(relative, number, line))
    return found


def live_policy_manifest_push_matches(root: Path) -> list[Match]:
    """Find control functions coupling a manifest delivery to live epoch sends."""
    result: list[Match] = []
    token = re.compile(r"\b(?:ManifestDelivery|ServerMessage_ManifestDelivery)\b|\bSendAtEpoch\s*\(")
    for path in files(root, {".go"}):
        relative = rel(root, path)
        if not relative.startswith("server/internal/") or path.name.endswith("_test.go") or is_generated(root, path):
            continue
        lines = source(path).splitlines()
        start = 0
        for number, line in enumerate(lines + ["func __judge_end__() {}"], 1):
            if number > 1 and re.match(r"^\s*func\s+", line):
                body = lines[start:number - 1]
                body_text = "\n".join(body)
                if re.search(r"\bSendAtEpoch\s*\(", body_text) and re.search(
                    r"\b(?:ManifestDelivery|ServerMessage_ManifestDelivery)\b", body_text
                ):
                    result.extend(Match(relative, index, value) for index, value in enumerate(body, start + 1) if token.search(value))
                start = number - 1
    return result


def _production_go_functions(root: Path) -> Iterable[tuple[str, list[tuple[int, str]]]]:
    for path in files(root, {".go"}):
        relative = rel(root, path)
        if not relative.startswith("server/internal/") or path.name.endswith("_test.go") or is_generated(root, path):
            continue
        lines = source(path).splitlines()
        start: int | None = None
        for number, line in enumerate(lines + ["func __judge_end__() {}"], 1):
            if re.match(r"^\s*func\s+", line):
                if start is not None:
                    yield relative, list(enumerate(lines[start - 1:number - 1], start))
                start = number


def live_control_coupling_matches(root: Path, category: str | None = None) -> list[Match]:
    """No live Sync/Reboot operation may use retired policy machinery."""
    patterns = {
        "action_type": re.compile(r"\bActionType\b|\bACTION_TYPE_(?:SYNC|REBOOT)\b"),
        "manifest": re.compile(r"\b(?:Manifest|OneShotAction|compile(?:d)?Manifest)\b", re.IGNORECASE),
        "delivery": re.compile(r"\b(?:Delivery|InsertDelivery|Submit(?:Batch)?|submitter)\b", re.IGNORECASE),
        "scheduler": re.compile(r"\b(?:scheduler|Schedule(?:d)?|queueDue|worker|sweep)\b", re.IGNORECASE),
        "timing": re.compile(r"\b(?:run_?at|RunAt|scheduled_?for|ScheduledFor|maintenance|retry|Retry|epoch)\b", re.IGNORECASE),
    }
    if category is not None and category not in patterns:
        raise JudgeError(f"unknown live-control coupling category: {category}")
    wanted = {category: patterns[category]} if category else patterns
    operation = re.compile(r"\b(?:Sync|Reboot)(?:Device|Agent)?\b|\b(?:send|trigger)(?:Sync|Reboot)", re.IGNORECASE)
    found: list[Match] = []
    for relative, body in _production_go_functions(root):
        if not operation.search(body[0][1]) or "/agentsync/" in f"/{relative}" or "/agentstream/" in f"/{relative}":
            continue
        for number, line in body:
            code = line.split("//", 1)[0]
            for name, pattern in wanted.items():
                if pattern.search(code):
                    found.append(Match(relative, number, f"{name}: {line}"))
    return found


def live_field_sealing_matches(root: Path) -> list[Match]:
    pattern = re.compile(
        r"(?:\bfieldSealVersion\b|\bsealedFieldVersion\b|\bprotocolVersion\b|\bwireProtocol\b|"
        r"\bSealedValue\b|\bagent_sealing_public_key\b|\bcontrol_sealing_public_key\b|"
        r"\bCADESTRO_SEALING_KEY(?:_FILE)?\b|\bConfigureSealing\b|\bFieldSealContext\b|"
        r"\bSealToPublicKey\b|\bOpenWithPrivateKey\b)", re.IGNORECASE)
    found: list[Match] = []
    for path in files(root, {".go", ".proto", ".sql", ".sh", ".yaml", ".yml"}):
        relative = rel(root, path)
        if not relative.startswith(("agent/", "server/", "contract/")) or path.name.endswith(("_test.go", "_test.sh")) or "/testdata/" in f"/{relative}/" or is_generated(root, path):
            continue
        for number, line in enumerate(source(path).splitlines(), 1):
            if pattern.search(line):
                found.append(Match(relative, number, line))
    return found


def legacy_device_identity_columns(root: Path) -> list[Match]:
    path = root / "server/internal/store/sqliteschema/schema.sql"
    if not path.is_file():
        return []
    result: list[Match] = []
    in_devices = False
    legacy = re.compile(r"^\s*(agent_sealing_public_key|cert_fingerprint|cert_not_after)\b", re.IGNORECASE)
    for number, line in enumerate(source(path).splitlines(), 1):
        if re.search(r"^\s*CREATE\s+TABLE\s+devices\b", line, re.IGNORECASE):
            in_devices = True
        elif in_devices and line.strip().startswith(")"):
            break
        elif in_devices and legacy.search(line):
            result.append(Match(rel(root, path), number, line))
    return result


def certificate_lifecycle_junk_matches(root: Path) -> list[Match]:
    pattern = re.compile(r"\b(?:revoked_certificates|RevocationChecker|RevokeInTx|startCertRotation|current_certificate|ca_certificate|CATrustBundle|SetTrustBundle|TrustBundle|AssertCSRMatchesCertKey|PeerClassFromPEM|ReplaceDeviceCertificate|ClearPendingDeviceCertificate)\b|CA_TRUST_BUNDLE_FILE|ca-trust-bundle\.crt")
    found: list[Match] = []
    for path in files(root, {".go", ".proto", ".sql", ".sh", ".yml", ".yaml"}):
        relative = rel(root, path)
        if path.name.endswith(("_test.go", "_test.sh")) or path.name.startswith("test_") or is_generated(root, path):
            continue
        if path.suffix == ".sql" and "revoked_certificates" not in relative:
            continue
        if path.suffix == ".proto" and "control.proto" not in relative:
            continue
        if path.suffix == ".go" and not ("enrollment/handlers.go" in relative or "cert_rotation.go" in relative or "revoked_certificates" in relative or "/ca/" in relative or relative.endswith(("cmd/cadestro/config.go", "cmd/cadestro/main.go"))):
            continue
        if path.suffix in {".sh", ".yml", ".yaml"} and "server/deploy/" not in relative:
            continue
        for number, line in enumerate(source(path).splitlines(), 1):
            if pattern.search(line):
                found.append(Match(relative, number, line))
    return found


def pull_sync_structure_matches(root: Path) -> list[Match]:
    """Require the typed control RPCs and the durable pull/result structures."""
    required = {
        "ControlService.SyncDevice": proto_service_methods(root, "ControlService").get("SyncDevice"),
        "ControlService.RebootDevice": proto_service_methods(root, "ControlService").get("RebootDevice"),
    }
    result = [Match("contract/proto", 0, f"missing {name}") for name, match in required.items() if match is None]
    text = "\n".join(source(path) for path in files(root, {".proto"}))
    for message in ("Manifest", "ManifestResult", "SyncState", "DesiredPolicy"):
        if not re.search(rf"\bmessage\s+{message}\s*\{{", text):
            result.append(Match("contract/proto", 0, f"missing message {message}"))
    sync_state = re.search(r"\bmessage\s+SyncState\s*\{([^}]*)\}", text, re.DOTALL)
    if not sync_state or not re.search(r"\bDesiredPolicy\s+desired_policy\s*=", sync_state.group(1)):
        result.append(Match("contract/proto", 0, "SyncState.desired_policy is not pull-sync data"))
    return result


def metric_matches(root: Path) -> dict[str, list[Match]]:
    return {
        "authored_sync_reboot_action_values": authored_action_value_matches(root),
        "legacy_live_control_rpc_apis": legacy_live_rpc_matches(root),
        "policy_receipt_protocol_types": delivery_protocol_matches(root),
        "live_policy_manifest_push_paths": live_policy_manifest_push_matches(root),
        "live_control_action_type_coupling": live_control_coupling_matches(root, "action_type"),
        "live_control_manifest_coupling": live_control_coupling_matches(root, "manifest"),
        "live_control_delivery_coupling": live_control_coupling_matches(root, "delivery"),
        "live_control_scheduler_coupling": live_control_coupling_matches(root, "scheduler"),
        "live_control_timing_coupling": live_control_coupling_matches(root, "timing"),
        "stale_field_sealing_protocol_machinery": live_field_sealing_matches(root),
        "legacy_device_identity_columns": legacy_device_identity_columns(root),
        "certificate_lifecycle_junk": certificate_lifecycle_junk_matches(root),
        "missing_pull_sync_structures": pull_sync_structure_matches(root),
    }


def report(root: Path) -> dict[str, object]:
    found = metric_matches(root)
    metrics = {
        name: {"count": len(items), "matches": [item.as_dict() for item in items], "pass": not items}
        for name, items in found.items()
    }
    passed = all(value["pass"] for value in metrics.values())
    return {"candidate": str(root), "metrics": metrics, "pass": passed}


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", default=".", help="repository to inspect")
    parser.add_argument("--candidate", default="WORKTREE", help="accepted for CI readability; the live tree is always inspected")
    parser.add_argument("--json", action="store_true", help="emit machine-readable JSON")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    root = Path(args.repo).resolve()
    if args.candidate not in {"WORKTREE", "working-tree", "."} and Path(args.candidate).is_dir():
        root = Path(args.candidate).resolve()
    try:
        result = report(root)
    except (OSError, JudgeError) as exc:
        result = {"candidate": str(root), "pass": False, "error": str(exc)}
    if args.json:
        print(json.dumps(result, indent=2, sort_keys=True))
    else:
        print(f"cutover judge: {'PASS' if result['pass'] else 'FAIL'}")
        for name, metric in result.get("metrics", {}).items():
            print(f"{name}={metric['count']}")
        if "error" in result:
            print(f"FAIL: {result['error']}", file=sys.stderr)
    return 0 if result["pass"] else 1


if __name__ == "__main__":
    raise SystemExit(main())

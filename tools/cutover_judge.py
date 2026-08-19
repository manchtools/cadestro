#!/usr/bin/env python3
"""Empirical feature-preservation and simplification judge for a cutover.

The scanner deliberately uses source discovery rather than a hand-maintained
feature list.  A baseline that discovers nothing is an error, not a green
result.  Candidate refs are archived into a temporary directory, so judging a
branch never checks it out or edits it.
"""

from __future__ import annotations

import argparse
import io
import json
import re
import sqlite3
import subprocess
import sys
import tarfile
import tempfile
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable


SKIP_PARTS = {"node_modules", ".git", "dist", "build"}
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
        if suffixes is not None and path.suffix not in suffixes:
            continue
        yield path


def text(path: Path) -> str:
    try:
        return path.read_text(encoding="utf-8", errors="replace")
    except OSError as exc:
        raise JudgeError(f"cannot read {path}: {exc}") from exc


def rel(root: Path, path: Path) -> str:
    return path.relative_to(root).as_posix()


def is_generated(root: Path, path: Path) -> bool:
    return any(part in GENERATED_PARTS for part in path.relative_to(root).parts)


def matches(root: Path, pattern: str, suffixes: set[str] | None = None) -> list[Match]:
    rx = re.compile(pattern, re.IGNORECASE)
    found: list[Match] = []
    for path in files(root, suffixes):
        if path.name.endswith("_test.go") or ".test." in path.name or is_generated(root, path):
            continue
        for number, line in enumerate(text(path).splitlines(), 1):
            if rx.search(line):
                found.append(Match(rel(root, path), number, line))
    return found


def proto_files(root: Path) -> list[Path]:
    return list(files(root, {".proto"}))


def block(source: str, start_pattern: str, end_pattern: str | None = None) -> str:
    start = re.search(start_pattern, source, re.IGNORECASE | re.MULTILINE)
    if not start:
        return ""
    tail = source[start.end() :]
    if end_pattern:
        end = re.search(end_pattern, tail, re.IGNORECASE | re.MULTILINE)
        if end:
            tail = tail[: end.start()]
    return tail


def discover_rpc(root: Path) -> list[str]:
    result: set[str] = set()
    for path in proto_files(root):
        source = text(path)
        for service in re.finditer(r"\bservice\s+(\w+)\s*\{(.*?)\}", source, re.DOTALL):
            service_name, body = service.groups()
            if not re.search(r"Control|Agent|DeviceAuth", service_name, re.IGNORECASE):
                continue
            for method in re.finditer(r"\brpc\s+(\w+)\s*\(", body):
                result.add(f"{service_name}.{method.group(1)}")
    return sorted(result)


def discover_rpc_group(root: Path, service_pattern: str) -> list[str]:
    result = []
    for item in discover_rpc(root):
        if re.search(service_pattern, item.split(".", 1)[0], re.IGNORECASE):
            result.append(item)
    return result


def discover_registration_token_posture(root: Path) -> list[str]:
    result: set[str] = set()
    wanted = {
        "RegistrationToken": {"max_uses", "current_uses", "expires_at", "disabled"},
        "CreateTokenRequest": {"max_uses", "expires_at"},
    }
    for path in proto_files(root):
        source = text(path)
        for message, fields in wanted.items():
            body = block(source, rf"\bmessage\s+{message}\s*\{{", r"\n\}")
            for field in fields:
                if re.search(rf"^\s*[\w.<>]+\s+{field}\s*=\s*\d+\s*;", body, re.MULTILINE):
                    result.add(f"{message}.{field}")
    return sorted(result)


def discover_actions(root: Path) -> tuple[list[str], list[str]]:
    sources = "\n".join(text(path) for path in proto_files(root))
    enum = block(sources, r"\benum\s+ActionType\s*\{", r"\n\}")
    values = re.findall(r"^\s*(ACTION_TYPE_[A-Z0-9_]+)\s*=\s*(-?\d+)", enum, re.MULTILINE)
    action_values = [f"{name}={number}" for name, number in values]
    action = block(sources, r"\bmessage\s+Action\s*\{", r"\n\}")
    params = re.findall(r"^\s*(\w+Params)\s+\w+\s*=\s*\d+\s*;", action, re.MULTILINE)
    # Keep the inventory useful if a proto moves the oneof into a helper.
    if not params:
        params = re.findall(r"^\s*message\s+(\w+Params)\s*\{", sources, re.MULTILINE)
    return sorted(set(action_values)), sorted(set(params))


def discover_routes(root: Path) -> list[str]:
    route_root = root / "web" / "src" / "routes"
    if not route_root.is_dir():
        return []
    result: set[str] = set()
    for path in route_root.rglob("*"):
        if not path.is_file():
            continue
        relative = path.relative_to(route_root).as_posix()
        pieces = relative.split("/")
        # A feature is a route directory, not every tab/component under it.
        if "+page." in path.name:
            feature = "/".join(piece for piece in pieces[:-1] if not (piece.startswith("(") and piece.endswith(")")))
            result.add("/" + feature.strip("/"))
    return sorted(result)


def discover_sdk(root: Path) -> tuple[list[str], list[str]]:
    sdk = root / "sdk"
    packages: set[str] = set()
    if sdk.is_dir():
        for path in sdk.iterdir():
            if path.is_dir() and any(path.glob("*.go")):
                packages.add(path.name)
    capabilities: set[str] = set()
    cap_root = sdk / "docs" / "03-capabilities"
    if cap_root.is_dir():
        for path in cap_root.glob("*.md"):
            capabilities.add(path.stem)
    return sorted(packages), sorted(capabilities)


def discover_domains(root: Path) -> list[str]:
    result: set[str] = set()
    for product in ("server", "agent"):
        internal = root / product / "internal"
        if not internal.is_dir():
            continue
        for path in internal.iterdir():
            if path.is_dir() and any(path.rglob("*.go")):
                result.add(f"{product}:{path.name}")
    return sorted(result)


def feature_inventory(root: Path, require_nonempty: bool = True) -> dict[str, list[str]]:
    action_values, action_params = discover_actions(root)
    sdk_packages, sdk_capabilities = discover_sdk(root)
    inventory = {
        "rpc": discover_rpc(root),
        "control_rpcs": discover_rpc_group(root, r"Control"),
        "agent_rpcs": discover_rpc_group(root, r"Agent"),
        "registration_token_posture": discover_registration_token_posture(root),
        "device_auth_rpcs": discover_rpc_group(root, r"DeviceAuth"),
        "action_values": action_values,
        "action_parameter_families": action_params,
        "web_routes": discover_routes(root),
        "sdk_packages": sdk_packages,
        "sdk_capabilities": sdk_capabilities,
    }
    if require_nonempty:
        empty = [name for name, values in inventory.items() if not values]
        if empty:
            raise JudgeError("baseline feature discovery was empty: " + ", ".join(empty))
    return inventory


def implementation_inventory(root: Path, require_nonempty: bool = False) -> dict[str, list[str]]:
    """Return observable package/domain inventory without treating names as API."""
    domains = discover_domains(root)
    inventory = {
        "server_domains": sorted(item for item in domains if item.startswith("server:")),
        "agent_domains": sorted(item for item in domains if item.startswith("agent:")),
    }
    if require_nonempty:
        empty = [name for name, values in inventory.items() if not values]
        if empty:
            raise JudgeError("baseline implementation-domain discovery was empty: " + ", ".join(empty))
    return inventory


def load_exceptions(path: Path | None) -> list[dict[str, object]]:
    if path is None or not path.exists():
        return []
    try:
        raw = json.loads(text(path))
    except json.JSONDecodeError as exc:
        raise JudgeError(f"invalid exception JSON {path}: {exc}") from exc
    entries = raw.get("feature_aliases", []) if isinstance(raw, dict) else None
    if not isinstance(entries, list):
        raise JudgeError("exceptions must contain a feature_aliases array")
    checked: list[dict[str, object]] = []
    for entry in entries:
        if not isinstance(entry, dict) or not isinstance(entry.get("category"), str) or not isinstance(entry.get("from"), str):
            raise JudgeError("every feature exception needs category and from")
        to = entry.get("to", [])
        reason = entry.get("reason")
        if not isinstance(to, list) or not all(isinstance(item, str) for item in to) or not isinstance(reason, str) or not reason.strip():
            raise JudgeError(f"feature exception {entry.get('from')} needs string to[] and non-empty reason")
        checked.append({"category": entry["category"], "from": entry["from"], "to": to, "reason": reason})
    return checked


def compare_features(baseline: dict[str, list[str]], candidate: dict[str, list[str]], exceptions: list[dict[str, object]]) -> dict[str, object]:
    aliases: dict[tuple[str, str], dict[str, object]] = {}
    for entry in exceptions:
        if not isinstance(entry, dict) or not isinstance(entry.get("category"), str) or not isinstance(entry.get("from"), str):
            raise JudgeError("every feature exception needs category and from")
        if not isinstance(entry.get("to"), list) or not all(isinstance(item, str) for item in entry["to"]):
            raise JudgeError(f"feature exception {entry['from']} needs string to[]")
        if not isinstance(entry.get("reason"), str) or not entry["reason"].strip():
            raise JudgeError(f"feature exception {entry['from']} needs a non-empty reason")
        key = (str(entry["category"]), str(entry["from"]))
        if key[0] not in baseline:
            raise JudgeError(f"feature exception uses unknown category: {key[0]}")
        if key[1] not in baseline[key[0]]:
            raise JudgeError(f"feature exception names no baseline feature: {key[0]} {key[1]}")
        if key in aliases:
            raise JudgeError(f"duplicate feature exception: {key[0]} {key[1]}")
        aliases[key] = entry
    categories: dict[str, object] = {}
    unexplained: list[dict[str, str]] = []
    for category, values in baseline.items():
        current = set(candidate.get(category, []))
        missing = sorted(set(values) - current)
        accepted: list[dict[str, object]] = []
        for item in missing:
            entry = aliases.get((category, item))
            if entry is None:
                unexplained.append({"category": category, "feature": item})
                continue
            targets = entry["to"]
            if not any(str(target) in current for target in targets):
                unexplained.append({"category": category, "feature": item, "reason": "mapping target is absent"})
            else:
                accepted.append(entry)
        categories[category] = {
            "baseline_count": len(values),
            "candidate_count": len(current),
            "missing": missing,
            "accepted_mappings": accepted,
        }
    return {"baseline": baseline, "candidate": candidate, "categories": categories, "unexplained_missing": unexplained, "pass": not unexplained}


def sql_durable_matches(root: Path) -> list[Match]:
    result: list[Match] = []
    table: str | None = None
    for path in files(root, {".sql"}):
        for number, line in enumerate(text(path).splitlines(), 1):
            create = re.search(r"\bCREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([\w]+)", line, re.IGNORECASE)
            if create:
                name = create.group(1)
                table = name if (
                    re.search(r"delivery|manifest|occurrence", name, re.IGNORECASE)
                    and not re.search(r"result|history|evidence", name, re.IGNORECASE)
                ) else None
                if table:
                    result.append(Match(rel(root, path), number, f"table {table}"))
                continue
            if table and re.search(r"\b(?:delivery|manifest|occurrence|push|state)\w*\b", line, re.IGNORECASE):
                result.append(Match(rel(root, path), number, line))
            if table and line.strip().startswith(")"):
                table = None
    return result


def agent_scheduled_work_schema(root: Path) -> list[Match]:
    """Materialize agent migrations and count active scheduling tables.

    Historical migrations must remain in source, so grepping their text would
    punish a correct forward cutover. Applying every Up section measures the
    schema an upgraded agent actually runs and catches parallel old/new table
    families even when the replacement is given a generic name.
    """
    migration_root = root / "agent" / "internal" / "store" / "migrations"
    if not migration_root.is_dir():
        return []
    connection = sqlite3.connect(":memory:")
    try:
        for path in sorted(migration_root.glob("*.sql")):
            source = text(path).split("-- +goose Down", 1)[0]
            try:
                connection.executescript(source)
            except sqlite3.Error as exc:
                raise JudgeError(f"cannot materialize agent migration {rel(root, path)}: {exc}") from exc
        names = [
            str(row[0])
            for row in connection.execute("SELECT name FROM sqlite_schema WHERE type = 'table' ORDER BY name")
            if re.fullmatch(
                r"(?:manifest_(?:deliveries|occurrences)|reboot_markers|"
                r"scheduled_work(?:_occurrences|_reboots)?|"
                r"work_(?:items|occurrences|reboot_markers)|transport_deliveries)",
                str(row[0]),
            )
        ]
        return [
            Match("agent:effective-scheduled-work-schema", line, f"table {name}")
            for line, name in enumerate(names, 1)
        ]
    finally:
        connection.close()


def delivery_protocol_matches(root: Path) -> list[Match]:
    return matches(
        root,
        r"^\s*(?:message\s+(?:ManifestDelivery|DeliveryReceipt)\b|type\s+DeliveryState\b)|\b(?:delivery_id|occurrence_id)\s*=\s*\d+\s*;",
        {".go", ".proto"},
    )


def ordinary_policy_matches(root: Path) -> list[Match]:
    # Count declarations and state-machine tokens, not every comment that
    # happens to mention a policy and delivery.  The locations are returned so
    # a failed ceiling is inspectable rather than a synthetic score.
    declarations = re.compile(r"\b(?:PolicyPush|PushPolicy|PolicyState|DeliveryState|DeliveryStatus|StatePending|StatePushed|StateAckedReceipt|delivery_state)\b", re.IGNORECASE)
    states = re.compile(r"\b(?:PENDING|PUSHED|ACKED_RECEIPT)\b", re.IGNORECASE)
    result: list[Match] = []
    for path in files(root, {".go", ".proto", ".sql"}):
        if path.name.endswith("_test.go") or is_generated(root, path):
            continue
        for number, line in enumerate(text(path).splitlines(), 1):
            if declarations.search(line) or (states.search(line) and (re.search(r"delivery|push|receipt", line, re.IGNORECASE) or "deliver" in rel(root, path).lower())):
                result.append(Match(rel(root, path), number, line))
    return result


def legacy_registration_token_matches(root: Path) -> list[Match]:
    """Find enrollment-token state that should not survive the cutover.

    The host bootstrap token is deliberately excluded: it is a separate,
    legitimately single-use credential.  This metric is about registration
    tokens carrying a mutable use counter, a one-time mode, or a human owner.
    """
    legacy = re.compile(
        r"\b(?:one_time|OneTime|oneTime|owner_id|OwnerID|ownerId|"
        r"max_uses_per_agent|MaxUsesPerAgent|maxUsesPerAgent)\b"
    )
    stored_counter = re.compile(r"\bcurrent_uses\b", re.IGNORECASE)
    result: list[Match] = []
    for path in files(root, {".go", ".proto", ".sql", ".svelte", ".ts"}):
        path_name = rel(root, path)
        if path.name.endswith("_test.go") or ".test." in path.name or is_generated(root, path):
            continue
        relevant = (
            "/registrationtoken/" in f"/{path_name}"
            or path_name == "server/internal/enrollment/handlers.go"
            or path_name == "server/internal/store/queries/registration_tokens.sql"
            or path_name == "server/internal/store/reads_tokens.go"
            or path_name.startswith("web/src/routes/(app)/tokens/")
        )
        in_token_block = False
        for number, line in enumerate(text(path).splitlines(), 1):
            if path_name == "server/internal/store/sqliteschema/schema.sql":
                if re.search(r"^\s*CREATE\s+TABLE\s+tokens\b", line, re.IGNORECASE):
                    in_token_block = True
                relevant = in_token_block
            elif path_name == "contract/proto/cadestro/v1/control.proto":
                message = re.search(r"^\s*message\s+(RegistrationToken|CreateTokenRequest)\s*\{", line)
                if message:
                    in_token_block = True
                relevant = in_token_block
            mutable_counter = (
                stored_counter.search(line)
                and path_name in {
                    "server/internal/store/sqliteschema/schema.sql",
                    "server/internal/store/queries/registration_tokens.sql",
                }
                and not re.search(r"\bAS\s+current_uses\b", line, re.IGNORECASE)
            )
            reservation = path.suffix == ".proto" and re.match(r"^\s*reserved\b", line)
            if relevant and not reservation and (legacy.search(line) or mutable_counter):
                result.append(Match(path_name, number, line))
            if in_token_block and line.strip() == "}":
                in_token_block = False
                relevant = False
    return result


def legacy_device_identity_columns(root: Path) -> list[Match]:
    """Count legacy identity/lifecycle state in the effective device schema.

    Counting references made a correct handler refactor look like new durable
    state.  The cutover concern is competing columns, so inspect only the
    canonical schema that a new server actually creates.
    """
    path = root / "server/internal/store/sqliteschema/schema.sql"
    if not path.is_file():
        return []
    in_devices = False
    result: list[Match] = []
    legacy = re.compile(r"^\s*(agent_sealing_public_key|cert_fingerprint|cert_not_after)\b", re.IGNORECASE)
    for number, line in enumerate(text(path).splitlines(), 1):
        if re.search(r"^\s*CREATE\s+TABLE\s+devices\b", line, re.IGNORECASE):
            in_devices = True
            continue
        if in_devices and line.strip().startswith(")"):
            break
        if in_devices and legacy.search(line):
            result.append(Match(rel(root, path), number, line))
    return result


def policy_result_transport_matches(root: Path) -> list[Match]:
    """Find a second result transport invented only for pulled policy."""
    result: list[Match] = []
    duplicate = re.compile(
        r"\b(?:SendPolicyActionResult|SendPolicyManifestResult|IsPolicyResult|work_kind)\b"
    )
    for relative in (
        "contract/client.go",
        "agent/cmd/cadestrod/runtime.go",
        "agent/internal/scheduler/scheduler.go",
        "agent/internal/store/manifest.go",
        "agent/internal/store/migrations/003_policy_manifests.sql",
    ):
        path = root / relative
        if not path.is_file():
            continue
        for number, line in enumerate(text(path).splitlines(), 1):
            if duplicate.search(line):
                result.append(Match(relative, number, line))
    return result


def policy_dispatch_matches(root: Path) -> list[Match]:
    result: list[Match] = []
    declaration = re.compile(r"^\s*func\s+[^\s(]*(?:Resolve|resolve|Effective|Dispatch|dispatch|Submit|submit)[^\s(]*\s*\(")
    for path in files(root, {".go"}):
        path_name = rel(root, path)
        if (path.name.endswith("_test.go") or path.name.endswith("_test.sh")
                or path.name.startswith("test_") or is_generated(root, path)):
            continue
        relevant = any(part in path_name for part in ("/assignment/", "/dispatch/", "/delivery/", "/compliance/"))
        for number, line in enumerate(text(path).splitlines(), 1):
            if declaration.search(line) and (relevant or re.search(r"policy", line, re.IGNORECASE)):
                result.append(Match(path_name, number, line))
    return result


def legacy_definition_compile_matches(root: Path) -> list[Match]:
    """Count only the declarations for the removed Definition compiler paths."""
    path = root / "server/internal/manifest/compiler.go"
    if not path.is_file():
        return []
    declarations = re.compile(
        r"^func \(c \*Compiler\) (?:definition|DefinitionRunbookForDevice)\(ctx context\.Context,"
    )
    return [
        Match(rel(root, path), number, line)
        for number, line in enumerate(text(path).splitlines(), 1)
        if declarations.search(line)
    ]


def assigned_policy_push_matches(root: Path) -> list[Match]:
    """Count the old assigned-policy-to-delivery submission seam."""
    result: list[Match] = []
    for relative in ("server/internal/dispatch/handlers.go", "server/internal/dispatch/assigned.go"):
        path = root / relative
        if not path.is_file():
            continue
        active = False
        for number, line in enumerate(text(path).splitlines(), 1):
            declaration = re.match(r"^func\s+.*\b(DispatchAssignedActions|assignedManifests)\s*\(", line)
            if declaration:
                active = True
                if declaration.group(1) == "assignedManifests":
                    result.append(Match(relative, number, line))
                continue
            if active and re.match(r"^func\s+", line):
                active = False
            if active and re.search(r"\b(?:assignedManifests|ManifestInput|submitter|Submit(?:Batch)?)\b", line):
                result.append(Match(relative, number, line))
    return result


def metric_matches(root: Path) -> dict[str, list[Match]]:
    return {
        "ordinary_policy_push_delivery_types_states": ordinary_policy_matches(root),
        "legacy_registration_token_counter_owner_state": legacy_registration_token_matches(root),
        "legacy_device_identity_columns": legacy_device_identity_columns(root),
        "device_authorization_paths": matches(root, r"(?:AuthorizeContext|EnforceDeviceScope|deviceScopeResolver|authorize\([^\n]*deviceID)", {".go"}),
        "manifest_delivery_protocol_types_fields": delivery_protocol_matches(root),
        "delivery_manifest_occurrence_durable_tables_columns": sql_durable_matches(root),
        "agent_scheduled_work_tables": agent_scheduled_work_schema(root),
        "policy_resolver_dispatch_entry_points": policy_dispatch_matches(root),
        "legacy_definition_compile_paths": legacy_definition_compile_matches(root),
        "assigned_policy_push_submission_coupling": assigned_policy_push_matches(root),
        "runtime_package_fanout_coupling": runtime_import_matches(root),
        "process_global_executor_managers": executor_global_matches(root),
        "policy_specific_result_transport_paths": policy_result_transport_matches(root),
        "stale_field_sealing_protocol_machinery": matches(root, r"\b(?:fieldSealVersion|sealedFieldVersion|protocolVersion|wireProtocol)\b", {".go", ".proto"}),
        "certificate_lifecycle_junk": certificate_lifecycle_junk_matches(root),
    }


def executor_global_matches(root: Path) -> list[Match]:
    result: list[Match] = []
    declaration = re.compile(r"^\s*(?:var|const)\s+(\w*(?:Mgr|Manager|executorRunner)\w*)\b")
    grouped = re.compile(r"^\s*(\w*(?:Mgr|Manager|executorRunner)\w*)\s*(?:=|$)")
    for path in files(root, {".go"}):
        path_name = rel(root, path)
        if path.name.endswith("_test.go") or is_generated(root, path) or "/executor/" not in f"/{path_name}":
            continue
        in_var_block = False
        for number, line in enumerate(text(path).splitlines(), 1):
            if re.match(r"^\s*var\s*\(\s*$", line):
                in_var_block = True
                continue
            if in_var_block and re.match(r"^\s*\)\s*$", line):
                in_var_block = False
                continue
            match = grouped.search(line) if in_var_block else declaration.search(line)
            if match:
                result.append(Match(path_name, number, line))
    return result


def runtime_import_matches(root: Path) -> list[Match]:
    result: list[Match] = []
    runtime = root / "server" / "internal" / "controlruntime"
    if not runtime.is_dir():
        return result
    imports: set[str] = set()
    for path in runtime.glob("*.go"):
        for number, line in enumerate(text(path).splitlines(), 1):
            if "server/internal/" in line:
                package = line.split("server/internal/", 1)[1].split('"', 1)[0].split("/", 1)[0]
                if package and package not in imports:
                    imports.add(package)
                    result.append(Match(rel(root, path), number, f"import server/internal/{package}"))
    return result


def certificate_lifecycle_junk_matches(root: Path) -> list[Match]:
    """Find removed renewal/revocation machinery, excluding deletion paths."""
    pattern = re.compile(r"\b(?:revoked_certificates|RevocationChecker|RevokeInTx|startCertRotation|current_certificate|ca_certificate|CATrustBundle|SetTrustBundle|TrustBundle|AssertCSRMatchesCertKey|PeerClassFromPEM|ReplaceDeviceCertificate|ClearPendingDeviceCertificate)\b|CA_TRUST_BUNDLE_FILE|ca-trust-bundle\.crt")
    result: list[Match] = []
    for path in files(root, {".go", ".proto", ".sql", ".sh", ".yml", ".yaml"}):
        relative = rel(root, path)
        if (path.name.endswith("_test.go") or path.name.endswith("_test.sh")
                or path.name.startswith("test_") or is_generated(root, path)):
            continue
        if path.suffix == ".sql" and "revoked_certificates" not in relative:
            continue
        if path.suffix == ".proto" and "control.proto" not in relative:
            continue
        if path.suffix == ".go" and not ("enrollment/handlers.go" in relative or "cert_rotation.go" in relative or "revoked_certificates" in relative or "/ca/" in relative or relative.endswith("cmd/cadestro/config.go") or relative.endswith("cmd/cadestro/main.go" ) ):
            continue
        if path.suffix in {".sh", ".yml", ".yaml"} and "server/deploy/" not in relative:
            continue
        for number, line in enumerate(text(path).splitlines(), 1):
            if pattern.search(line):
                result.append(Match(relative, number, line))
    return result


def simplification_report(root: Path, baseline_counts: dict[str, int]) -> dict[str, object]:
    found = metric_matches(root)
    current = {name: len(items) for name, items in found.items()}
    metrics: dict[str, object] = {}
    for name, count in current.items():
        baseline_count = baseline_counts[name]
        metrics[name] = {
            "baseline": baseline_count,
            "candidate": count,
            "delta": count - baseline_count,
            "ceiling": baseline_count,
            "matches": [item.as_dict() for item in found[name]],
            "pass": count <= baseline_count,
        }
    # Completed cutovers are hard zeroes, not merely improvements over the
    # archive. Otherwise half of a removed mechanism could quietly grow back
    # while the aggregate scoreboard still passed.
    zero = {
        "process_global_executor_managers": current["process_global_executor_managers"] == 0,
        "policy_specific_result_transport_paths": current["policy_specific_result_transport_paths"] == 0,
        "legacy_registration_token_counter_owner_state": current["legacy_registration_token_counter_owner_state"] == 0,
        "assigned_policy_push_submission_coupling": current["assigned_policy_push_submission_coupling"] == 0,
        "legacy_definition_compile_paths": current["legacy_definition_compile_paths"] == 0,
        "certificate_lifecycle_junk": current["certificate_lifecycle_junk"] == 0,
    }
    for name, passed in zero.items():
        metrics[name]["zero_invariant"] = True
        metrics[name]["pass"] = bool(metrics[name]["pass"] and passed)
    deltas = {name: value["delta"] for name, value in metrics.items()}
    improved = any(delta < 0 for delta in deltas.values())
    legacy_detected = any(value > 0 for value in baseline_counts.values())
    return {"baseline_counts": baseline_counts, "candidate_counts": current, "metrics": metrics, "legacy_detected": legacy_detected, "improved": improved, "pass": all(bool(value["pass"]) for value in metrics.values()) and improved}


def materialize(repo: Path, ref: str) -> tuple[Path, tempfile.TemporaryDirectory[str] | None]:
    candidate_path = Path(ref).expanduser()
    if candidate_path.is_dir():
        return candidate_path.resolve(), None
    if ref in {".", "WORKTREE", "working-tree"}:
        return repo, None
    temp = tempfile.TemporaryDirectory(prefix="cadestro-cutover-")
    archive = subprocess.run(["git", "-C", str(repo), "archive", "--format=tar", ref], stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
    if archive.returncode != 0:
        temp.cleanup()
        raise JudgeError(f"cannot archive ref {ref}: {archive.stderr.decode(errors='replace').strip()}")
    with tarfile.open(fileobj=io.BytesIO(archive.stdout), mode="r:") as tar:
        destination = Path(temp.name).resolve()
        for member in tar.getmembers():
            if member.issym() or member.islnk():
                temp.cleanup()
                raise JudgeError(f"git archive contained an unsupported link: {member.name}")
            target = (destination / member.name).resolve()
            if target != destination and destination not in target.parents:
                temp.cleanup()
                raise JudgeError(f"git archive contained an unsafe path: {member.name}")
        tar.extractall(temp.name)
    return Path(temp.name), temp


def run(args: argparse.Namespace) -> tuple[dict[str, object], int]:
    repo = Path(args.repo).resolve()
    baseline_root, baseline_temp = materialize(repo, args.baseline)
    candidate_root, candidate_temp = materialize(repo, args.candidate)
    try:
        baseline_features = feature_inventory(baseline_root)
        candidate_features = feature_inventory(candidate_root, require_nonempty=False)
        baseline_implementation = implementation_inventory(baseline_root, require_nonempty=True)
        candidate_implementation = implementation_inventory(candidate_root)
        exceptions_path = Path(args.exceptions) if args.exceptions else None
        feature = compare_features(baseline_features, candidate_features, load_exceptions(exceptions_path))
        baseline_counts = {name: len(items) for name, items in metric_matches(baseline_root).items()}
        simplify = simplification_report(candidate_root, baseline_counts)
        report: dict[str, object] = {
            "baseline": args.baseline,
            "candidate": args.candidate,
            "feature_preservation": feature,
            "implementation_inventory": {
                "baseline": baseline_implementation,
                "candidate": candidate_implementation,
                "removed_domains": sorted(set(sum(baseline_implementation.values(), [])) - set(sum(candidate_implementation.values(), []))),
                "note": "informational: package/domain names are not feature-preservation gates",
            },
            "implementation_simplification": simplify,
            "pass": bool(feature["pass"] and simplify["pass"]),
        }
        return report, 0 if report["pass"] else 1
    finally:
        if baseline_temp:
            baseline_temp.cleanup()
        if candidate_temp:
            candidate_temp.cleanup()


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", default=".", help="repository to inspect")
    parser.add_argument("--baseline", default="archive/cadestro-before-foundation-20260816", help="baseline git ref")
    parser.add_argument("--candidate", default="WORKTREE", help="candidate git ref, WORKTREE, or .")
    parser.add_argument("--exceptions", help="JSON file containing deliberate feature rename/merge mappings")
    parser.add_argument("--json", action="store_true", help="emit machine-readable JSON")
    args = parser.parse_args(argv)
    try:
        report, code = run(args)
    except JudgeError as exc:
        payload = {"pass": False, "error": str(exc)}
        if args.json:
            print(json.dumps(payload, indent=2, sort_keys=True))
        else:
            print(f"FAIL: {exc}", file=sys.stderr)
        return 2
    if args.json:
        print(json.dumps(report, indent=2, sort_keys=True))
    else:
        feature = report["feature_preservation"]
        simplify = report["implementation_simplification"]
        print(f"cutover judge: {'PASS' if report['pass'] else 'FAIL'}")
        print(f"features: {'PASS' if feature['pass'] else 'FAIL'}; unexplained loss={len(feature['unexplained_missing'])}")
        print(f"simplification: {'PASS' if simplify['pass'] else 'FAIL'}; changed metrics={sum(v != 0 for v in (simplify['candidate_counts'][k] - simplify['baseline_counts'][k] for k in simplify['baseline_counts']))}")
        print("metrics: " + ", ".join(f"{k}={simplify['candidate_counts'][k]} (Δ{simplify['candidate_counts'][k]-simplify['baseline_counts'][k]:+d})" for k in simplify["baseline_counts"]))
    return code


if __name__ == "__main__":
    raise SystemExit(main())

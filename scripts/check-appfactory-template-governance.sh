#!/usr/bin/env bash

set -euo pipefail

template_ref="${APPFACTORY_TEMPLATE_GOVERNANCE_TEMPLATE_DIR:-${1:-flutter-finance-lite}}"
governance_root="${APPFACTORY_TEMPLATE_GOVERNANCE_ROOT:-${2:-workspace/appfactory/template-governance}}"
latest_json_path="${APPFACTORY_TEMPLATE_GOVERNANCE_LATEST_JSON:-${governance_root}/latest.json}"
latest_markdown_path="${APPFACTORY_TEMPLATE_GOVERNANCE_LATEST_MARKDOWN:-${governance_root}/latest.md}"
cache_root="${APPFACTORY_TEMPLATE_GOVERNANCE_CACHE_ROOT:-workspace/appfactory/builder-cache/template-verify}"
builder_image="${ONEAPPFACTORY_BUILDER_IMAGE:-oneappfactory/builder:local}"
governance_source_mode="${APPFACTORY_TEMPLATE_GOVERNANCE_SOURCE:-image}"
template_root_in_image="${APPFACTORY_TEMPLATE_ROOT_IN_IMAGE:-/opt/appfactory/templates}"
repo_root_in_image="${APPFACTORY_REPO_ROOT_IN_IMAGE:-/opt/appfactory}"
repo_root="$(cd "$(dirname "$0")/.." && pwd)"

abs_path() {
  local target="$1"
  if [[ "$target" = /* ]]; then
    printf '%s\n' "$target"
  else
    printf '%s/%s\n' "$PWD" "${target#./}"
  fi
}

resolve_host_template_dir() {
  local ref="$1"
  if [ -d "$ref" ]; then
    printf '%s\n' "$ref"
    return
  fi
  if [ -d "$PWD/$ref" ]; then
    printf '%s\n' "$PWD/$ref"
    return
  fi
  if [ -d "$PWD/examples/appfactory/templates/$ref" ]; then
    printf '%s\n' "$PWD/examples/appfactory/templates/$ref"
    return
  fi
  return 1
}

resolve_image_template_dir() {
  local ref="$1"
  if [[ "$ref" = /* ]]; then
    printf '%s\n' "$ref"
    return
  fi
  ref="${ref#./}"
  ref="${ref#examples/appfactory/templates/}"
  printf '%s\n' "$template_root_in_image/$ref"
}

map_to_container_repo_path() {
  local host_path="$1"
  local repo_root_abs="$2"
  local rel
  case "$host_path" in
    "$repo_root_abs")
      printf '%s\n' /workspace/host
      ;;
    "$repo_root_abs"/*)
      rel="${host_path#"$repo_root_abs"/}"
      printf '%s/%s\n' /workspace/host "$rel"
      ;;
    *)
      echo "image mode only supports paths under repo root: $host_path" >&2
      exit 1
      ;;
  esac
}

case "$governance_source_mode" in
  image|host)
    ;;
  *)
    echo "unsupported template governance source: $governance_source_mode" >&2
    echo "expected one of: image, host" >&2
    exit 1
    ;;
esac

repo_root_abs="$(abs_path "$repo_root")"
governance_root_abs="$(abs_path "$governance_root")"
latest_json_abs="$(abs_path "$latest_json_path")"
latest_markdown_abs="$(abs_path "$latest_markdown_path")"
cache_root_abs="$(abs_path "$cache_root")"
mkdir -p "$governance_root_abs" "$cache_root_abs"

python_script_path="$(mktemp "$cache_root_abs/template-governance.XXXXXX.py")"
cleanup() {
  rm -f "$python_script_path"
}
trap cleanup EXIT

cat <<'PY' > "$python_script_path"
import json
import re
import sys
from datetime import datetime, timezone
from pathlib import Path


def parse_pubspec_dependencies(path: Path):
    sections = {"dependencies": {}, "dev_dependencies": {}}
    current_section = None
    current_dep = None
    current_nested = None
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.rstrip("\n")
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue
        if not line.startswith(" "):
            key = stripped.split(":", 1)[0].strip()
            current_section = key if key in sections else None
            current_dep = None
            current_nested = None
            continue
        if not current_section:
            continue
        if line.startswith("  ") and not line.startswith("    ") and ":" in stripped:
            name, rest = stripped.split(":", 1)
            current_dep = name.strip()
            sections[current_section][current_dep] = {
                "constraint": rest.strip() or None,
                "declared_source": "hosted" if rest.strip() else None,
            }
            current_nested = None
            continue
        if current_dep and line.startswith("    ") and ":" in stripped:
            subkey, rest = stripped.split(":", 1)
            subkey = subkey.strip()
            rest = rest.strip()
            if subkey == "sdk":
                sections[current_section][current_dep]["constraint"] = f"sdk:{rest}"
                sections[current_section][current_dep]["declared_source"] = "sdk"
            elif subkey == "path":
                sections[current_section][current_dep]["declared_source"] = "path"
                sections[current_section][current_dep]["declared_path"] = rest or None
            elif subkey == "git":
                sections[current_section][current_dep]["declared_source"] = "git"
                if rest:
                    sections[current_section][current_dep]["git_url"] = rest
                current_nested = "git"
            elif subkey == "hosted":
                sections[current_section][current_dep]["declared_source"] = "hosted"
                current_nested = "hosted"
            elif subkey == "version":
                sections[current_section][current_dep]["constraint"] = rest or None
            elif subkey == "url" and current_nested == "hosted":
                sections[current_section][current_dep]["hosted_url"] = rest or None
            elif subkey == "ref" and current_nested == "git":
                sections[current_section][current_dep]["git_ref"] = rest or None
            elif subkey == "url" and current_nested == "git":
                sections[current_section][current_dep]["git_url"] = rest or None
            elif subkey == "path" and current_nested == "git":
                sections[current_section][current_dep]["git_path"] = rest or None
            elif subkey == "tag" and current_nested == "git":
                sections[current_section][current_dep]["git_tag"] = rest or None
            else:
                current_nested = subkey
    return sections


def parse_pubspec_lock(path: Path):
    packages = {}
    current_name = None
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.rstrip("\n")
        if re.match(r"^  [A-Za-z0-9_+\-.]+:$", line):
            current_name = line.strip().rstrip(":")
            packages[current_name] = {}
            continue
        if not current_name:
            continue
        stripped = line.strip()
        if stripped.startswith("dependency:"):
            packages[current_name]["dependency"] = stripped.split(":", 1)[1].strip().strip('"')
        elif stripped.startswith("source:"):
            packages[current_name]["source"] = stripped.split(":", 1)[1].strip().strip('"')
        elif stripped.startswith("version:"):
            packages[current_name]["version"] = stripped.split(":", 1)[1].strip().strip('"')
        elif stripped.startswith("url:"):
            packages[current_name]["url"] = stripped.split(":", 1)[1].strip().strip('"')
    return packages


def find_nearest_evidence(start: Path, repo_root: Path, patterns):
    current = start
    while True:
        for pattern in patterns:
            matches = sorted(current.glob(pattern))
            if matches:
                return matches[0]
        if current == repo_root or current.parent == current:
            break
        current = current.parent
    return None


def collect_manifest_permissions(path: Path):
    if not path.is_file():
        return []
    text = path.read_text(encoding="utf-8")
    return sorted(set(re.findall(r'uses-permission[^>]*android:name="([^"]+)"', text)))


def collect_license_evidence(path: Path):
    if not path.is_dir():
        return None
    for pattern in ("LICENSE", "LICENSE*", "COPYING", "COPYING*", "NOTICE", "NOTICE*"):
        matches = sorted(path.glob(pattern))
        if matches:
            return matches[0]
    return None


def evaluate_constraint(constraint):
    if not constraint:
        return "missing direct dependency constraint"
    normalized = str(constraint).strip()
    if not normalized or normalized.startswith("sdk:"):
        return None
    if normalized in {"any", "*"}:
        return f"loose direct dependency constraint: {normalized}"
    if normalized.startswith(">") or normalized.startswith("<"):
        return f"range-style direct dependency constraint requires manual review: {normalized}"
    return None


RISK_PATTERNS = {
    "payment": ["stripe", "paypal", "alipay", "wechat_pay", "google_pay", "in_app_purchase", "revenuecat", "purchases"],
    "ads_tracking": ["admob", "ads", "advert", "facebook_audience", "unity_ads", "appsflyer", "adjust"],
    "push": ["firebase_messaging", "onesignal", "getui", "jpush", "push"],
    "maps_location": ["google_maps", "mapbox", "amap", "geolocator", "geocoding", "location"],
    "realtime_communication": ["web_socket", "socket_io", "agora", "jitsi", "webrtc", "mqtt"],
    "auth_account": ["firebase_auth", "google_sign_in", "sign_in_with_apple", "oauth", "auth0"],
    "sensitive_native": ["camera", "contacts", "sms", "microphone", "bluetooth", "biometric"],
}

RISK_PERMISSIONS = {
    "android.permission.ACCESS_FINE_LOCATION": "maps_location",
    "android.permission.ACCESS_COARSE_LOCATION": "maps_location",
    "android.permission.CAMERA": "sensitive_native",
    "android.permission.RECORD_AUDIO": "sensitive_native",
    "android.permission.READ_CONTACTS": "sensitive_native",
    "android.permission.WRITE_CONTACTS": "sensitive_native",
    "android.permission.READ_SMS": "sensitive_native",
    "android.permission.RECEIVE_SMS": "sensitive_native",
    "android.permission.SEND_SMS": "sensitive_native",
    "android.permission.READ_PHONE_STATE": "sensitive_native",
    "android.permission.SYSTEM_ALERT_WINDOW": "sensitive_native",
    "android.permission.QUERY_ALL_PACKAGES": "sensitive_native",
    "android.permission.POST_NOTIFICATIONS": "push",
}

template_dir = Path(sys.argv[1]).expanduser().resolve()
governance_root = Path(sys.argv[2]).expanduser()
latest_json_path = Path(sys.argv[3]).expanduser()
latest_markdown_path = Path(sys.argv[4]).expanduser()
repo_root = Path(sys.argv[5]).expanduser().resolve()
cache_root = Path(sys.argv[6]).expanduser()

if not template_dir.is_dir():
    raise SystemExit(f"template directory not found: {template_dir}")

run_id = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
template_slug = template_dir.name.replace(" ", "-")
run_dir = governance_root / "runs" / f"{run_id}-{template_slug}"
run_dir.mkdir(parents=True, exist_ok=True)
result_json_path = run_dir / "template-governance-result.json"
result_markdown_path = run_dir / "template-governance-result.md"

pubspec_path = template_dir / "pubspec.yaml"
pubspec_lock_path = template_dir / "pubspec.lock"
manifest_path = template_dir / "android" / "app" / "src" / "main" / "AndroidManifest.xml"
readme_path = template_dir / "README.md"
license_path = find_nearest_evidence(template_dir, repo_root, ["LICENSE", "LICENSE*", "COPYING", "COPYING*"])

deps = parse_pubspec_dependencies(pubspec_path) if pubspec_path.is_file() else {"dependencies": {}, "dev_dependencies": {}}
lock_packages = parse_pubspec_lock(pubspec_lock_path) if pubspec_lock_path.is_file() else {}
permissions = collect_manifest_permissions(manifest_path)
pub_cache_root = cache_root / "pub-cache" / "hosted" / "pub.dev"

dependency_rows = []
for section_name in ("dependencies", "dev_dependencies"):
    for name, payload in sorted(deps.get(section_name, {}).items()):
        lock_payload = lock_packages.get(name, {})
        dependency_rows.append(
            {
                "name": name,
                "section": section_name,
                "constraint": payload.get("constraint"),
                "declared_source": payload.get("declared_source"),
                "declared_path": payload.get("declared_path"),
                "git_url": payload.get("git_url"),
                "git_ref": payload.get("git_ref"),
                "version": lock_payload.get("version"),
                "source": lock_payload.get("source"),
                "url": lock_payload.get("url"),
            }
        )

transitive_dependency_rows = []
transitive_dependency_risks = []
transitive_license_warnings = []
for name, payload in sorted(lock_packages.items()):
    if payload.get("dependency") != "transitive":
        continue
    license_evidence_path = None
    if payload.get("source") == "hosted" and payload.get("version"):
        package_dir = pub_cache_root / f"{name}-{payload.get('version')}"
        license_evidence = collect_license_evidence(package_dir)
        license_evidence_path = license_evidence.as_posix() if license_evidence else None
        if package_dir.exists() and not license_evidence:
            transitive_license_warnings.append(
                {
                    "name": name,
                    "version": payload.get("version"),
                    "message": "missing transitive license evidence in hydrated pub cache",
                }
            )
    transitive_dependency_rows.append(
        {
            "name": name,
            "dependency": payload.get("dependency"),
            "version": payload.get("version"),
            "source": payload.get("source"),
            "url": payload.get("url"),
            "license_evidence_path": license_evidence_path,
        }
    )
    lowered = name.lower()
    for category, patterns in RISK_PATTERNS.items():
        if any(pattern in lowered for pattern in patterns):
            transitive_dependency_risks.append(
                {
                    "kind": "transitive_dependency",
                    "category": category,
                    "name": name,
                }
            )

dependency_risks = []
for item in dependency_rows:
    lowered = item["name"].lower()
    for category, patterns in RISK_PATTERNS.items():
        if any(pattern in lowered for pattern in patterns):
            dependency_risks.append(
                {
                    "kind": "dependency",
                    "category": category,
                    "name": item["name"],
                    "section": item["section"],
                }
            )

permission_risks = []
for permission in permissions:
    category = RISK_PERMISSIONS.get(permission)
    if category:
        permission_risks.append(
            {
                "kind": "permission",
                "category": category,
                "name": permission,
            }
        )

policy_violations = []
constraint_warnings = []
for item in dependency_rows:
    constraint_warning = evaluate_constraint(item.get("constraint"))
    if constraint_warning:
        constraint_warnings.append(
            {
                "kind": "constraint",
                "name": item["name"],
                "section": item["section"],
                "message": constraint_warning,
            }
        )
    declared_source = item.get("declared_source")
    if declared_source == "path":
        policy_violations.append(
            {
                "kind": "dependency_source",
                "name": item["name"],
                "section": item["section"],
                "message": "path dependency is not allowed in internal trial baseline",
            }
        )
    elif declared_source == "git":
        policy_violations.append(
            {
                "kind": "dependency_source",
                "name": item["name"],
                "section": item["section"],
                "message": "git dependency requires explicit vendoring or hosted mirror before internal trial",
            }
        )
    elif item.get("source") and item["source"] not in {"hosted", "sdk"}:
        policy_violations.append(
            {
                "kind": "lock_source",
                "name": item["name"],
                "section": item["section"],
                "message": f"non-standard lock source requires governance review: {item['source']}",
            }
        )

warnings = []
if license_path is None:
    policy_violations.append(
        {
            "kind": "license",
            "name": "template",
            "section": "metadata",
            "message": "missing license evidence file",
        }
    )
if not readme_path.is_file():
    warnings.append("missing template README.md")
if not pubspec_lock_path.is_file():
    warnings.append("missing pubspec.lock")
if not pub_cache_root.is_dir():
    warnings.append("template verify pub-cache not hydrated; transitive license evidence may be incomplete")

status = "passed"
if dependency_risks or transitive_dependency_risks or permission_risks or policy_violations:
    status = "failed"
elif warnings or constraint_warnings or transitive_license_warnings:
    status = "warning"

payload = {
    "generated_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    "run_id": f"{run_id}-{template_slug}",
    "template_dir": template_dir.as_posix(),
    "run_dir": run_dir.as_posix(),
    "status": status,
    "license_evidence_path": license_path.as_posix() if license_path else None,
    "readme_path": readme_path.as_posix() if readme_path.is_file() else None,
    "pubspec_path": pubspec_path.as_posix() if pubspec_path.is_file() else None,
    "pubspec_lock_path": pubspec_lock_path.as_posix() if pubspec_lock_path.is_file() else None,
    "android_manifest_path": manifest_path.as_posix() if manifest_path.is_file() else None,
    "dependency_rows": dependency_rows,
    "dependency_count": len(dependency_rows),
    "transitive_dependency_rows": transitive_dependency_rows,
    "transitive_dependency_count": len(transitive_dependency_rows),
    "permissions": permissions,
    "dependency_risks": dependency_risks,
    "transitive_dependency_risks": transitive_dependency_risks,
    "permission_risks": permission_risks,
    "policy_violations": policy_violations,
    "constraint_warnings": constraint_warnings,
    "transitive_license_warnings": transitive_license_warnings,
    "warnings": warnings,
    "manual_review_required": bool(dependency_risks or transitive_dependency_risks or permission_risks or policy_violations or warnings or constraint_warnings or transitive_license_warnings),
    "recommended_commands": [
        "make verify-appfactory-template-fast",
        "make verify-appfactory-template",
    ],
}

result_json_path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
latest_json_path.parent.mkdir(parents=True, exist_ok=True)
latest_json_path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

lines = [
    "# AppFactory 模板治理检查",
    "",
    f"- generated_at: {payload['generated_at']}",
    f"- run_id: {payload['run_id']}",
    f"- status: {payload['status']}",
    f"- template_dir: {payload['template_dir']}",
    f"- license_evidence_path: {payload['license_evidence_path'] or '-'}",
    f"- readme_path: {payload['readme_path'] or '-'}",
    f"- dependency_count: {payload['dependency_count']}",
    f"- transitive_dependency_count: {payload['transitive_dependency_count']}",
    "",
    "## Recommended Commands",
    "",
]
for command in payload["recommended_commands"]:
    lines.append(f"- {command}")

lines.extend(["", "## Direct Dependencies", "", "| name | section | constraint | version | source |", "|---|---|---|---|---|"])
for item in dependency_rows:
    lines.append(
        f"| {item['name']} | {item['section']} | {item.get('constraint') or '-'} | {item.get('version') or '-'} | {item.get('source') or '-'} |"
    )

lines.extend(["", "## Manifest Permissions", ""])
if permissions:
    for permission in permissions:
        lines.append(f"- {permission}")
else:
    lines.append("- none")

lines.extend(["", "## Risk Hits", ""])
if dependency_risks or transitive_dependency_risks or permission_risks or policy_violations:
    for item in dependency_risks + transitive_dependency_risks + permission_risks + policy_violations:
        if item.get("message"):
            lines.append(f"- {item['kind']} -> {item['name']}: {item['message']}")
            continue
        lines.append(f"- {item['kind']}:{item['category']} -> {item['name']}")
else:
    lines.append("- none")

lines.extend(["", "## Transitive Dependency License Warnings", ""])
if transitive_license_warnings:
    for item in transitive_license_warnings[:20]:
        lines.append(f"- {item['name']} {item.get('version') or '-'}: {item['message']}")
else:
    lines.append("- none")

lines.extend(["", "## Transitive Dependencies", "", "| name | dependency | version | source | license_evidence |", "|---|---|---|---|---|"])
for item in transitive_dependency_rows[:40]:
    lines.append(
        f"| {item['name']} | {item.get('dependency') or '-'} | {item.get('version') or '-'} | {item.get('source') or '-'} | {item.get('license_evidence_path') or '-'} |"
    )

lines.extend(["", "## Constraint Warnings", ""])
if constraint_warnings:
    for item in constraint_warnings:
        lines.append(f"- {item['name']} ({item['section']}): {item['message']}")
else:
    lines.append("- none")

lines.extend(["", "## Warnings", ""])
if warnings:
    for warning in warnings:
        lines.append(f"- {warning}")
else:
    lines.append("- none")

markdown = "\n".join(lines) + "\n"
result_markdown_path.write_text(markdown, encoding="utf-8")
latest_markdown_path.parent.mkdir(parents=True, exist_ok=True)
latest_markdown_path.write_text(markdown, encoding="utf-8")

print(f"template_governance_status={status}")
print(f"template_governance_transitive_dependency_count={len(transitive_dependency_rows)}")
print(f"template_governance_transitive_license_warning_count={len(transitive_license_warnings)}")
print(f"template_governance_result_json={result_json_path.as_posix()}")
print(f"template_governance_latest_json={latest_json_path.as_posix()}")
print(f"template_governance_latest_markdown={latest_markdown_path.as_posix()}")
if dependency_risks or transitive_dependency_risks or permission_risks or policy_violations:
    sys.exit(1)
PY

container_template_dir=""
if [ "$governance_source_mode" = "host" ]; then
  if ! command -v python3 >/dev/null 2>&1; then
    echo "python3 is required in host mode" >&2
    exit 1
  fi
  if ! template_dir="$(resolve_host_template_dir "$template_ref")"; then
    echo "template directory not found on host: $template_ref" >&2
    exit 1
  fi
  python3 "$python_script_path" "$template_dir" "$governance_root_abs" "$latest_json_abs" "$latest_markdown_abs" "$repo_root_abs" "$cache_root_abs"
  exit 0
fi

if ! docker image inspect "$builder_image" >/dev/null 2>&1; then
  echo "builder image not found: $builder_image" >&2
  echo "run 'make build-appfactory-builder' first or set ONEAPPFACTORY_BUILDER_IMAGE to an existing image" >&2
  exit 1
fi

container_template_dir="$(resolve_image_template_dir "$template_ref")"
container_governance_root="$(map_to_container_repo_path "$governance_root_abs" "$repo_root_abs")"
container_latest_json="$(map_to_container_repo_path "$latest_json_abs" "$repo_root_abs")"
container_latest_markdown="$(map_to_container_repo_path "$latest_markdown_abs" "$repo_root_abs")"
container_cache_root="$(map_to_container_repo_path "$cache_root_abs" "$repo_root_abs")"

docker run --rm \
  -v "$repo_root_abs:/workspace/host" \
  -v "$python_script_path:/workspace/check-template-governance.py:ro" \
  "$builder_image" \
  exec python3 /workspace/check-template-governance.py \
    "$container_template_dir" \
    "$container_governance_root" \
    "$container_latest_json" \
    "$container_latest_markdown" \
    "$repo_root_in_image" \
    "$container_cache_root"
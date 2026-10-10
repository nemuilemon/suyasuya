#!/usr/bin/env bash
set -euo pipefail
if [[ $# != 1 ]]; then
  echo "Usage: $0 KUBECTL_CONTEXT" >&2
  exit 2
fi
repo_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
secret_dir="$repo_dir/deploy/secrets"
# Read as data, never source a file containing credentials as shell code.
python3 - "$secret_dir" <<'PY'
import sys
from pathlib import Path
root = Path(sys.argv[1])
keys = {'postgres': {'POSTGRES_PASSWORD', 'DB_CONTROL_PASSWORD', 'DB_AGENT_PASSWORD'},
        'control': {'DB_CONTROL_PASSWORD', 'APP_PASSWORD'},
        'agent': {'DB_AGENT_PASSWORD', 'GEMINI_API_KEY'}}
values = {}
for name, expected in keys.items():
    path = root / (name + '.env')
    if not path.is_file():
        sys.exit(f'Missing {name}.env')
    data = {}
    for line in path.read_text().splitlines():
        if not line or line.startswith('#'):
            continue
        key, sep, value = line.partition('=')
        if not sep or key not in expected or key in data or not value or value.startswith('replace_'):
            sys.exit(f'Invalid or missing setting in {name}.env (values omitted)')
        if key.endswith('PASSWORD') and len(value) < 12:
            sys.exit(f'{name}.env: passwords must have at least 12 characters')
        data[key] = value
    if data.keys() != expected:
        sys.exit(f'Unexpected or missing keys in {name}.env')
    values[name] = data
cf = root / 'cloudflared.env'
if cf.is_file():
    lines = [l for l in cf.read_text().splitlines() if l and not l.startswith('#')]
    key, sep, value = (lines[0].partition('=') if len(lines) == 1 else ('', '', ''))
    if key != 'TUNNEL_TOKEN' or not sep or len(value) < 100 or value.startswith('replace_'):
        sys.exit('Invalid cloudflared.env: expected one TUNNEL_TOKEN= line (value omitted)')
for component, key in [('control', 'DB_CONTROL_PASSWORD'), ('agent', 'DB_AGENT_PASSWORD')]:
    if values['postgres'][key] != values[component][key]:
        sys.exit(f'{key} must match in postgres.env and {component}.env')
PY
kubectl --context "$1" create namespace suyasuya --dry-run=client -o yaml |
  kubectl --context "$1" apply --server-side -f -
components=(postgres control agent)
# cloudflared.env is optional: only the GKE overlay runs cloudflared.
[[ -f "$secret_dir/cloudflared.env" ]] && components+=(cloudflared)
for component in "${components[@]}"; do
  kubectl --context "$1" -n suyasuya create secret generic "$component-secrets" \
    --from-env-file="$secret_dir/$component.env" --dry-run=client -o yaml |
    kubectl --context "$1" -n suyasuya apply --server-side -f -
done

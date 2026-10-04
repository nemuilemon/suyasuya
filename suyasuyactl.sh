#!/usr/bin/env bash
#
# suyasuyactl — local dev (docker compose) と本番 GKE の操作をまとめる
#
#   ./suyasuyactl.sh dev <command> [args]
#   ./suyasuyactl.sh gke <command> [args]
#
# どこから実行してもよい（repo root に cd してから動く）。
# 手順の正本は README.md と deploy/README.md。ここはそれをなぞるだけで、新しい手順は持たない。
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

# --- 本番（GKE） --------------------------------------------------------------
KUBE_CONTEXT="${KUBE_CONTEXT:-gke_suyasuya-prod_asia-northeast1_suyasuya}"
REGISTRY="${REGISTRY:-asia-northeast1-docker.pkg.dev/suyasuya-prod/suyasuya}"
OVERLAY="deploy/overlays/gke"
NS="suyasuya"
PLATFORM="${PLATFORM:-linux/amd64}"

info() { printf '\033[34m[suyasuyactl]\033[0m %s\n' "$*"; }
warn() { printf '\033[33m[suyasuyactl]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[31m[suyasuyactl]\033[0m %s\n' "$*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "$1 が見つからない"; }

# 本番に触る操作の前に止まる。SUYASUYA_YES=1 で省略できる。
confirm() {
  [[ "${SUYASUYA_YES:-}" == 1 ]] && return 0
  read -r -p "$1 [y/N] " ans
  [[ "$ans" == y || "$ans" == Y ]] || die "中止した"
}

# migration は docker/NN-*.sql に限る（任意の SQL を管理者で流さない）
migration_file() {
  local f="${1:-}"
  [[ -n "$f" ]] || die "migration の file を指定する（例: docker/92-agent-steps.sql）"
  [[ "$f" =~ ^docker/[0-9]{2}-[a-z0-9-]+\.sql$ ]] || die "docker/NN-*.sql だけを受け付ける: $f"
  [[ -f "$f" ]] || die "見つからない: $f"
  printf '%s' "$f"
}

# --- dev（docker compose） ----------------------------------------------------
dc() {
  [[ -f .env ]] || die ".env が無い。.env.sample をコピーして埋める（README「MVP の起動」）"
  docker compose "$@"
}

dev_psql_admin() {
  dc exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' "$@"
}

dev() {
  local cmd="${1:-help}"; shift || true
  case "$cmd" in
    up)      dc up --build -d "$@"
             info "http://localhost:${APP_PORT:-8080}" ;;
    down)    dc down "$@" ;;               # -v は記録ごと消すので付けない（README）
    restart) dc restart "$@" ;;
    build)   dc build "$@" ;;
    ps|status) dc ps ;;
    logs)    [[ $# -gt 0 ]] || set -- control agent
             dc logs -f --tail=100 "$@" ;;
    psql)    dc exec postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"' ;;
    migrate) local f; f="$(migration_file "${1:-}")"
             info "dev DB に $f を適用"
             dev_psql_admin < "$f" ;;
    test)    need go
             info "go vet / go test"
             go vet ./... && go test ./...
             info "schema test（dev DB、ROLLBACK するのでデータは残らない）"
             dev_psql_admin < tests/schema.sql
             info "OK" ;;
    *) usage ;;
  esac
}

# --- gke（本番） --------------------------------------------------------------
kc() { kubectl --context "$KUBE_CONTEXT" -n "$NS" "$@"; }

# 今の overlay に書いてある control の tag
current_tag() {
  awk '/name: suyasuya-control/{f=1} f&&/newTag:/{print $2; exit}' "$OVERLAY/kustomization.yaml"
}

# deploy-YYYYMMDD-N。同じ日の 2 回目以降は N を進める
next_tag() {
  local today cur n
  today="deploy-$(date +%Y%m%d)"
  cur="$(current_tag)"
  if [[ "$cur" == "$today"-* ]]; then
    n="${cur##*-}"
    printf '%s-%d' "$today" $((n + 1))
  else
    printf '%s-1' "$today"
  fi
}

# control / agent だけを差し替える。postgres の image は空の PGDATA の初期化にしか
# 使われず、tag を変えると StatefulSet が DB を再起動するので、ここでは触らない。
set_tag() {
  local tag="$1" c
  for c in control agent; do
    sed -i "/name: suyasuya-$c\$/,/newTag:/ s/^\(  newTag: \).*/\1$tag/" "$OVERLAY/kustomization.yaml"
  done
  grep -q "newTag: $tag" "$OVERLAY/kustomization.yaml" || die "kustomization.yaml の tag を書き換えられなかった"
}

gke_build() {
  local tag="$1" c
  need docker
  for c in control agent; do
    info "build & push $c:$tag ($PLATFORM)"
    docker buildx build --platform "$PLATFORM" --push \
      -f "docker/$c.Dockerfile" -t "$REGISTRY/$c:$tag" .
  done
}

gke_rollout() {
  kc rollout status deployment/control --timeout=300s
  kc rollout status deployment/agent --timeout=300s
  kc get pods
}

gke_backup() {
  umask 077
  mkdir -p data/backup
  local out="data/backup/gke-$(date +%Y%m%d-%H%M%S).dump"
  info "pg_dump → $out"
  kc exec postgres-0 -- sh -ec 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "$out"
  [[ -s "$out" ]] || die "backup が空: $out"
  info "backup: $(du -h "$out" | cut -f1)"
}

gke() {
  need kubectl
  local cmd="${1:-help}"; shift || true
  case "$cmd" in
    deploy)
      # 作業ツリーの中身がそのまま image になるので、未 commit があれば止める
      if [[ -n "$(git status --porcelain -- . ':!deploy/overlays')" ]]; then
        [[ "${ALLOW_DIRTY:-}" == 1 ]] || die "未 commit の変更がある（ALLOW_DIRTY=1 で無視）"
        warn "未 commit の変更を含めて build する"
      fi
      local tag="${1:-$(next_tag)}"
      info "context=$KUBE_CONTEXT  $(current_tag) → $tag  ($(git rev-parse --short HEAD))"
      confirm "本番に deploy する？"
      gke_build "$tag"
      set_tag "$tag"
      info "server dry-run"
      kubectl --context "$KUBE_CONTEXT" apply --dry-run=server -k "$OVERLAY" >/dev/null
      kubectl --context "$KUBE_CONTEXT" apply -k "$OVERLAY"
      gke_rollout
      info "deployed $tag。$OVERLAY/kustomization.yaml の tag 変更を commit すること" ;;
    apply)    # image は変えずに overlay だけ反映する
      kubectl --context "$KUBE_CONTEXT" diff -k "$OVERLAY" || true
      confirm "この差分を適用する？"
      kubectl --context "$KUBE_CONTEXT" apply -k "$OVERLAY"
      gke_rollout ;;
    diff)     kubectl --context "$KUBE_CONTEXT" diff -k "$OVERLAY" || true ;;
    status|ps) kc get pods,pvc,svc,ingress
               info "tag: $(current_tag)" ;;
    logs)     kc logs -f --tail=100 "deployment/${1:-agent}" ;;
    restart)  kc rollout restart "deployment/${1:?control か agent}"
              kc rollout status "deployment/$1" --timeout=300s ;;
    psql)     kc exec -it postgres-0 -- sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"' ;;
    backup)   gke_backup ;;
    migrate)  # deploy/README: 既存 DB の更新はバックアップ後に管理者で明示的に流す
      local f; f="$(migration_file "${1:-}")"
      confirm "本番 DB に $f を適用する（先に backup を取る）？"
      gke_backup
      info "適用: $f"
      kc exec -i postgres-0 -- sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < "$f"
      info "OK" ;;
    secrets)  confirm "deploy/secrets/*.env を本番の Secret に反映する？"
              ./deploy/scripts/apply-secrets.sh "$KUBE_CONTEXT" ;;
    *) usage ;;
  esac
}

usage() {
  cat <<EOF
usage: $0 dev|gke <command> [args]

dev（docker compose、.env が必要）
  up [svc]          build して起動（http://localhost:\${APP_PORT:-8080}）
  down              停止（-v は記録ごと消えるので付けない）
  restart [svc]     再起動
  build [svc]       image を build
  ps                状態
  logs [svc..]      log を追う（既定 control agent）
  psql              DB に入る
  migrate FILE      docker/NN-*.sql を dev DB に適用
  test              go vet / go test と tests/schema.sql

gke（本番、context=$KUBE_CONTEXT）
  deploy [TAG]      control/agent を build・push → overlay の tag 更新 → apply → rollout
                    TAG 省略時は deploy-YYYYMMDD-N。未 commit があると止まる（ALLOW_DIRTY=1）
  apply             image は変えず overlay の差分だけ適用
  diff              overlay と cluster の差分
  status            pods / pvc / svc / ingress と今の tag
  logs [control|agent]
  restart control|agent
  psql              本番 DB に入る
  backup            pg_dump を data/backup/ に取る
  migrate FILE      backup を取ってから docker/NN-*.sql を本番 DB に適用
  secrets           deploy/secrets/*.env を Secret に反映

本番に触る操作は確認を挟む（SUYASUYA_YES=1 で省略）。
環境変数で上書き: KUBE_CONTEXT REGISTRY PLATFORM
EOF
}

case "${1:-}" in
  dev) shift; dev "$@" ;;
  gke) shift; gke "$@" ;;
  *)   usage ;;
esac

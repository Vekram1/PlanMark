#!/usr/bin/env bash
set -euo pipefail

REPO_SLUG="${PLANMARK_REPO:-Vekram1/PlanMark}"
REPO_URL="${PLANMARK_REPO_URL:-https://github.com/${REPO_SLUG}.git}"
INSTALL_DIR="${PLANMARK_INSTALL_DIR:-$HOME/.local/bin}"
AUTO_INSTALL_DEPS="${PLANMARK_AUTO_INSTALL_DEPS:-1}"
PLANMARK_CHANNEL="${PLANMARK_CHANNEL:-stable}"
PLANMARK_REF="${PLANMARK_REF:-}"
TMP_DIR=""

cleanup() {
  if [[ -n "${TMP_DIR}" && -d "${TMP_DIR}" ]]; then
    rm -rf "${TMP_DIR}"
  fi
}
trap cleanup EXIT

log() {
  printf '[planmark-install] %s\n' "$*"
}

has_cmd() {
  command -v "$1" >/dev/null 2>&1
}

detect_os() {
  case "$(uname -s)" in
    Darwin) echo "darwin" ;;
    Linux) echo "linux" ;;
    *) echo "unsupported" ;;
  esac
}

try_install_with_brew() {
  if ! has_cmd brew; then
    return 1
  fi
  log "Installing missing dependency with Homebrew: $1"
  brew install "$1"
}

try_install_with_apt() {
  if ! has_cmd apt-get; then
    return 1
  fi
  log "Installing missing dependency with apt-get: $1"
  sudo apt-get update
  sudo apt-get install -y "$1"
}

try_install_with_dnf() {
  if ! has_cmd dnf; then
    return 1
  fi
  log "Installing missing dependency with dnf: $1"
  sudo dnf install -y "$1"
}

try_install_with_yum() {
  if ! has_cmd yum; then
    return 1
  fi
  log "Installing missing dependency with yum: $1"
  sudo yum install -y "$1"
}

install_dep_or_fail() {
  local dep="$1"
  local os_name="$2"
  if has_cmd "$dep"; then
    return 0
  fi

  if [[ "${AUTO_INSTALL_DEPS}" == "1" ]]; then
    case "${os_name}" in
      darwin)
        case "$dep" in
          go) try_install_with_brew go || true ;;
          git) try_install_with_brew git || true ;;
          curl) try_install_with_brew curl || true ;;
          tar) try_install_with_brew gnu-tar || true ;;
        esac
        ;;
      linux)
        case "$dep" in
          go)
            try_install_with_apt golang-go || try_install_with_dnf golang || try_install_with_yum golang || true
            ;;
          git)
            try_install_with_apt git || try_install_with_dnf git || try_install_with_yum git || true
            ;;
          curl)
            try_install_with_apt curl || try_install_with_dnf curl || try_install_with_yum curl || true
            ;;
          tar)
            try_install_with_apt tar || try_install_with_dnf tar || try_install_with_yum tar || true
            ;;
        esac
        ;;
    esac
  fi

  if ! has_cmd "$dep"; then
    log "Missing required dependency: ${dep}"
    if [[ "${os_name}" == "darwin" ]]; then
      log "Install manually (macOS): brew install ${dep}"
    elif [[ "${os_name}" == "linux" ]]; then
      log "Install manually (Linux): sudo apt-get install -y ${dep}  # or dnf/yum equivalent"
    fi
    exit 1
  fi
}

resolve_target_ref() {
  if [[ -n "${PLANMARK_REF}" ]]; then
    echo "${PLANMARK_REF}"
    return 0
  fi

  if [[ "${PLANMARK_CHANNEL}" == "edge" ]]; then
    echo "master"
    return 0
  fi
  if [[ "${PLANMARK_CHANNEL}" != "stable" ]]; then
    log "Invalid PLANMARK_CHANNEL=${PLANMARK_CHANNEL}. Use stable or edge."
    exit 1
  fi

  local latest_api
  latest_api="https://api.github.com/repos/${REPO_SLUG}/releases/latest"
  local latest_tag
  latest_tag="$(curl -fsSL "${latest_api}" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
  if [[ -z "${latest_tag}" ]]; then
    log "Could not resolve latest stable release tag from GitHub."
    log "Set PLANMARK_REF=<tag> explicitly, or PLANMARK_CHANNEL=edge."
    exit 1
  fi
  echo "${latest_tag}"
}

main() {
  local os_name
  local target_ref
  os_name="$(detect_os)"
  if [[ "${os_name}" == "unsupported" ]]; then
    log "Unsupported OS. This installer currently supports macOS and Linux."
    exit 1
  fi

  log "Checking dependencies..."
  install_dep_or_fail curl "${os_name}"
  install_dep_or_fail tar "${os_name}"
  install_dep_or_fail git "${os_name}"
  install_dep_or_fail go "${os_name}"

  target_ref="$(resolve_target_ref)"

  TMP_DIR="$(mktemp -d)"
  log "Cloning ${REPO_URL} (ref=${target_ref})..."
  git clone --depth 1 --branch "${target_ref}" "${REPO_URL}" "${TMP_DIR}/planmark"

  log "Building plan binary..."
  (
    cd "${TMP_DIR}/planmark"
    go build -trimpath -ldflags "-s -w" -o "${TMP_DIR}/plan" ./cmd/plan
  )

  mkdir -p "${INSTALL_DIR}"
  cp "${TMP_DIR}/plan" "${INSTALL_DIR}/plan"
  chmod +x "${INSTALL_DIR}/plan"

  log "Installed: ${INSTALL_DIR}/plan"
  if "${INSTALL_DIR}/plan" version --format text >/dev/null 2>&1; then
    log "Verification: OK"
  else
    log "Verification failed: installed binary did not execute as expected."
    exit 1
  fi

  if [[ ":${PATH}:" != *":${INSTALL_DIR}:"* ]]; then
    log "Add this to your shell profile:"
    log "  export PATH=\"${INSTALL_DIR}:\$PATH\""
  fi

  log "Next steps:"
  log "  1) cd <your-project>"
  log "  2) plan init --dir . --format text"
  log "  3) plan compile --plan PLAN.md --out .planmark/tmp/plan.json"
  log "Installed release/ref: ${target_ref}"
}

main "$@"

#!/usr/bin/env bash
set -euo pipefail

REPO_SLUG="${PLANMARK_REPO:-Vekram1/PlanMark}"
INSTALL_DIR="${PLANMARK_INSTALL_DIR:-$HOME/.local/bin}"
PLANMARK_CHANNEL="${PLANMARK_CHANNEL:-stable}"
PLANMARK_REF="${PLANMARK_REF:-}"
PLANMARK_BIN_NAME="${PLANMARK_BIN_NAME:-planmark}"
PLANMARK_LEGACY_ALIAS="${PLANMARK_LEGACY_ALIAS:-1}"
GITHUB_BASE_URL="${PLANMARK_GITHUB_BASE_URL:-https://github.com/${REPO_SLUG}}"
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

install_dep_or_fail() {
  local dep="$1"
  if has_cmd "$dep"; then
    return 0
  fi
  log "Missing required dependency: ${dep}"
  exit 1
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
  install_dep_or_fail curl
  install_dep_or_fail tar

  target_ref="$(resolve_target_ref)"
  arch="$(uname -m)"
  case "${arch}" in
    x86_64|amd64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *)
      log "Unsupported architecture: ${arch}"
      exit 1
      ;;
  esac

  TMP_DIR="$(mktemp -d)"
  local archive_name
  archive_name="planmark_${target_ref#v}_${os_name}_${arch}.tar.gz"
  local download_url
  download_url="${GITHUB_BASE_URL}/releases/download/${target_ref}/${archive_name}"
  log "Downloading ${download_url}"
  curl -fsSL "${download_url}" -o "${TMP_DIR}/${archive_name}"
  tar -xzf "${TMP_DIR}/${archive_name}" -C "${TMP_DIR}"

  mkdir -p "${INSTALL_DIR}"
  cp "${TMP_DIR}/planmark" "${INSTALL_DIR}/${PLANMARK_BIN_NAME}"
  chmod +x "${INSTALL_DIR}/${PLANMARK_BIN_NAME}"
  if [[ "${PLANMARK_LEGACY_ALIAS}" == "1" ]]; then
    cp "${TMP_DIR}/planmark" "${INSTALL_DIR}/plan"
    chmod +x "${INSTALL_DIR}/plan"
  fi

  log "Installed: ${INSTALL_DIR}/${PLANMARK_BIN_NAME}"
  if "${INSTALL_DIR}/${PLANMARK_BIN_NAME}" version --format text >/dev/null 2>&1; then
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
  log "  2) ${PLANMARK_BIN_NAME} --help"
  log "  3) ${PLANMARK_BIN_NAME} init --dir . --format text"
  log "  4) ${PLANMARK_BIN_NAME} compile --plan PLAN.md --out .planmark/tmp/plan.json"
  log "Installed release/ref: ${target_ref}"
}

main "$@"

#!/usr/bin/env bash
set -euo pipefail

REPO="${REPO:-dhwang2/go-proxy}"
VERSION="${VERSION:-latest}"
INSTALL_PATH="${INSTALL_PATH:-/usr/bin/gproxy}"
TMP_DIR=""
RELEASE_PREFIX="${RELEASE_PREFIX:-v}"

# Runtime paths (must match app/internal/config/paths.go).
WORK_DIR="/etc/go-proxy"
BIN_DIR="${WORK_DIR}/bin"
CONF_DIR="${WORK_DIR}/conf"
LOG_DIR="${WORK_DIR}/logs"

usage() {
  cat <<'EOF'
go-proxy installer

Environment variables:
  REPO         GitHub repository (default: dhwang2/go-proxy)
  VERSION      Release tag or "latest" (default: latest)
  INSTALL_PATH Install target path (default: /usr/bin/gproxy)

Example:
  curl -fsSL https://raw.githubusercontent.com/dhwang2/go-proxy/main/app/install.sh | sudo bash
  REPO=owner/go-proxy VERSION=v1.0.0 bash install.sh
EOF
}

cleanup() {
  if [[ -n "${TMP_DIR}" && -d "${TMP_DIR}" ]]; then
    rm -rf "${TMP_DIR}"
  fi
}
trap cleanup EXIT

require_root() {
  if [[ "${EUID}" -ne 0 ]]; then
    echo "error: installer must run as root" >&2
    exit 1
  fi
}

detect_arch() {
  local machine
  machine="$(uname -m)"
  case "${machine}" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *)
      echo "error: unsupported architecture ${machine}" >&2
      exit 1
      ;;
  esac
}

fetch_url() {
  local url="$1"
  local output="$2"
  curl -fsSL -H "Accept: application/octet-stream" -o "${output}" "${url}"
}

resolve_release_tag() {
  if [[ "${VERSION}" != "latest" ]]; then
    echo "${VERSION}"
    return 0
  fi

  local api_url="https://api.github.com/repos/${REPO}/releases?per_page=100"
  local payload
  payload="$(curl -fsSL "${api_url}")" || return 1

  printf '%s' "${payload}" \
    | grep -o '"tag_name":[[:space:]]*"[^"]*"' \
    | cut -d'"' -f4 \
    | grep "^${RELEASE_PREFIX}" \
    | head -n1
}

fetch_api_payload() {
  local url="$1"
  curl -fsSL "${url}"
}

extract_asset_api_url() {
  local payload="$1"
  local target_name="$2"
  printf '%s\n' "${payload}" | awk -v target="${target_name}" '
    /"url":[[:space:]]*"https:\/\/api.github.com\/repos\/.*\/releases\/assets\// {
      current = $0
      sub(/.*"url":[[:space:]]*"/, "", current)
      sub(/".*/, "", current)
      next
    }
    /"name":[[:space:]]*"/ {
      name = $0
      sub(/.*"name":[[:space:]]*"/, "", name)
      sub(/".*/, "", name)
      if (name == target && current != "") {
        print current
        exit
      }
    }
  '
}

resolve_download_urls() {
  local arch="$1"
  local release_tag
  release_tag="$(resolve_release_tag)"
  if [[ -z "${release_tag}" ]]; then
    return 0
  fi
  local release_api="https://api.github.com/repos/${REPO}/releases/tags/${release_tag}"
  local payload
  payload="$(fetch_api_payload "${release_api}")"

  local candidates=(
    "gproxy-linux-${arch}"
    "gproxy-${arch}"
    "gproxy-linux-${arch}.tar.gz"
    "gproxy-${arch}.tar.gz"
  )

  local name
  for name in "${candidates[@]}"; do
    local asset_url
    asset_url="$(extract_asset_api_url "${payload}" "${name}")"
    if [[ -n "${asset_url}" ]]; then
      echo "${asset_url}"
    fi
  done
}

download_first_available() {
  local output="$1"
  shift
  local url
  for url in "$@"; do
    if fetch_url "${url}" "${output}"; then
      echo "${url}"
      return 0
    fi
  done
  return 1
}

extract_if_needed() {
  local source_path="$1"
  local source_ref="$2"
  local arch="$3"
  local out="$4"

  case "${source_ref}" in
    *.tar.gz)
      tar -xzf "${source_path}" -C "${TMP_DIR}"
      if [[ -f "${TMP_DIR}/gproxy" ]]; then
        cp "${TMP_DIR}/gproxy" "${out}"
      elif [[ -f "${TMP_DIR}/gproxy-linux-${arch}" ]]; then
        cp "${TMP_DIR}/gproxy-linux-${arch}" "${out}"
      elif [[ -f "${TMP_DIR}/gproxy-${arch}" ]]; then
        cp "${TMP_DIR}/gproxy-${arch}" "${out}"
      else
        echo "error: cannot find gproxy binary inside archive" >&2
        exit 1
      fi
      ;;
    *)
      cp "${source_path}" "${out}"
      ;;
  esac
}

cleanup_stale_binaries() {
  local target="$1"
  local known_paths=("/usr/bin/gproxy" "/usr/local/bin/gproxy")
  local p
  for p in "${known_paths[@]}"; do
    if [[ "${p}" != "${target}" && -f "${p}" ]]; then
      rm -f "${p}"
      echo "removed stale: ${p}"
    fi
  done
  # Clear bash hash so the current shell finds the new path.
  hash -r 2>/dev/null || true
}

install_binary() {
  local source="$1"
  local target="$2"
  local backup=""
  if [[ -f "${target}" ]]; then
    backup="${target}.$(date -u +%Y%m%dT%H%M%SZ).bak"
    cp "${target}" "${backup}"
    echo "backup: ${backup}"
  fi
  install -m 0755 "${source}" "${target}"
  cleanup_stale_binaries "${target}"
}

download_file() {
  local url="$1"
  local output="$2"
  curl -fsSL --retry 3 --retry-delay 1 --connect-timeout 10 -o "${output}" "${url}"
}

resolve_github_latest_tag() {
  local repo="$1"
  local api_url="https://api.github.com/repos/${repo}/releases/latest"
  local payload
  payload="$(curl -fsSL "${api_url}" 2>/dev/null)" || return 1
  printf '%s' "${payload}" | grep -o '"tag_name":[[:space:]]*"[^"]*"' | head -1 | cut -d'"' -f4
}

install_singbox_core() {
  local arch="$1"
  local version
  version="$(resolve_github_latest_tag "SagerNet/sing-box")"
  version="${version#v}"
  version="${version:-1.10.0}"
  echo "installing sing-box v${version}..."

  mkdir -p "${BIN_DIR}" "${CONF_DIR}" "${LOG_DIR}"
  local filename="sing-box-${version}-linux-${arch}.tar.gz"
  local url="https://github.com/SagerNet/sing-box/releases/download/v${version}/${filename}"

  download_file "${url}" "${TMP_DIR}/${filename}"
  tar -zxf "${TMP_DIR}/${filename}" -C "${TMP_DIR}"
  local extracted_bin
  extracted_bin="$(find "${TMP_DIR}" -name sing-box -type f | head -n 1)"
  if [[ -z "${extracted_bin}" || ! -f "${extracted_bin}" ]]; then
    echo "warn: sing-box binary not found in archive, skipping" >&2
    return 1
  fi
  install -m 755 "${extracted_bin}" "${BIN_DIR}/sing-box"
  echo "installed: ${BIN_DIR}/sing-box"
}

install_snell() {
  local arch="$1"
  local version="5.0.1"
  echo "installing snell-v5 v${version}..."

  mkdir -p "${BIN_DIR}"
  local snell_arch="${arch}"
  [[ "${arch}" == "arm64" ]] && snell_arch="aarch64"

  local filename="snell-server-v${version}-linux-${snell_arch}.zip"
  local url="https://dl.nssurge.com/snell/${filename}"

  download_file "${url}" "${TMP_DIR}/${filename}"
  unzip -o "${TMP_DIR}/${filename}" -d "${TMP_DIR}" >/dev/null 2>&1
  if [[ ! -f "${TMP_DIR}/snell-server" ]]; then
    echo "warn: snell-server binary not found, skipping" >&2
    return 1
  fi
  install -m 755 "${TMP_DIR}/snell-server" "${BIN_DIR}/snell-server"
  echo "installed: ${BIN_DIR}/snell-server"
}

install_shadowtls() {
  local arch="$1"
  echo "installing shadow-tls..."

  local st_version
  st_version="$(resolve_github_latest_tag "ihciah/shadow-tls")"
  st_version="${st_version#v}"
  st_version="${st_version:-0.2.25}"

  local st_arch="x86_64-unknown-linux-musl"
  [[ "${arch}" == "arm64" ]] && st_arch="aarch64-unknown-linux-musl"

  local filename="shadow-tls-${st_arch}"
  local url="https://github.com/ihciah/shadow-tls/releases/download/v${st_version}/${filename}"

  mkdir -p "${BIN_DIR}"
  download_file "${url}" "${TMP_DIR}/${filename}"
  if [[ ! -f "${TMP_DIR}/${filename}" ]]; then
    echo "warn: shadow-tls download failed, skipping" >&2
    return 1
  fi
  install -m 755 "${TMP_DIR}/${filename}" "${BIN_DIR}/shadow-tls"
  echo "installed: ${BIN_DIR}/shadow-tls"
}

install_caddy() {
  local arch="$1"
  echo "installing caddy..."

  local version
  version="$(resolve_github_latest_tag "caddyserver/caddy")"
  version="${version#v}"
  version="${version:-2.9.1}"

  local filename="caddy_${version}_linux_${arch}.tar.gz"
  local url="https://github.com/caddyserver/caddy/releases/download/v${version}/${filename}"

  mkdir -p "${BIN_DIR}"
  download_file "${url}" "${TMP_DIR}/${filename}"
  tar -zxf "${TMP_DIR}/${filename}" -C "${TMP_DIR}" caddy 2>/dev/null || tar -zxf "${TMP_DIR}/${filename}" -C "${TMP_DIR}"
  if [[ ! -f "${TMP_DIR}/caddy" ]]; then
    echo "warn: caddy binary not found in archive, skipping" >&2
    return 1
  fi
  install -m 755 "${TMP_DIR}/caddy" "${BIN_DIR}/caddy"
  echo "installed: ${BIN_DIR}/caddy"
}

install_cores() {
  local arch="$1"
  # Each core install is best-effort; failures are non-fatal.
  install_singbox_core "${arch}" || echo "warn: sing-box installation failed" >&2
  rm -rf "${TMP_DIR:?}"/* 2>/dev/null || true
  install_snell "${arch}" || echo "warn: snell installation failed" >&2
  rm -rf "${TMP_DIR:?}"/* 2>/dev/null || true
  install_shadowtls "${arch}" || echo "warn: shadow-tls installation failed" >&2
  rm -rf "${TMP_DIR:?}"/* 2>/dev/null || true
  install_caddy "${arch}" || echo "warn: caddy installation failed" >&2
}

main() {
  if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
    usage
    exit 0
  fi

  require_root
  local arch
  arch="$(detect_arch)"

  TMP_DIR="$(mktemp -d)"

  # --- Install gproxy binary ---
  local -a candidates=()
  mapfile -t candidates < <(resolve_download_urls "${arch}")
  local downloaded="${TMP_DIR}/downloaded.bin"
  local extracted="${TMP_DIR}/gproxy"
  local url=""
  if [[ "${#candidates[@]}" -eq 0 ]]; then
    echo "error: no go-proxy release tag found for ${REPO} (${VERSION})" >&2
    echo "hint: publish a ${RELEASE_PREFIX}* release with matching binary assets" >&2
    exit 1
  fi
  if ! url="$(download_first_available "${downloaded}" "${candidates[@]}")"; then
    echo "error: no release asset found for ${REPO} (${VERSION}) arch=${arch}" >&2
    echo "hint: publish a ${RELEASE_PREFIX}* release with matching binary assets" >&2
    echo "hint: upload assets named gproxy-linux-${arch} or gproxy-${arch}" >&2
    exit 1
  fi

  echo "download: ${url}"
  extract_if_needed "${downloaded}" "${url}" "${arch}" "${extracted}"
  install_binary "${extracted}" "${INSTALL_PATH}"

  echo "installed: ${INSTALL_PATH}"
  "${INSTALL_PATH}" version || true

  # --- Install all service cores ---
  rm -rf "${TMP_DIR:?}"/* 2>/dev/null || true
  install_cores "${arch}"

  # --- Initialize config, services, and watchdog ---
  echo "initializing services..."
  "${INSTALL_PATH}" init || echo "warn: gproxy init failed" >&2

  echo "installation complete"
}

main "$@"

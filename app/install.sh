#!/usr/bin/env bash
set -euo pipefail

REPO="${REPO:-dhwang2/go-proxy}"
VERSION="${VERSION:-latest}"
INSTALL_PATH="${INSTALL_PATH:-/usr/bin/proxy}"
TMP_DIR=""
RELEASE_PREFIX="${RELEASE_PREFIX:-v}"

usage() {
  cat <<'EOF'
go-proxy installer

Environment variables:
  REPO         GitHub repository (default: dhwang2/go-proxy)
  VERSION      Release tag or "latest" (default: latest)
  INSTALL_PATH Install target path (default: /usr/bin/proxy)

Example:
  curl -fsSL https://raw.githubusercontent.com/dhwang2/go-proxy/main/app/install.sh | sudo bash
  REPO=owner/proxy VERSION=v1.0.0 bash install.sh
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
    "proxy-linux-${arch}"
    "proxy-${arch}"
    "proxy-linux-${arch}.tar.gz"
    "proxy-${arch}.tar.gz"
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
      if [[ -f "${TMP_DIR}/proxy" ]]; then
        cp "${TMP_DIR}/proxy" "${out}"
      elif [[ -f "${TMP_DIR}/proxy-linux-${arch}" ]]; then
        cp "${TMP_DIR}/proxy-linux-${arch}" "${out}"
      elif [[ -f "${TMP_DIR}/proxy-${arch}" ]]; then
        cp "${TMP_DIR}/proxy-${arch}" "${out}"
      else
        echo "error: cannot find proxy binary inside archive" >&2
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
  local known_paths=("/usr/bin/proxy" "/usr/local/bin/proxy")
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

main() {
  if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
    usage
    exit 0
  fi

  require_root
  local arch
  arch="$(detect_arch)"

  TMP_DIR="$(mktemp -d)"
  local -a candidates=()
  mapfile -t candidates < <(resolve_download_urls "${arch}")
  local downloaded="${TMP_DIR}/downloaded.bin"
  local extracted="${TMP_DIR}/proxy"
  local url=""
  if [[ "${#candidates[@]}" -eq 0 ]]; then
    echo "error: no go-proxy release tag found for ${REPO} (${VERSION})" >&2
    echo "hint: publish a ${RELEASE_PREFIX}* release with matching binary assets" >&2
    exit 1
  fi
  if ! url="$(download_first_available "${downloaded}" "${candidates[@]}")"; then
    echo "error: no release asset found for ${REPO} (${VERSION}) arch=${arch}" >&2
    echo "hint: publish a ${RELEASE_PREFIX}* release with matching binary assets" >&2
    echo "hint: upload assets named proxy-linux-${arch} or proxy-${arch}" >&2
    exit 1
  fi

  echo "download: ${url}"
  extract_if_needed "${downloaded}" "${url}" "${arch}" "${extracted}"
  install_binary "${extracted}" "${INSTALL_PATH}"

  echo "installed: ${INSTALL_PATH}"
  "${INSTALL_PATH}" version || true
}

main "$@"

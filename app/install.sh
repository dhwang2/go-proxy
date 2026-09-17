#!/usr/bin/env bash
set -euo pipefail

REPO="${REPO:-dhwang2/go-proxy}"
VERSION="${VERSION:-latest}"
INSTALL_PATH="${INSTALL_PATH:-/usr/bin/gproxy}"

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  printf '%s\n' "go-proxy installer" "Environment: REPO, VERSION, INSTALL_PATH" "Installs a verified release, then runs gproxy init."
  exit 0
fi
[[ $# == 0 ]] || { echo "error: unexpected installer arguments" >&2; exit 2; }
[[ "${EUID}" == 0 ]] || { echo "error: installer requires root" >&2; exit 1; }
[[ "$(uname -s)" == Linux ]] || { echo "error: Linux is required" >&2; exit 1; }
[[ "${REPO}" =~ ^[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$ ]] || { echo "error: invalid repository" >&2; exit 2; }
[[ "${INSTALL_PATH}" == /* && "${INSTALL_PATH}" != */ ]] || { echo "error: INSTALL_PATH must be an absolute file path" >&2; exit 2; }
for tool in curl sha256sum install mktemp; do
  command -v "${tool}" >/dev/null || { echo "error: required tool missing: ${tool}" >&2; exit 1; }
done

case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "error: unsupported architecture" >&2; exit 1 ;;
esac

download() {
  curl -fsSL --connect-timeout 10 --max-time 180 --retry 2 --retry-delay 1 "$1" -o "$2"
}

task_dir="$(mktemp -d)"
staging=""
cleanup() {
  if [[ -n "${staging}" && -f "${staging}" ]]; then rm -f -- "${staging}"; fi
  if [[ -d "${task_dir}" ]]; then rm -rf -- "${task_dir}"; fi
}
trap cleanup EXIT

tag="${VERSION}"
if [[ "${tag}" == latest ]]; then
  download "https://api.github.com/repos/${REPO}/releases/latest" "${task_dir}/release.json"
  tag="$(sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' "${task_dir}/release.json" | head -n 1)"
fi
[[ "${tag}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-.][a-zA-Z0-9.-]+)?$ ]] || { echo "error: invalid release version" >&2; exit 1; }

asset="gproxy-linux-${arch}"
base="https://github.com/${REPO}/releases/download/${tag}"
download "${base}/${asset}" "${task_dir}/${asset}"
download "${base}/${asset}.sha256" "${task_dir}/checksum"
expected="$(awk -v name="${asset}" '$2 == name {print $1}' "${task_dir}/checksum")"
[[ "${expected}" =~ ^[a-fA-F0-9]{64}$ ]] || { echo "error: invalid release checksum" >&2; exit 1; }
actual="$(sha256sum "${task_dir}/${asset}" | cut -d ' ' -f 1)"
[[ "${expected,,}" == "${actual}" ]] || { echo "error: release checksum mismatch" >&2; exit 1; }

chmod 755 "${task_dir}/${asset}"
[[ "$("${task_dir}/${asset}" version)" == "go-proxy ${tag}" ]] || { echo "error: release version mismatch" >&2; exit 1; }
install_dir="$(dirname "${INSTALL_PATH}")"
mkdir -p "${install_dir}"
staging="$(mktemp "${install_dir}/.gproxy-install.XXXXXX")"
install -m 755 "${task_dir}/${asset}" "${staging}"
mv -f -- "${staging}" "${INSTALL_PATH}"
staging=""
"${INSTALL_PATH}" init
printf 'installed go-proxy %s at %s\n' "${tag}" "${INSTALL_PATH}"

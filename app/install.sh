#!/usr/bin/env bash
# go-proxy installer.
#
# Every step runs from main(), called on the last line. Piped through
# `curl | bash`, a download cut short never reaches that line, so a partial
# script defines functions and runs nothing.
set -Eeuo pipefail

REPO="${REPO:-dhwang2/go-proxy}"
VERSION="${VERSION:-latest}"
INSTALL_PATH="${INSTALL_PATH:-/usr/bin/gproxy}"

# These two lines must match config.BashrcCompletionBeginMark and
# config.BashrcCompletionEndMark: uninstall removes the block between them.
BASHRC_BEGIN="# >>> go-proxy: load bash completion in every interactive shell >>>"
BASHRC_END="# <<< go-proxy <<<"

# Messages follow shell-proxy's installer: a bold green line per step, yellow
# for what is worth knowing, red for what failed. Colour only on a terminal.
if [[ -t 1 && -z "${NO_COLOR:-}" ]]; then
  green() { printf '\033[32m\033[01m%s\033[0m\n' "$*"; }
  yellow() { printf '\033[33m\033[01m%s\033[0m\n' "$*"; }
  red() { printf '\033[31m\033[01m%s\033[0m\n' "$*" >&2; }
else
  green() { printf '%s\n' "$*"; }
  yellow() { printf '%s\n' "$*"; }
  red() { printf '%s\n' "$*" >&2; }
fi

fail() {
  red "error: $*"
  exit 1
}

on_error() {
  red "installation failed at line $1: $2"
}

usage() {
  printf '%s\n' "go-proxy installer" \
    "Environment: REPO, VERSION (default latest), INSTALL_PATH (default /usr/bin/gproxy)" \
    "Installs a verified release, then runs gproxy init."
}

# download fetches one file, showing curl's progress bar on a terminal. A
# network or server failure is retried, each attempt saying which one it was
# and the HTTP status, as shell-proxy's bootstrap does; the last failure names
# the URL and status and stops.
download() {
  local url="$1" output="$2" attempt code=""
  local progress=(--silent --show-error)
  [[ -t 2 ]] && progress=(--progress-bar)
  for attempt in 1 2 3; do
    if code="$(curl --fail --location --connect-timeout 10 --max-time 300 "${progress[@]}" \
      --output "${output}" --write-out '%{http_code}' "${url}")"; then
      return 0
    fi
    # A 4xx answer will not change on a retry: the file is not there.
    [[ "${code}" =~ ^4 ]] && break
    if ((attempt < 3)); then
      yellow "download retry (${attempt}/3): ${url} [HTTP ${code:-000}]"
      sleep 1
    fi
  done
  red "download failed: ${url}"
  red "HTTP status: ${code:-000}"
  return 1
}

# latest_tag follows github.com's own redirect from /releases/latest to
# /releases/tag/<tag>. Unlike the REST API it has no 60-requests-an-hour limit
# for anonymous callers, and a failure is GitHub being unreachable rather than
# an empty answer mistaken for a version.
latest_tag() {
  local answer code url
  answer="$(curl --silent --location --head --connect-timeout 10 --max-time 30 \
    --output /dev/null --write-out '%{http_code} %{url_effective}' "https://github.com/${REPO}/releases/latest")" || true
  code="${answer%% *}" url="${answer#* }"
  case "${code}" in
    200) printf '%s\n' "${url##*/}" ;;
    404) red "no published release found for ${REPO}"; return 1 ;;
    *)
      red "could not reach GitHub to find the latest release of ${REPO} [HTTP ${code:-000}]"
      yellow "set VERSION=vX.Y.Z to install a specific release"
      return 1
      ;;
  esac
}

# install_completion writes a completion script where the shell's own directory
# already exists, so the installer never creates a completion framework the host
# does not use. Completion is a convenience: a failure here is reported and the
# install carries on.
install_completion() {
  local shell="$1" target="$2"
  [[ -d "$(dirname "${target}")" ]] || return 0
  if "${INSTALL_PATH}" completion "${shell}" >"${target}.tmp" 2>/dev/null; then
    chmod 644 "${target}.tmp"
    mv -f -- "${target}.tmp" "${target}"
    green "installed ${shell} completion at ${target}"
  else
    rm -f -- "${target}.tmp"
    yellow "warning: could not generate ${shell} completion"
  fi
}

# enable_bash_completion makes completion work in every interactive root shell.
# Debian and Ubuntu load bash-completion from /etc/profile.d, which only a login
# shell reads (sudo -i, su -); their /etc/bash.bashrc carries the loader for
# every other shell (sudo su, sudo -s) commented out. One marked block loads it
# there, once, and uninstall removes exactly that block. Distributions without
# /etc/bash.bashrc load completion in every shell already.
enable_bash_completion() {
  local bashrc=/etc/bash.bashrc loader=/usr/share/bash-completion/bash_completion
  [[ -f "${bashrc}" ]] || return 0
  if [[ ! -r "${loader}" ]]; then
    yellow "bash-completion is not installed; install it (apt install bash-completion) for Tab completion"
    return 0
  fi
  if grep -qxF "${BASHRC_BEGIN}" "${bashrc}" ||
    grep -Eq "^[[:space:]]*(\.|source)[[:space:]]+${loader}" "${bashrc}"; then
    return 0
  fi
  # Start on a line of its own when the file does not end with a newline.
  [[ -z "$(tail -c 1 "${bashrc}")" ]] || printf '\n' >>"${bashrc}"
  # The block is written as literal text; it expands in the shell that reads it.
  # shellcheck disable=SC2016
  {
    printf '%s\n' "${BASHRC_BEGIN}"
    printf '%s\n' 'if [ -n "${PS1-}" ] && [ -z "${BASH_COMPLETION_VERSINFO-}" ] && shopt -q progcomp && [ -r /usr/share/bash-completion/bash_completion ]; then'
    printf '%s\n' '  . /usr/share/bash-completion/bash_completion'
    printf '%s\n' 'fi'
    printf '%s\n' "${BASHRC_END}"
  } >>"${bashrc}"
  green "enabled bash completion in every interactive shell (${bashrc}); open a new shell to use it"
}

main() {
  if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
    usage
    return 0
  fi
  [[ $# == 0 ]] || { red "error: unexpected installer arguments"; exit 2; }
  [[ "${EUID}" == 0 ]] || fail "the installer requires root"
  [[ "$(uname -s)" == Linux ]] || fail "Linux is required"
  [[ "${REPO}" =~ ^[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$ ]] || { red "error: invalid repository"; exit 2; }
  [[ "${INSTALL_PATH}" == /* && "${INSTALL_PATH}" != */ ]] || { red "error: INSTALL_PATH must be an absolute file path"; exit 2; }
  local tool
  for tool in curl sha256sum install mktemp; do
    command -v "${tool}" >/dev/null || fail "required tool missing: ${tool}"
  done

  local arch
  case "$(uname -m)" in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) fail "unsupported architecture: $(uname -m)" ;;
  esac

  trap 'on_error "${LINENO}" "${BASH_COMMAND}"' ERR
  task_dir="$(mktemp -d)"
  staging=""
  trap 'rm -f -- "${staging:-}"; rm -rf -- "${task_dir}"' EXIT

  printf '%s\n' "================================================="
  printf '%s\n' "   go-proxy installer"
  printf '%s\n' "================================================="

  local tag="${VERSION}"
  if [[ "${tag}" == latest ]]; then
    green "Resolving the latest release of ${REPO}..."
    tag="$(latest_tag)"
  fi
  [[ "${tag}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-.][a-zA-Z0-9.-]+)?$ ]] ||
    fail "no usable release version (${tag:-none}); set VERSION=vX.Y.Z"

  local asset="gproxy-linux-${arch}"
  local base="https://github.com/${REPO}/releases/download/${tag}"
  green "Installing go-proxy ${tag} (linux-${arch})..."
  green "Downloading ${asset}..."
  download "${base}/${asset}" "${task_dir}/${asset}"
  download "${base}/${asset}.sha256" "${task_dir}/checksum"

  green "Verifying the checksum..."
  local expected actual
  expected="$(awk -v name="${asset}" '$2 == name {print $1}' "${task_dir}/checksum")"
  [[ "${expected}" =~ ^[a-fA-F0-9]{64}$ ]] || fail "invalid release checksum"
  actual="$(sha256sum "${task_dir}/${asset}" | cut -d ' ' -f 1)"
  [[ "${expected,,}" == "${actual}" ]] || fail "release checksum mismatch"
  chmod 755 "${task_dir}/${asset}"
  [[ "$("${task_dir}/${asset}" version)" == "go-proxy ${tag}" ]] || fail "release version mismatch"

  green "Installing ${INSTALL_PATH}..."
  local install_dir
  install_dir="$(dirname "${INSTALL_PATH}")"
  mkdir -p "${install_dir}"
  staging="$(mktemp "${install_dir}/.gproxy-install.XXXXXX")"
  install -m 755 "${task_dir}/${asset}" "${staging}"
  mv -f -- "${staging}" "${INSTALL_PATH}"
  staging=""

  # init also moves a running watchdog onto the new binary when this
  # install replaced it.
  green "Initializing the runtime (a first install downloads sing-box)..."
  "${INSTALL_PATH}" init

  install_completion bash /usr/share/bash-completion/completions/gproxy
  install_completion zsh /usr/share/zsh/site-functions/_gproxy
  enable_bash_completion

  green "Installation complete: go-proxy ${tag} at ${INSTALL_PATH}"
  yellow "Run gproxy status for the dashboard, or gproxy protocol add to create a node."
}

main "$@"

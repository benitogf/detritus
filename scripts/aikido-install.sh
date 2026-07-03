#!/usr/bin/env bash
#
# aikido-install.sh — offline provisioning installer for the Aikido security
# scanner suite and its mirrored vulnerability databases.
#
# Designed for air-gapped / offline hosts: everything is provisioned from a
# staged bundle directory rather than the network. Run with --check first to
# preflight the host and report what is staged before committing to an install.
#
# Usage:
#   aikido-install.sh [--check] [--help]
#
#   --check   Preflight the host and report staged artifacts; make no changes.
#   --help    Show this help.
#
# Environment overrides:
#   AIKIDO_BUNDLE_DIR   Directory holding the offline bundle (scanners + DBs).
#                       Default: <repo>/vendor/aikido-bundle
#   AIKIDO_INSTALL_DIR  Where scanner binaries are installed.
#                       Default: $HOME/.aikido/bin
#   AIKIDO_DB_DIR       Where mirrored vulnerability DBs are provisioned.
#                       Default: $HOME/.aikido/db
#
set -euo pipefail

# --- configuration ---------------------------------------------------------

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

BUNDLE_DIR="${AIKIDO_BUNDLE_DIR:-${REPO_ROOT}/vendor/aikido-bundle}"
INSTALL_DIR="${AIKIDO_INSTALL_DIR:-${HOME}/.aikido/bin}"
DB_DIR="${AIKIDO_DB_DIR:-${HOME}/.aikido/db}"

# The Opengrep ruleset is a full third-party mirror (opengrep/opengrep-rules at
# plugins/aikido-rules/PINNED_COMMIT). It is NOT committed — it carries secret-
# detection test fixtures that trip GitHub push protection, and it is ~4000
# files of upstream content. It is staged in the bundle under rules/ and copied
# into this gitignored path, which aikido-guard points `opengrep --config` at.
RULES_ARTIFACT="rules"
RULES_DEST="${AIKIDO_RULES_DIR:-${REPO_ROOT}/plugins/aikido-rules/rules}"

# Security scanners provisioned offline. Each entry: "<name>:<bundle-artifact>".
# These are the open-source engines the aikido-guard predictor maps onto:
# opengrep (SAST), checkov (IaC/misconfig), gitleaks (secrets),
# osv-scanner (SCA/deps), govulncheck (Go stdlib + module vulns).
SCANNERS=(
  "opengrep:scanners/opengrep"
  "checkov:scanners/checkov"
  "gitleaks:scanners/gitleaks"
  "osv-scanner:scanners/osv-scanner"
  "govulncheck:scanners/govulncheck"
)

# Mirrored vulnerability databases. Each entry: "<name>:<bundle-artifact>".
# osv-scanner reads the OSV mirror; govulncheck reads the Go vuln DB mirror.
DB_MIRRORS=(
  "osv:db/osv.db"
  "govulndb:db/govulndb"
  "ghsa:db/ghsa.db"
)

# Base tooling the offline installer relies on to unpack/verify the bundle.
REQUIRED_CMDS=(bash cp mkdir install sha256sum tar)

# --- output helpers --------------------------------------------------------

info() { printf '  \033[0;34m•\033[0m %s\n' "$*"; }
ok()   { printf '  \033[0;32m✓\033[0m %s\n' "$*"; }
warn() { printf '  \033[0;33m!\033[0m %s\n' "$*"; }
err()  { printf '  \033[0;31m✗\033[0m %s\n' "$*" >&2; }

usage() {
  sed -n '3,26p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
}

# --- checks ----------------------------------------------------------------

# have_cmd NAME — true if NAME is on PATH.
have_cmd() { command -v "$1" >/dev/null 2>&1; }

# check_host — verify the base tooling needed to run the installer exists.
# Returns non-zero if any required command is missing (host cannot provision).
check_host() {
  local missing=0 cmd
  for cmd in "${REQUIRED_CMDS[@]}"; do
    if have_cmd "$cmd"; then
      ok "tool: ${cmd}"
    else
      err "tool: ${cmd} (missing — required to unpack the bundle)"
      missing=1
    fi
  done
  return "$missing"
}

# report_staged LIST_NAME — report which artifacts in a name:artifact list are
# staged in the bundle. Prints per-item status; never fails (missing artifacts
# are reported as warnings, since the bundle may not be staged on this host).
report_staged() {
  local -n items="$1"
  local label="$2"
  local entry name artifact path staged=0 total=0
  for entry in "${items[@]}"; do
    name="${entry%%:*}"
    artifact="${entry#*:}"
    path="${BUNDLE_DIR}/${artifact}"
    total=$((total + 1))
    if [ -e "$path" ]; then
      ok "${label}: ${name} (staged)"
      staged=$((staged + 1))
    else
      warn "${label}: ${name} (not staged at ${artifact})"
    fi
  done
  info "${label}: ${staged}/${total} staged"
}

# run_check — full preflight. Exits 0 if the host can run the installer,
# non-zero only if the host itself is incapable (missing base tooling).
run_check() {
  echo "Aikido offline provisioning — preflight"
  echo
  echo "Paths:"
  info "bundle : ${BUNDLE_DIR}"
  info "install: ${INSTALL_DIR}"
  info "db     : ${DB_DIR}"
  echo
  echo "Host tooling:"
  local host_ok=0
  check_host || host_ok=1
  echo
  echo "Bundle contents:"
  if [ -d "$BUNDLE_DIR" ]; then
    ok "bundle directory present"
  else
    warn "bundle directory absent (${BUNDLE_DIR}) — stage the bundle before install"
  fi
  report_staged SCANNERS "scanner"
  report_staged DB_MIRRORS "db"
  if [ -d "${BUNDLE_DIR}/${RULES_ARTIFACT}" ]; then
    ok "ruleset: opengrep-rules (staged)"
  else
    warn "ruleset: opengrep-rules (not staged at ${RULES_ARTIFACT}/)"
  fi
  echo

  if [ "$host_ok" -ne 0 ]; then
    err "host is missing base tooling; cannot provision offline"
    return 1
  fi
  ok "host is ready to provision from a staged bundle"
  return 0
}

# --- install ---------------------------------------------------------------

# verify_bundle — confirm the bundle dir and every required artifact exist.
verify_bundle() {
  if [ ! -d "$BUNDLE_DIR" ]; then
    err "bundle directory not found: ${BUNDLE_DIR}"
    err "stage the offline bundle (or set AIKIDO_BUNDLE_DIR) before installing"
    return 1
  fi
  local entry artifact path missing=0
  for entry in "${SCANNERS[@]}" "${DB_MIRRORS[@]}"; do
    artifact="${entry#*:}"
    path="${BUNDLE_DIR}/${artifact}"
    if [ ! -e "$path" ]; then
      err "missing bundle artifact: ${artifact}"
      missing=1
    fi
  done
  if [ ! -d "${BUNDLE_DIR}/${RULES_ARTIFACT}" ]; then
    err "missing bundle artifact: ${RULES_ARTIFACT}/ (opengrep ruleset)"
    missing=1
  fi
  return "$missing"
}

# verify_checksums — if the bundle ships a SHA256SUMS manifest, verify it.
verify_checksums() {
  local manifest="${BUNDLE_DIR}/SHA256SUMS"
  if [ ! -f "$manifest" ]; then
    warn "no SHA256SUMS manifest in bundle — skipping integrity check"
    return 0
  fi
  info "verifying bundle integrity"
  ( cd "$BUNDLE_DIR" && sha256sum -c SHA256SUMS >/dev/null ) \
    || { err "checksum verification failed"; return 1; }
  ok "bundle integrity verified"
}

# install_scanners — copy scanner binaries into the install dir.
install_scanners() {
  mkdir -p "$INSTALL_DIR"
  local entry name artifact
  for entry in "${SCANNERS[@]}"; do
    name="${entry%%:*}"
    artifact="${entry#*:}"
    install -m 0755 "${BUNDLE_DIR}/${artifact}" "${INSTALL_DIR}/${name}"
    ok "installed scanner: ${name}"
  done
}

# install_dbs — copy mirrored vulnerability databases into the db dir.
install_dbs() {
  mkdir -p "$DB_DIR"
  local entry name artifact
  for entry in "${DB_MIRRORS[@]}"; do
    name="${entry%%:*}"
    artifact="${entry#*:}"
    cp -f "${BUNDLE_DIR}/${artifact}" "${DB_DIR}/$(basename "$artifact")"
    ok "provisioned db mirror: ${name}"
  done
}

# install_rules — populate the gitignored ruleset path from the staged bundle.
# aikido-guard points `opengrep --config` here; the mirror is never committed
# (see plugins/aikido-rules/README.md and RULES_DEST above).
install_rules() {
  mkdir -p "$RULES_DEST"
  cp -a "${BUNDLE_DIR}/${RULES_ARTIFACT}/." "${RULES_DEST}/"
  ok "provisioned ruleset: opengrep-rules -> ${RULES_DEST}"
}

run_install() {
  echo "Aikido offline provisioning — install"
  echo
  check_host || { err "host missing base tooling; aborting"; return 1; }
  verify_bundle || return 1
  verify_checksums || return 1
  echo
  install_scanners
  install_dbs
  install_rules
  echo
  ok "install complete"
  info "scanners: ${INSTALL_DIR}"
  info "databases: ${DB_DIR}"
  info "ruleset: ${RULES_DEST}"
  info "add '${INSTALL_DIR}' to PATH to use the scanners"
}

# --- entrypoint ------------------------------------------------------------

main() {
  local mode="install"
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --check) mode="check" ;;
      --help|-h) usage; return 0 ;;
      *) err "unknown argument: $1"; usage; return 2 ;;
    esac
    shift
  done

  case "$mode" in
    check)   run_check ;;
    install) run_install ;;
  esac
}

main "$@"

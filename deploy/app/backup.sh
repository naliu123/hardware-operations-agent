#!/usr/bin/env bash
set -euo pipefail

umask 077

env_file=${HWOPS_ENV_FILE:-/etc/hwops/hwops.env}
destination=${1:-}
if [[ ${EUID} -ne 0 || -z ${destination} ]]; then
  echo "usage: sudo $0 /absolute/backup-directory" >&2
  exit 2
fi
if [[ ! -r ${env_file} ]]; then
  echo "deployment environment is unavailable: ${env_file}" >&2
  exit 1
fi

set -a
# The deployment environment is root-owned trusted input.
source "${env_file}"
set +a
: "${HWOPS_DATABASE_URL:?HWOPS_DATABASE_URL is required}"
: "${HWOPS_FILES_DIR:?HWOPS_FILES_DIR is required}"

mkdir -p "${destination}"
destination=$(cd "${destination}" && pwd -P)
files_parent=$(cd "$(dirname "${HWOPS_FILES_DIR}")" && pwd -P)
files_name=$(basename "${HWOPS_FILES_DIR}")
case "${destination}/" in
  "${files_parent}/${files_name}/"*) echo "backup destination cannot be inside HWOPS_FILES_DIR" >&2; exit 1 ;;
esac

stamp=$(date -u +%Y%m%dT%H%M%SZ)
target="${destination}/hwops-${stamp}"
mkdir "${target}"

restart=false
if systemctl is-active --quiet hwopsd.service; then
  restart=true
  systemctl stop hwopsd.service
fi
cleanup() {
  if ${restart}; then
    systemctl start hwopsd.service
  fi
}
trap cleanup EXIT

pg_dump --dbname="${HWOPS_DATABASE_URL}" --format=custom --no-owner \
  --file="${target}/database.dump"
tar --create --gzip --file="${target}/files.tar.gz" \
  --directory="${files_parent}" "${files_name}"
(
  cd "${target}"
  sha256sum database.dump files.tar.gz > SHA256SUMS
)
printf 'created_at=%s\nfiles_name=%s\n' "${stamp}" "${files_name}" > "${target}/manifest"
sync "${target}/database.dump" "${target}/files.tar.gz" "${target}/SHA256SUMS" "${target}/manifest"

trap - EXIT
cleanup
echo "backup completed: ${target}"

#!/usr/bin/env bash
set -euo pipefail

umask 077

env_file=${HWOPS_ENV_FILE:-/etc/hwops/hwops.env}
backup=${1:-}
confirmation=${2:-}
if [[ ${EUID} -ne 0 || -z ${backup} || ${confirmation} != "--confirm-restore" ]]; then
  echo "usage: sudo $0 /absolute/hwops-backup --confirm-restore" >&2
  exit 2
fi
backup=$(cd "${backup}" && pwd -P)
if [[ ! -r ${env_file} || ! -f ${backup}/database.dump ||
      ! -f ${backup}/files.tar.gz || ! -f ${backup}/SHA256SUMS ]]; then
  echo "deployment environment or backup set is incomplete" >&2
  exit 1
fi

set -a
# The deployment environment is root-owned trusted input.
source "${env_file}"
set +a
: "${HWOPS_DATABASE_URL:?HWOPS_DATABASE_URL is required}"
: "${HWOPS_FILES_DIR:?HWOPS_FILES_DIR is required}"

(
  cd "${backup}"
  sha256sum --check SHA256SUMS
)

systemctl stop hwopsd.service
files_parent=$(dirname "${HWOPS_FILES_DIR}")
files_name=$(basename "${HWOPS_FILES_DIR}")
mkdir -p "${files_parent}"
staging=$(mktemp -d "${files_parent}/.hwops-restore.XXXXXX")
cleanup() {
  rm -rf "${staging}"
}
trap cleanup EXIT

tar --extract --gzip --file="${backup}/files.tar.gz" --directory="${staging}"
if [[ ! -d ${staging}/${files_name} ]]; then
  echo "backup file tree does not match HWOPS_FILES_DIR" >&2
  exit 1
fi

pg_restore --dbname="${HWOPS_DATABASE_URL}" --clean --if-exists --no-owner \
  --single-transaction "${backup}/database.dump"

old="${files_parent}/.${files_name}.pre-restore-$(date -u +%Y%m%dT%H%M%SZ)"
if [[ -e ${HWOPS_FILES_DIR} ]]; then
  mv "${HWOPS_FILES_DIR}" "${old}"
fi
mv "${staging}/${files_name}" "${HWOPS_FILES_DIR}"
chown -R hwops:hwops "${HWOPS_FILES_DIR}"

trap - EXIT
cleanup
systemctl start hwopsd.service
echo "restore completed; previous files retained at ${old}"

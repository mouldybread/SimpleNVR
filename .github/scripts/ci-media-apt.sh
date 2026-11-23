#!/usr/bin/env bash
set -euo pipefail

sources=/etc/apt/sources.list.d/ubuntu.sources
[[ -s "$sources" ]] || { echo "Ubuntu APT sources not found: $sources" >&2; exit 1; }

# Tried in order. archive.ubuntu.com is deliberately not listed: it was too slow in CI.
mirrors=(
  http://azure.archive.ubuntu.com/ubuntu
  https://mirrors.edge.kernel.org/ubuntu
  http://mirror.math.princeton.edu/pub/ubuntu
)

opts=(
  -o "Dir::Etc::sourcelist=$sources"
  -o "Dir::Etc::sourceparts=-"
  -o Acquire::Retries=1
  -o Acquire::http::Timeout=20
  -o Acquire::https::Timeout=20
)

case "${1:-}" in
  update)
    for mirror in "${mirrors[@]}"; do
      echo "apt update using $mirror"
      # Persist the working mirror on this CI runner so later installs and Playwright's APT calls also use it.
      sudo sed -Ei "s@^URIs:.*\$@URIs: ${mirror}@" "$sources"
      if sudo apt-get "${opts[@]}" -o APT::Update::Error-Mode=any update; then
        exit 0
      fi
      echo "::warning::apt mirror unavailable: $mirror" >&2
    done
    echo "All configured apt mirrors failed" >&2
    exit 1
    ;;
  install) shift; sudo apt-get "${opts[@]}" install -y --no-install-recommends "$@" ;;
  *) echo "Expected update or install" >&2; exit 2 ;;
esac

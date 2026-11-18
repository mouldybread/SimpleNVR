#!/usr/bin/env bash
set -euo pipefail

sources=/etc/apt/sources.list.d/ubuntu.sources
[[ -s "$sources" ]] || { echo "Ubuntu APT sources not found: $sources" >&2; exit 1; }

opts=(
  -o "Dir::Etc::sourcelist=$sources"
  -o "Dir::Etc::sourceparts=-"
  -o Acquire::Retries=1
  -o Acquire::http::Timeout=20
  -o Acquire::https::Timeout=20
)

case "${1:-}" in
  update) sudo apt-get "${opts[@]}" -o APT::Update::Error-Mode=any update ;;
  install) shift; sudo apt-get "${opts[@]}" install -y --no-install-recommends "$@" ;;
  *) echo "Expected update or install" >&2; exit 2 ;;
esac

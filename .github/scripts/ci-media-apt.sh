#!/usr/bin/env bash
set -euo pipefail

sources=/etc/apt/sources.list.d/ubuntu.sources
mirrorlist=/etc/apt/ci-ubuntu-mirrors.txt
[[ -s "$sources" ]] || { echo "Ubuntu APT sources not found: $sources" >&2; exit 1; }

opts=(
  -o "Dir::Etc::sourcelist=$sources"
  -o "Dir::Etc::sourceparts=-"
  -o Acquire::Retries=1
  -o Acquire::http::Timeout=20
  -o Acquire::https::Timeout=20
)

case "${1:-}" in
  update)
    # apt's mirror method tries the lowest priority first and moves to the next mirror
    # for any file that fails to download. This covers update, install and Playwright's
    # own apt calls. archive.ubuntu.com is deliberately not listed: it was too slow in CI.
    printf '%s\n' \
      $'http://azure.archive.ubuntu.com/ubuntu/\tpriority:1' \
      $'https://mirrors.edge.kernel.org/ubuntu/\tpriority:2' \
      $'http://mirror.math.princeton.edu/pub/ubuntu/\tpriority:3' | sudo tee "$mirrorlist" >/dev/null
    printf '%s\n' 'Acquire::Retries "2";' 'Acquire::http::Timeout "20";' 'Acquire::https::Timeout "20";' | sudo tee /etc/apt/apt.conf.d/99ci-apt-retries >/dev/null
    # Persist on this CI runner so Playwright's APT calls also use it.
    sudo sed -Ei "s@^URIs:.*\$@URIs: mirror+file:${mirrorlist}@" "$sources"
    sudo apt-get "${opts[@]}" -o APT::Update::Error-Mode=any update
    ;;
  install) shift; sudo apt-get "${opts[@]}" install -y --no-install-recommends "$@" ;;
  *) echo "Expected update or install" >&2; exit 2 ;;
esac

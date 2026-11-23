#!/usr/bin/env bash
set -euo pipefail

sources=/etc/apt/sources.list.d/ubuntu.sources
mirrorlist=/etc/apt/ci-ubuntu-mirrors.txt
[[ -s "$sources" ]] || { echo "Ubuntu APT sources not found: $sources" >&2; exit 1; }

# archive.ubuntu.com is deliberately not listed: it was too slow in CI.
MIRRORS=(
  "http://azure.archive.ubuntu.com/ubuntu/"
  "https://mirrors.edge.kernel.org/ubuntu/"
  "http://mirror.math.princeton.edu/pub/ubuntu/"
)

opts=(
  -o "Dir::Etc::sourcelist=$sources"
  -o "Dir::Etc::sourceparts=-"
  -o Acquire::Retries=1
  -o Acquire::http::Timeout=20
  -o Acquire::https::Timeout=20
)

# Writes the mirror list with $1 first. apt's mirror method tries the lowest priority
# first. The list keeps the same file name, so the package lists already downloaded stay
# valid whichever mirror is first.
write_mirrorlist() {
  local first=$1 priority=1 mirror
  {
    printf '%s\tpriority:%d\n' "$first" "$priority"
    for mirror in "${MIRRORS[@]}"; do
      if [[ "$mirror" != "$first" ]]; then
        priority=$((priority + 1))
        printf '%s\tpriority:%d\n' "$mirror" "$priority"
      fi
    done
  } | sudo tee "$mirrorlist" >/dev/null
}

# Mirror that is currently first, then the rest in their configured order.
ordered_mirrors() {
  local current="" mirror
  if [[ -s "$mirrorlist" ]]; then
    current=$(head -n1 "$mirrorlist" | cut -f1)
  fi
  if [[ -n "$current" ]]; then
    printf '%s\n' "$current"
  fi
  for mirror in "${MIRRORS[@]}"; do
    if [[ "$mirror" != "$current" ]]; then
      printf '%s\n' "$mirror"
    fi
  done
}

# apt's own timeout only fires when no data arrives, so a mirror that trickles bytes can
# stall a step until the job limit. Give each mirror a wall-clock limit instead and move
# to the next one when it runs out. Partial downloads are kept and resumed. The mirror
# that succeeds stays first for later apt calls, such as Playwright's.
run_with_mirror_rotation() {
  local seconds=$1 mirror
  shift
  local -a order
  mapfile -t order < <(ordered_mirrors)
  for mirror in "${order[@]}"; do
    write_mirrorlist "$mirror"
    echo "Trying Ubuntu mirror ${mirror} (limit ${seconds}s)"
    if sudo timeout --kill-after=10 "$seconds" "$@"; then
      return 0
    fi
    echo "::warning::Mirror ${mirror} did not finish within ${seconds}s or failed; trying the next mirror"
  done
  return 1
}

case "${1:-}" in
  update)
    printf '%s\n' 'Acquire::Retries "2";' 'Acquire::http::Timeout "20";' 'Acquire::https::Timeout "20";' | sudo tee /etc/apt/apt.conf.d/99ci-apt-retries >/dev/null
    # Persist on this CI runner so Playwright's APT calls also use it.
    write_mirrorlist "${MIRRORS[0]}"
    sudo sed -Ei "s@^URIs:.*\$@URIs: mirror+file:${mirrorlist}@" "$sources"
    run_with_mirror_rotation "${CI_APT_UPDATE_SECONDS:-75}" apt-get "${opts[@]}" -o APT::Update::Error-Mode=any update
    ;;
  install)
    shift
    # Download first, one wall-clock-limited attempt per mirror, then install from the
    # downloaded files. If every attempt fails, the normal install below still runs.
    if ! run_with_mirror_rotation "${CI_APT_DOWNLOAD_SECONDS:-120}" apt-get "${opts[@]}" install -y --no-install-recommends --download-only "$@"; then
      echo "::warning::Pre-download did not finish on any mirror; running the normal install"
    fi
    sudo apt-get "${opts[@]}" install -y --no-install-recommends "$@"
    ;;
  *) echo "Expected update or install" >&2; exit 2 ;;
esac

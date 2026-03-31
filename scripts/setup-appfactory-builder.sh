#!/usr/bin/env bash

set -euo pipefail

command="${1:-doctor}"
shift || true

print_usage() {
  cat <<'EOF'
Usage:
  setup-appfactory-builder doctor
  setup-appfactory-builder smoke
  setup-appfactory-builder shell
  setup-appfactory-builder exec <command...>

Commands:
  doctor  Print Flutter and Android toolchain status.
  smoke   Run minimal builder environment checks.
  shell   Start an interactive shell.
  exec    Run a custom command inside the prepared environment.
EOF
}

run_doctor() {
  echo "== flutter --version =="
  flutter --version
  echo
  echo "== flutter doctor -v =="
  flutter doctor -v
  echo
  echo "== sdkmanager --list_installed =="
  sdkmanager --list_installed
}

run_smoke() {
  run_doctor
  echo
  echo "== dart --version =="
  dart --version
  echo
  echo "== java -version =="
  java -version
  echo
  echo "== adb version =="
  adb version
}

case "$command" in
  doctor)
    run_doctor
    ;;
  smoke)
    run_smoke
    ;;
  shell)
    exec /bin/bash "$@"
    ;;
  exec)
    if [ "$#" -eq 0 ]; then
      echo "missing command for exec" >&2
      exit 1
    fi
    exec "$@"
    ;;
  help|-h|--help)
    print_usage
    ;;
  *)
    echo "unknown command: $command" >&2
    print_usage >&2
    exit 1
    ;;
esac
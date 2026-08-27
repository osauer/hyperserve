#!/bin/sh
set -eu

case "${1:-} ${2:-}" in
  "run list")
    printf '%s\n' "${FAKE_RUN_ID-321}"
    ;;
  "run watch")
    exit "${FAKE_WATCH_EXIT:-0}"
    ;;
  "run view")
    case "$*" in
      *"--json headSha --jq .headSha"*)
        printf '%s\n' "${FAKE_HEAD_SHA-0123456789abcdef0123456789abcdef01234567}"
        ;;
      *"--json event --jq .event"*)
        printf '%s\n' "${FAKE_EVENT-push}"
        ;;
      *"--json status --jq .status"*)
        printf '%s\n' "${FAKE_STATUS-completed}"
        ;;
      *"--json conclusion --jq .conclusion"*)
        printf '%s\n' "${FAKE_CONCLUSION-success}"
        ;;
      *"--json url --jq .url"*)
        printf '%s\n' "${FAKE_URL-https://example.invalid/actions/runs/321}"
        ;;
      *"--json jobs --template"*)
        printf '%b' "${FAKE_JOBS-Test\tsuccess\nBenchmark\tsuccess\n}"
        ;;
      *)
        echo "fake-gh: unsupported run view invocation: $*" >&2
        exit 2
        ;;
    esac
    ;;
  *)
    echo "fake-gh: unsupported invocation: $*" >&2
    exit 2
    ;;
esac

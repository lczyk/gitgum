#!/usr/bin/env -S bash -e

# emits a mix of stdout entries and stderr noise to exercise Sync's
# tearing recovery. pipe into ff:  ./scripts/tearing-test.sh 2>&1 | ff

ITEMS=20
STDERR_EVERY=4

function main() {
    for ((i=1; i<=ITEMS; i++)); do
        printf 'item-%03d some/path/to/file-%d.txt\n' "$i" "$i"

        if ((i % STDERR_EVERY == 0)); then
            printf 'tearing-test: permission denied: /fake/dir-%d\n' "$i" >&2
        fi

        sleep 0.05
    done
}

main "$@"

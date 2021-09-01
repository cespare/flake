#!/bin/bash
set -eu -o pipefail

# echo $FLAKEDIR
echo "stdout"
echo "stderr" >&2

((RANDOM == 1)) || exit 0
exit 1

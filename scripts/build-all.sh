#!/usr/bin/env bash
# Builda o node PANDA para TODAS as plataformas (Linux, macOS e Windows).
# Para distribuir para a turma de uma vez só.
#
# Uso:  scripts/build-all.sh
set -euo pipefail
dir="$(dirname "${BASH_SOURCE[0]}")"

"$dir/build-linux.sh"
"$dir/build-macos.sh"
"$dir/build-windows.sh"

echo "🎉 todos os alvos buildados."

#!/usr/bin/env bash
# La frontera que sabe fallar: comprueba que un módulo ajeno NO puede importar
# nada de internal/.
#
# Qué hace, en una carpeta temporal fuera de este repositorio. Los dos pasos
# importan EL MISMO paquete con EL MISMO montaje (require + replace hacia este
# repositorio); lo único que cambia es la ruta del módulo que importa:
#
#   1. CONTROL. Un módulo cuya ruta empieza igual que la de éste
#      («<este módulo>/control») importa internal/servicio. La regla de
#      internal/ va por ruta de importación, así que a éste SÍ lo deja: tiene
#      que compilar. Si no compila, el experimento está mal montado (el paquete
#      no existe, la ruta del replace está rota, el go.mod no sirve) y el paso 2
#      no probaría nada: se para en rojo.
#   2. VIOLACIÓN. Un módulo con una ruta ajena («ejemplo.com/intruso») importa
#      el mismo paquete. Tiene que FALLAR, y con el mensaje de la regla de
#      internal/ («use of internal package ... not allowed»). Si compila, o si
#      falla por cualquier otra razón, el guion sale en rojo.
#
# Uso:  bash herramientas/probar-frontera.sh   → sale 0 si la frontera se sostiene
#
# Por qué así: un experimento que solo espera un rojo se pone «verde» también
# cuando falla por la razón equivocada (una ruta mal escrita, un go.mod roto).
# Dos cosas lo impiden. El control resuelve de verdad el paquete por el mismo
# montaje, así que un montaje roto lo pone rojo a él. Y la violación exige el
# mensaje exacto de la regla, no cualquier fallo. Entre los dos pasos solo
# cambia quién importa: la regla queda demostrada por sus dos lados.
set -euo pipefail

raiz="$(cd "$(dirname "$0")/.." && pwd)"
modulo="$(cd "$raiz" && go list -m)"
paquete="$modulo/internal/servicio"

tmp="$(mktemp -d)"
# Al salir se borra la carpeta temporal, y el código de salida se conserva. Con
# el bash 3.2 que trae macOS, si el guion se corta por una variable sin definir
# (set -u), la trampa recibe un 0 en «$?»: sin la bandera «terminado», un
# guion cortado a la mitad saldría 0, un falso verde (medido). Por eso, si no
# llegó al final, sale 1 aunque «$?» diga 0.
salir() {
  local codigo=$?
  rm -rf "$tmp"
  if [ "$codigo" -eq 0 ] && [ "${terminado:-}" != 1 ]; then
    echo "❌ el guion se cortó antes de terminar" >&2
    codigo=1
  fi
  exit "$codigo"
}
trap salir EXIT

# armar <carpeta> <ruta del módulo>: un módulo que depende de éste e importa
# $paquete. Vive FUERA del árbol de este repositorio.
armar() {
  mkdir -p "$1"
  cat > "$1/go.mod" <<EOF
module $2

go 1.27

require $modulo v0.0.0

replace $modulo => $raiz
EOF
  cat > "$1/main.go" <<EOF
package main

import "$paquete"

func main() { _ = servicio.Servicio{} }
EOF
}

# GOWORK=off: que un go.work de la máquina no cambie el experimento.
compilar() { (cd "$1" && GOWORK=off GOFLAGS=-mod=mod go build -o /dev/null . 2>&1); }

echo "1/2 control: $modulo/control importa $paquete"
armar "$tmp/control" "$modulo/control"
if ! salida="$(compilar "$tmp/control")"; then
  echo "❌ el control NO compiló; el experimento está mal montado:"
  printf '%s\n' "$salida" | sed 's/^/     /'
  exit 1
fi
echo "   ✅ compiló: con la misma ruta de arriba, la regla lo deja pasar"

echo "2/2 violación: ejemplo.com/intruso importa $paquete"
armar "$tmp/intruso" "ejemplo.com/intruso"
if salida="$(compilar "$tmp/intruso")"; then
  echo "❌ COMPILÓ: otro módulo pudo importar $paquete. La frontera no existe."
  exit 1
fi
if ! printf '%s\n' "$salida" | grep -F -q "use of internal package $paquete not allowed"; then
  echo "❌ falló, pero NO por la regla de internal/. Esto no demuestra la frontera:"
  printf '%s\n' "$salida" | sed 's/^/     /'
  exit 1
fi
printf '%s\n' "$salida" | sed 's/^/     /'
echo "   ✅ la herramienta go lo rechazó por la regla de internal/"
echo "✅ frontera: se sostiene"
terminado=1

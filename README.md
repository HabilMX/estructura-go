> El código es MIT; el texto y las explicaciones son CC BY-SA 4.0.

# estructura-go

**Una plantilla de Go que se clona, corre a la primera y trae la estructura que recomienda la guía
oficial de Go para un servidor**: el programa en `cmd/`, el resto en `internal/`, y nada más hasta que
haga falta.

Es una **semilla**, no un molde completo: trae una funcionalidad real de punta a punta (un servidor que
revisa si otros servicios responden), sus pruebas y un CI que comprueba la única frontera que Go vigila
por ti. Lo demás crece cuando duela, y la sección [«Cómo crece»](#cómo-crece-cuándo-agregar-cada-cosa)
dice cuándo.

Es parte de la serie «La estructura de un proyecto», de Hábil. El ejemplo es el `revisor` del
[curso de Go](https://www.habil.mx/es/cursos/go/).

---

## Córrelo

Necesitas Go 1.27 o más nuevo (`go version`). Si tu sistema trae uno más viejo, baja el de
[go.dev/dl](https://go.dev/dl/) y sigue [go.dev/doc/install](https://go.dev/doc/install). El guion de
la frontera necesita `bash`: en Linux y macOS ya está; en Windows, usa WSL o Git Bash.

```bash
go test ./...              # las pruebas, sin red: todas deben salir «ok»
go run ./cmd/revisor       # arranca en :8080 con testdata/servicios.json (Ctrl+C lo apaga)
curl -s localhost:8080/servicios   # en otra terminal
```

Lo que contestó el 22-oct-2026 (partido en tres líneas para que quepa; las duraciones cambian, y `go.dev`
sale en `"ok": true` solo si tienes internet):

```json
[{"nombre":"el-propio-revisor","url":"http://localhost:8080/salud","ok":true,"codigo":200,"duracion_ms":2},
{"nombre":"go.dev","url":"https://go.dev","ok":true,"codigo":200,"duracion_ms":140},
{"nombre":"no-existe","url":"http://servicio-que-no-existe.invalid","ok":false,"duracion_ms":1,"motivo":"no responde: dial tcp: lookup servicio-que-no-existe.invalid: no such host"}]
```

Las otras dos rutas:

```bash
curl -s localhost:8080/salud                 # {"estado":"ok"}
curl -si localhost:8080/servicios/nomina     # 404 con Content-Type: application/problem+json
```

```
HTTP/1.1 404 Not Found
Content-Type: application/problem+json

{"type":"about:blank","title":"Not Found","status":404,"detail":"No hay ningún servicio llamado \"nomina\".","instance":"/servicios/nomina"}
```

Para la frontera que sabe fallar (ver abajo):

```bash
bash herramientas/probar-frontera.sh
```

---

## Qué es cada carpeta y por qué existe

```
estructura-go/
├── go.mod                       el módulo: su ruta y la versión de Go
├── cmd/
│   └── revisor/                 el programa: lee la configuración, arma las piezas y arranca
├── internal/                    todo lo demás; desde otro módulo no se puede importar
│   ├── servicio/                el vocabulario: Servicio, Estado, qué es «sano». Sin red ni archivos
│   ├── revisar/                 consultar muchos servicios a la vez; declara la interfaz que USA
│   ├── httpapi/                 la puerta HTTP: rutas, JSON y sus errores con Problem Details
│   └── config/                  banderas, variables de entorno y la lista de servicios
├── testdata/servicios.json      el ejemplo que usa «Córrelo»; go ignora esta carpeta al compilar
├── herramientas/
│   └── probar-frontera.sh       demuestra que internal/ de verdad está cerrado
└── .github/workflows/verificar.yml
```

**`cmd/revisor/`.** La guía oficial *Organizing a Go module* recomienda, para un servidor, guardar los
programas en `cmd/` y la lógica en `internal/`. `main.go` es delgado a propósito: si algo de ahí merece
una prueba, ya no pertenece ahí.

**`internal/`.** Es la única carpeta de este árbol que la herramienta `go` vigila. Un paquete bajo
`internal/` solo lo puede importar código que comparte la ruta de arriba de esa carpeta. La regla va
por ruta de importación, no por módulo; en la práctica, un módulo ajeno no puede importarlo.
Un servidor no exporta paquetes para nadie, así que todo vive aquí y puedes reacomodarlo sin romper a
nadie.

**`internal/servicio/`.** El dominio. No importa a ningún otro paquete de la plantilla y todos los
paquetes de `internal/` lo importan a él, así que nunca puede formar parte de un ciclo de importaciones
(la especificación de Go prohíbe que un paquete se importe a sí mismo, directa o indirectamente). Se
llama por lo que contiene.

**`internal/revisar/`.** La consulta concurrente. La interfaz `Consultor` vive aquí porque aquí se usa:
*Go Code Review Comments* pide declarar las interfaces en el paquete que las usa, no en el que las
implementa. La consulta HTTP real es un tipo concreto, y el doble de pruebas vive en un `_test.go`.

**`internal/httpapi/`.** Traduce HTTP a llamadas y resultados a JSON. Usa las rutas con método y comodín
de `net/http` (`GET /servicios/{nombre}`), sin dependencias externas. Los errores que escribe la
plantilla salen en formato Problem Details (RFC 9457), el mismo del curso de HTTP de Hábil; los que
contesta el enrutador por su cuenta (un `405` por método equivocado, el `404` de una ruta que no
existe) salen en texto plano. La URL de cada servicio sale **sin usuario, contraseña ni query
string**, y el motivo de una falla no repite la URL: un token pegado en una URL no debe terminar en
una respuesta pública (lo comprueba `TestLaURLPublicaNoLlevaSecretos`). La ruta sí sale, porque dice
qué se revisa: un token metido en la ruta (`/hooks/<token>`) se publicaría entero, así que no lo
pongas ahí.

**`internal/config/`.** Lee banderas y variables de entorno (`REVISOR_DIRECCION`, `REVISOR_SERVICIOS`)
y valida la lista de servicios. Rechaza una URL con credenciales (`https://usuario:clave@...`) sin
repetirla en el error: las credenciales van en otro lado, nunca en la URL. Recibe el entorno y el
archivo como parámetros, así que se prueba sin tocar ninguno de los dos.

**`testdata/`.** La herramienta `go` ignora las carpetas llamadas `testdata`: es el lugar para datos de
apoyo. Aquí guarda el ejemplo, y una prueba (`TestElEjemploDelRepoEsValido`) lo valida.

---

## La frontera que sabe fallar

`herramientas/probar-frontera.sh` arma, en una carpeta temporal, dos módulos que dependen de éste por
el mismo camino e importan el mismo paquete, `internal/servicio`. Solo cambia la ruta de quien importa:

1. **Control:** un módulo cuya ruta empieza igual que la de éste
   (`github.com/HabilMX/estructura-go/control`). La regla lo deja pasar, así que tiene que compilar. Si
   no compila, el experimento está mal montado (el paquete no existe, la ruta está rota) y el guion
   para en rojo.
2. **Violación:** un módulo ajeno (`ejemplo.com/intruso`). Tiene que fallar **con el mensaje de la
   regla**:

```
main.go:3:8: use of internal package github.com/HabilMX/estructura-go/internal/servicio not allowed
```

Si compila, o si falla por otra razón, el guion sale en rojo: exige ese mensaje exacto, no cualquier
fallo. El CI lo corre en cada cambio.

**Rómpelo tú**, en dos pasos, y mira dos rojos distintos:

1. Renombra `internal/` a `interno/` y cambia los imports. En Linux:

   ```bash
   mv internal interno
   grep -rl 'estructura-go/internal/' --include='*.go' . | xargs sed -i 's#estructura-go/internal/#estructura-go/interno/#'
   ```

   En macOS, el `sed` de BSD pide un argumento vacío después de `-i`:

   ```bash
   mv internal interno
   grep -rl 'estructura-go/internal/' --include='*.go' . | xargs sed -i '' 's#estructura-go/internal/#estructura-go/interno/#'
   ```

   `go build ./...` sigue compilando. Corre el guion: sale
   `❌ el control NO compiló; el experimento está mal montado`, porque busca un paquete que ya no
   existe. Ese rojo es el control negándose a dar un verde por la razón equivocada.
2. En `herramientas/probar-frontera.sh`, en la línea `paquete=`, cambia `internal` por `interno`. Corre
   el guion otra vez: `❌ COMPILÓ: otro módulo pudo importar github.com/HabilMX/estructura-go/interno/servicio. La frontera no existe.`

Para regresar todo: `git checkout -- . && git clean -fd`. Ojo: `git clean -fd` también borra
cualquier archivo nuevo que hayas creado en el repositorio, como tus notas.

---

## Qué NO trae y por qué

| No trae | Por qué |
|---|---|
| `pkg/` | Un servidor no tiene paquetes para que otros importen. La guía oficial no menciona `pkg/`, y Russ Cox señaló en 2021 que la gran mayoría de los paquetes de Go tampoco ([issue #117](https://github.com/golang-standards/project-layout/issues/117)) |
| `util/`, `common/`, `misc/` | El blog de Go los llama nombres sin significado: no dicen qué hay adentro y acumulan dependencias ([Package names](https://go.dev/blog/package-names)) |
| `api/`, `types/`, `interfaces/` como cajón de todas las interfaces | El mismo artículo lo llama un error: sufren los mismos problemas que `util`. Cada interfaz vive junto a quien la usa |
| `web/`, `configs/`, `build/`, `deployments/`, `scripts/` | Son carpetas de `golang-standards/project-layout`, un repositorio que se presenta como **no** oficial y avisa que para un proyecto simple es «an overkill». Se agregan cuando aparece lo que guardan |
| Base de datos, Docker, autenticación | Fuera del ejemplo. Ver «Cómo crece» |
| Framework web o dependencias | `net/http` ya trae rutas con método y comodín desde Go 1.22 |

---

## Cómo crece: cuándo agregar cada cosa

| Cuando… | Agrega… | Y deja… |
|---|---|---|
| Aparece un segundo programa (un migrador, una tarea nocturna) | `cmd/<nombre>/` con su `main.go` delgado | la lógica compartida en `internal/` |
| Un paquete de `internal/` crece y mezcla dos temas | dos paquetes con nombre de lo que hacen (`internal/notificar/`) | sin `util/` |
| Guardas estado en una base de datos | `internal/<algo>/` con el código que habla con la base; si el dominio necesita una interfaz para eso, se declara del lado que la usa | `servicio/` sin SQL |
| Otro proyecto necesita **de verdad** importar algo de aquí | sácalo de `internal/` a un paquete público, o a su propio módulo, como sugiere la guía oficial | todo lo demás en `internal/` |
| Hay archivos que no son Go (contrato OpenAPI, manifiestos de despliegue) | la carpeta que los nombre (`api/`, `deploy/`) en la raíz | el Go en `cmd/` e `internal/` |
| Se necesita un contenedor | un `Dockerfile` en la raíz | — |

Regla para decidir: **una carpeta nueva tiene que resolver un problema que ya tienes**. Si no puedes
decir cuál, todavía no la necesitas.

---

## Cómo quitar el ejemplo

El `revisor` está para demostrar la estructura. Para empezar tu propio proyecto:

1. Cambia la ruta del módulo: `go mod edit -module github.com/<tu-usuario>/<tu-proyecto>` y reemplaza la
   ruta vieja en los imports (`grep -rl HabilMX/estructura-go --include=*.go .`).
2. Renombra `cmd/revisor/` al nombre de tu programa.
3. Borra `internal/servicio/`, `internal/revisar/` y `testdata/servicios.json`.
4. Reescribe `internal/httpapi/` y `internal/config/` con tus rutas y tu configuración. `/salud` y
   `escribirProblema` suelen sobrevivir.
5. En `herramientas/probar-frontera.sh`, cambia `internal/servicio` por un paquete tuyo de `internal/`.
6. Corre `go vet ./... && go test ./... && bash herramientas/probar-frontera.sh`.
7. Reescribe este README. El código y `LICENSE` son MIT; si conservas el texto de los `.md`, mira
   `LICENSE-CONTENIDO.md`.

---

## Versiones

Medidas el 22 de octubre de 2026.

| Pieza | Versión |
|---|---|
| Go | 1.27 en `go.mod`. Probado con 1.27.1 y 1.27.3, la estable más reciente en go.dev/dl ese día |
| Dependencias | Ninguna: solo la biblioteca estándar |
| CI | GitHub Actions: `actions/checkout@v4`, `actions/setup-go@v5` (las mismas que los cursos de Hábil); corre en cada cambio, a mano y cada lunes. GitHub apaga la corrida de los lunes si el repositorio público pasa 60 días sin actividad |
| Sistemas | macOS 27 (arm64) y Linux (contenedor `golang:1.27.3-trixie`, arm64). Falta Linux Mint en x86-64 |

---

## Licencia

El código es [MIT](LICENSE). El texto y las explicaciones son
[CC BY-SA 4.0](LICENSE-CONTENIDO.md).

---
title: Desarrollo
description: Estructura del código, cómo compilar, cómo ejecutar los tests y cómo se prueba nixup sin tocar la red ni tu sistema.
lang: es
---

# Desarrollo

[English](../development.md) | **Español**

## Estructura

```text
cmd/nixup/          main: conecta la terminal real, el ejecutor de procesos y el cliente de GitHub
internal/
  cli/              parseo de flags, subcomandos, códigos de salida
  check/            las comprobaciones de solo lectura: inputs, packages, profile, flatpak
  update/           los dos comandos que escriben: Lock (flake.lock) y Pkg (packages/)
  report/           salida en texto para la terminal y en Markdown
  flake/            flake.lock, localizar el flake, hosts, paquetes propios, copia temporal
  gh/               tags y ramas con git; commits y release notes con la API
  ver/              orden de versiones, detección de prereleases, avisos de salto mayor
  run/              el único sitio que lanza procesos (Runner), más un Fake programable
  ui/               salida por terminal, color, preguntas de sí/no
  par/              concurrencia acotada
  exit/             códigos de salida comunes a todos los comandos
  testutil/         API de GitHub falsa, árboles y terminales temporales (solo para tests)
```

Las dependencias van en un solo sentido: `cli → update → report → check → flake → gh → run, ver`. Dos reglas hacen que el código se pueda probar:

- Solo `internal/run` lanza procesos. Todo lo demás recibe un `run.Runner`.
- Solo `internal/ui` habla con la terminal. `main` le pasa stderr al ejecutor para que los builds salgan por ahí.

El código usa solo la biblioteca estándar de Go. `go.mod` no tiene dependencias.

## Compilar y ejecutar

```bash
nix develop                          # go, gopls, nix-update
go build -o nixup ./cmd/nixup
./nixup help
```

Un `go build` normal se identifica como `nixup dev`. El flake fija la versión y el commit reales con `-ldflags -X` sobre `internal/cli.Version` e `internal/cli.Commit`:

```bash
nix build
./result/bin/nixup version
```

## Tests

```bash
go test -race ./...
nix build            # también ejecuta los tests, dentro del sandbox de Nix
```

La CI (`.github/workflows/ci.yml`) se ejecuta en cada push a `main` y en las pull requests: `gofmt`, `go vet`, `go test -race -count=1`, `nix build -L` y `nix flake check`. Ejecuta `gofmt -l .` antes de hacer push; si imprime algo, la CI falla.

## Cómo funcionan los tests

Los tests nunca tocan la red, tus repositorios ni tu sistema.

**`run.Fake`** sustituye al ejecutor de procesos. Cada `run.Rule` responde al primer comando cuya línea de comandos empieza por su `Prefix`, con un stdout, un stderr y, si quieres, un error. La función `Do` de una regla se ejecuta antes de responder, para simular efectos secundarios como que `nix-update` edite un archivo. Los comandos sin regla terminan bien y sin salida. `Missing` hace que `Has` diga que una herramienta no está, y `Calls` y `Count` permiten comprobar en un test qué se ha ejecutado, o que algo no se ha ejecutado nunca:

```go
f := (&run.Fake{}).
	On(run.Rule{Prefix: "nix flake update", Fail: errors.New("exit 1"), Err: "error: boom\n"})
// ... ejecuta el código a probar con f como Runner ...
if f.Count("nh os build") != 0 {
	t.Error("must not build after a failed update")
}
```

**`testutil.New`** monta un mundo falso encima: un `run.Fake` y un servidor `httptest` que hace de API de GitHub. Vacía `GH_TOKEN` y `GITHUB_TOKEN` y marca `gh` como ausente, así que no se lee ningún token real. Para el resto hay helpers:

- `Tags` y `Head` responden a `git ls-remote` para un repositorio.
- `Compare` y `Releases` responden a los endpoints de comparación y de releases de la API. `API` responde a cualquier otra ruta.
- `Tree` escribe un directorio temporal a partir de un mapa de archivos.
- `Term` devuelve una terminal que captura stdout y stderr, y que contesta a las preguntas con una cadena si se la das.
- `LockJSON` construye un `flake.lock` mínimo.

Unos pocos tests ejecutan procesos reales a propósito: los tests del paquete `run` lanzan `sh`, y un test de `flake` crea un repositorio git temporal para probar contra git de verdad la copia temporal, la comprobación de cambios sin commitear y la restauración. Si `git` no está instalado, ese test se salta.

## Commits

Los mensajes de commit siguen [Conventional Commits](https://www.conventionalcommits.org/). Las release notes se generan a partir de ellos, así que el tipo decide en qué sección aparece cada cambio. Lo tienes en [Releases](releases.md).

---
title: Comandos
description: Todos los comandos de nixup, sus flags, formatos de salida y códigos de salida.
lang: es
---

# Comandos

[English](../commands.md) | **Español**

## Sinopsis

```text
nixup [check] [flags]
nixup lock [input...] [flags]
nixup pkg [package...] [flags]
nixup list [flags]
nixup search <name> [flags]
nixup version
nixup help
```

Sin comando, nixup ejecuta `check`, así que `nixup`, `nixup --review` y `nixup check --review` hacen lo mismo.

Los flags pueden ir antes, entre o después de los nombres: `nixup lock nixpkgs -y home-manager` actualiza dos inputs sin preguntar. Los flags admiten uno o dos guiones (`-y`, `--review`, `--only=inputs`).

Todos los comandos que trabajan sobre un flake aceptan `-C <dir>`. En [Configuración](configuration.md) se explica cómo se encuentra el flake si no pasas `-C`.

`nixup <comando> -h` imprime los flags de ese comando por stderr y sale con 0.

## Salidas

- Los resultados, las revisiones y las preguntas van a stdout.
- Los errores, y la línea `nixup: using <path> ($VAR)` (en [Configuración](configuration.md) se explica cuándo sale), van a stderr.
- La salida de `nix flake update`, `nh os build`, `nix-update` y `nix build` se vuelca en directo a stderr.

Así, `nixup check --json > out.json` te da un JSON limpio.

## check

Lista lo que está desactualizado. Solo lee.

```bash
nixup check
nixup check --review
nixup check --only inputs,packages --all
nixup check --json
nixup check --markdown --review
```

| Flag | Descripción |
|---|---|
| `-C <dir>` | Directorio del flake. |
| `--only <lista>` | Secciones a ejecutar, separadas por comas: `inputs`, `packages`, `profile`, `flatpak`. Por defecto, todas. Un nombre desconocido es un error. |
| `--all` | Lista también las entradas al día en la salida de texto. Sin él solo se cuentan. |
| `--review` | Añade la revisión de upstream (commits, enlace de comparación, release notes) a cada entrada desactualizada. |
| `--json` | Imprime todas las filas en JSON, incluidas las que están al día. Tiene prioridad sobre `--markdown`. |
| `--markdown` | Imprime solo las entradas desactualizadas o con error, en Markdown, para el cuerpo de una pull request o un resumen de CI. Si no hay nada, imprime `Everything is up to date.` |

`check` no recibe nombres; lo que escribas detrás se ignora.

La salida de texto marca cada entrada como `UPDATE`, `ok`, `info` o `ERROR`. En JSON esos estados son `outdated`, `ok`, `info` y `error`:

```json
[
  {
    "section": "packages",
    "name": "ripgrep",
    "current": "14.0.0",
    "latest": "15.2.0",
    "status": "outdated",
    "note": "nixup pkg ripgrep",
    "warning": "MAJOR version jump: read the changelog, expect breaking changes"
  }
]
```

`latest`, `note`, `warning`, `hint` y `review` no aparecen si están vacíos. `hint` indica que el nixpkgs del flake ya tiene el paquete (ver [Cómo funciona](how-it-works.md#packages)). Con `--review`, `review` contiene `repo`, `from`, `to`, `compare_url`, `commits`, `subjects`, `releases` (cada una con `tag` y `body`), `skipped_prereleases` y, si la API de GitHub ha fallado, `error`.

Códigos de salida:

| Código | Significado |
|---|---|
| 0 | Todo lo comprobado está al día. |
| 1 | Al menos una entrada está desactualizada. |
| 2 | Al menos una comprobación ha fallado, o la línea de comandos o la ubicación del flake estaban mal. Un fallo pesa más que las entradas desactualizadas. |

## lock

Actualiza `flake.lock` en una copia temporal, te enseña qué se ha movido, hace allí el build de los hosts y escribe el nuevo lock en tu repositorio cuando confirmas.

```bash
nixup lock
nixup lock nixpkgs home-manager
nixup lock -H desktop -H laptop
nixup lock --no-build
nixup lock -y --no-build --report report.md
```

Los nombres son inputs del flake; sin nombres se actualizan todos.

| Flag | Descripción |
|---|---|
| `-C <dir>` | Directorio del flake. |
| `-H <host>` | Host del que hacer el build. Se puede repetir. Por defecto, todas las entradas de `nixosConfigurations`. |
| `--no-build` | Actualiza y enseña los cambios, pero no hace el build. |
| `-y` | Escribe el nuevo lock sin preguntar. Necesario en ejecuciones desatendidas. |
| `--report <archivo>` | Añade al final de este archivo un informe en Markdown (inputs movidos, revisiones, resultado de los builds). Si no existe, lo crea. |

Los pasos, en orden:

1. Si `flake.lock` tiene cambios sin commitear, no arranca.
2. Copia el árbol de trabajo a un directorio temporal y ejecuta allí `nix flake update --flake <copia> [input...]`.
3. Si no ha cambiado nada, imprime `flake.lock is already up to date.` y para.
4. Para cada input directo que se ha movido, imprime el commit antiguo y el nuevo y los commits de upstream. Los inputs que no son directos se cuentan en una línea.
5. Salvo con `--no-build`, ejecuta `nh os build <copia> -H <host>` para cada host. El primer fallo para la ejecución y el repositorio se queda como estaba.
6. Si has pasado `--report`, añade el informe.
7. Pregunta `Write this flake.lock to the repo? [y/N]`, salvo con `-y`. Después copia el nuevo `flake.lock` al repositorio.

Códigos de salida:

| Código | Significado |
|---|---|
| 0 | Se ha escrito el lock, has contestado que no o no había nada que actualizar. |
| 1 | `flake.lock` tiene cambios sin commitear. Haz commit o stash y vuelve a ejecutarlo. |
| 2 | Algo ha fallado: `nix flake update`, el listado de hosts, un build o la escritura del archivo. |

## pkg

Actualiza los paquetes que tu flake define en su propia salida `packages.<system>`, de uno en uno, después de enseñarte qué ha cambiado upstream.

```bash
nixup pkg
nixup pkg ripgrep
nixup pkg ripgrep --no-build
nixup pkg -y --report report.md
```

Los nombres son nombres de atributo de los paquetes; sin nombres se miran todos. Un nombre que no existe es un error.

| Flag | Descripción |
|---|---|
| `-C <dir>` | Directorio del flake. |
| `--no-build` | No ejecuta `nix build` después de `nix-update`. |
| `-y` | Actualiza sin preguntar. Necesario en ejecuciones desatendidas. |
| `--report <archivo>` | Añade al final de este archivo un informe en Markdown por cada paquete actualizado. Si no existe, lo crea. |

Para cada paquete desactualizado, nixup:

1. Imprime la versión actual y la nueva, el aviso de salto mayor si lo hay y la revisión de upstream.
2. Pregunta `Update <name> to <version>? [y/N]`, salvo con `-y`. Si dices que no, imprime `Skipped.` y sigue con el siguiente.
3. Si `packages/<name>` tiene cambios sin commitear, rechaza este paquete y sigue con el siguiente.
4. Ejecuta `nix-update --flake <name> --version=<la versión revisada>`, más los argumentos extra del `passthru.updateScript` del paquete.
5. Salvo con `--no-build`, ejecuta `nix build --no-link .#<name>`.
6. Si falla el paso 4 o el 5, restaura `packages/<name>` con `git checkout`. Si no, imprime `git diff -- packages/<name>`.

Los paquetes al día solo se listan si los nombras. Las entradas `info` y `ERROR` se listan siempre. Si no nombras ningún paquete y todo está al día, `pkg` no imprime nada.

Códigos de salida. Con varios paquetes, gana el código más alto:

| Código | Significado |
|---|---|
| 0 | Todos los paquetes se han actualizado, saltado o ya estaban al día. |
| 1 | El directorio de algún paquete tenía cambios sin commitear. |
| 2 | Ha fallado una comprobación, un nombre no existe, o ha fallado `nix-update` o el build. |

## list

Lista todo lo que vigila nixup, con la versión que tienes ahora. Lee `flake.lock`, evalúa tus paquetes y pregunta a `nix profile` y a `flatpak` qué hay instalado. No sale nada a la red, así que es rápido; `check --all` da la misma lista con las versiones de upstream.

```bash
nixup list
nixup list --json
```

| Flag | Descripción |
|---|---|
| `-C <dir>` | Directorio del flake. |
| `--json` | Un array JSON de fuentes, cada una con `source`, `items` (`name`, `version`, `note`) y, si ha fallado, `error`. |

Los inputs muestran el tag al que están fijados, o el commit bloqueado y su antigüedad; la nota dice de dónde vienen (`github:owner/repo/ref`). Los paquetes muestran su upstream de GitHub. Sale con 0, o con 2 si ha fallado alguna fuente.

## search

Busca un nombre en todos los sitios de los que puede venir un paquete, y marca con un `●` verde lo que está en el sistema en marcha.

```bash
nixup search ripgrep
nixup search obsidian --limit 20
nixup search hex --json
```

| Fuente | Dónde busca |
|---|---|
| `nixpkgs <rev>` | La revisión de nixpkgs que fija tu `flake.lock`, así que las versiones son las que instalarías hoy. Necesita un input llamado `nixpkgs`. |
| `packages (yours)` | La salida `packages.<system>` de tu propio flake. |
| `input <name>` | La salida `packages.<system>` de cada input. |
| `flatpak` | Todos los remotos de Flatpak que tengas configurados, normalmente Flathub. |

La búsqueda compara con los nombres de los paquetes (nombres de atributo, IDs y nombres de Flatpak), no con las descripciones, sin distinguir mayúsculas. Primero salen las coincidencias exactas, luego los nombres que empiezan así y luego el resto. Puede tener letras, dígitos y `. _ + -`.

| Flag | Descripción |
|---|---|
| `-C <dir>` | Directorio del flake. |
| `--limit <n>` | Resultados por fuente (10 por defecto). Se indica cuántos se han quedado fuera. |
| `--json` | El mismo formato que `list --json`, más `more` (resultados que no se muestran) e `in_system`. |

`●` significa que en `/run/current-system` hay una ruta del store con el mismo nombre y versión. En una máquina sin él (CI, un contenedor) no se marca nada.

| Código | Significado |
|---|---|
| 0 | Ha encontrado algo. |
| 1 | No ha encontrado nada. |
| 2 | La búsqueda no es un nombre de paquete, o no se ha podido evaluar el flake. |

## version y help

```bash
nixup version      # también: nixup --version, nixup -v
nixup help         # también: nixup --help, nixup -h
```

`version` imprime `nixup <versión> (<commit>)` en los builds hechos desde el flake, y `nixup dev` con un `go build` normal. `help` imprime la lista de comandos por stdout. Los dos salen con 0.

## Errores en la línea de comandos

Un comando desconocido imprime `nixup: unknown command "<nombre>"` y el uso por stderr, y sale con 2. Un flag desconocido imprime los flags del comando y sale con 2. Si no has escrito ningún comando, nixup además avisa de que ha supuesto `check`:

```text
$ nixup --bogus
flag provided but not defined: -bogus
Usage of check:
  ...
nixup: no command given, 'check' was assumed (see 'nixup help' for the others)
```

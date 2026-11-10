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
nixup update [name...] [flags]
nixup list [flags]
nixup search <name> [flags]
nixup version
nixup help
```

Sin comando, nixup ejecuta `check`, así que `nixup`, `nixup --review` y `nixup check --review` hacen lo mismo.

Los flags pueden ir antes, entre o después de los nombres: `nixup update nixpkgs -y home-manager` actualiza dos inputs sin preguntar. Los flags admiten uno o dos guiones (`-y`, `--review`, `--source=input`).

Todos los comandos que trabajan sobre un flake aceptan `-C <dir>`. En [Configuración](configuration.md) se explica cómo se encuentra el flake si no pasas `-C`.

`nixup <comando> -h` imprime los flags de ese comando por stderr y sale con 0.

## Salidas

- Los resultados, las revisiones y las preguntas van a stdout.
- Los errores van a stderr.
- La salida de `nix flake update`, `nh os build`, `nix-update`, `nix build`, `flatpak update` y `nix profile upgrade` se vuelca en directo a stderr.

Así, `nixup check --json > out.json` te da un JSON limpio.

## check

Lista lo que está desactualizado. Solo lee.

```bash
nixup check
nixup check --review
nixup check --source input,package --all
nixup check --json
nixup check --markdown --review
```

| Flag | Descripción |
|---|---|
| `-C <dir>` | Directorio del flake. |
| `--source <lista>` | Orígenes a comprobar, separados por comas: `input`, `package`, `flatpak`, `profile` (también en plural). Por defecto, todos. Un nombre desconocido es un error. |
| `--all` | Lista también las entradas al día en la salida de texto. Sin él solo se cuentan. |
| `--review` | Añade la revisión de upstream (commits, enlace de comparación, release notes) a cada entrada desactualizada. |
| `--json` | Imprime todas las filas en JSON, incluidas las que están al día. Tiene prioridad sobre `--markdown`. |
| `--markdown` | Imprime solo las entradas desactualizadas o con error, en Markdown, para el cuerpo de una pull request o un resumen de CI. Si no hay nada, imprime `Everything is up to date.` |

`check` no recibe nombres; lo que escribas detrás se ignora.

La salida de texto es una tabla: `SOURCE`, `NAME`, `CURRENT`, `LATEST`, `STATUS` y `ACTION`, seguida de una línea de resumen y, con `--review`, un bloque de cambios de upstream por cada actualización. `STATUS` es `update`, `major` o `0.x-minor` (probablemente incompatible), `ok`, `info` o `error`; ver [Primeros pasos](getting-started.md#cómo-leer-el-informe). En JSON los estados son `outdated`, `ok`, `info` y `error`, y un salto probablemente incompatible lleva `warning`:

```json
[
  {
    "section": "packages",
    "name": "ripgrep",
    "current": "14.0.0",
    "latest": "15.2.0",
    "status": "outdated",
    "note": "nixup update ripgrep",
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

## update

Revisa y aplica las actualizaciones en cuatro pasos, en este orden: los inputs del flake (`flake.lock`), los paquetes que tu flake define en su propia salida `packages.<system>`, Flatpak y `nix profile`.

Los pasos de inputs y de paquetes solo editan archivos de tu repositorio; los aplicas al sistema con nh. Los pasos de Flatpak y de `nix profile` instalan en la máquina en el momento, tras una pregunta cada uno, y te dicen cómo deshacerlo.

```bash
nixup update
nixup update nixpkgs home-manager
nixup update ripgrep --no-build
nixup update co.hyprlab.Hylki
nixup update --source input -H desktop -H laptop
nixup update --source package -y --report report.md
nixup update --source flatpak
```

Sin nombres se actualiza todo: todos los inputs, todos los paquetes, todas las actualizaciones de Flatpak pendientes y todas las entradas de `nix profile` que siguen una referencia móvil. Cada nombre se busca como input directo de `flake.lock`, como paquete del flake (solo si el paso de paquetes está activo), como ID de una app de Flatpak instalada y como entrada de `nix profile`. Cada nombre va al paso de su origen; un paso sin nombres de su tipo no se ejecuta, así que `nixup update ripgrep` no toca `flake.lock` y `nixup update co.hyprlab.Hylki` solo ejecuta el paso de Flatpak. Los inputs van siempre primero, para que el build de los paquetes se haga con el lock nuevo.

Un nombre que no lleva a un único paso es un error, y no se ejecuta nada:

- Si está en más de un origen activo: `<name> is both an input and a package: pick one with --source input or --source package`. Con tres orígenes o más: `<name> is an input, a package and a Flatpak app: pick one with --source input, --source package or --source flatpak`.
- Si solo está en orígenes que `--source` deja fuera: `<name> is an input, which --source leaves out`.
- Si no está en ninguno: `no input, package, Flatpak app or nix profile entry called <name>`. El mensaje solo nombra los orígenes activos, así que con `--source input` es `no input called <name>`, y los paquetes ni se evalúan.

| Flag | Descripción |
|---|---|
| `-C <dir>` | Directorio del flake. |
| `--source <lista>` | Pasos a ejecutar, separados por comas: `input`, `package`, `flatpak`, `profile` (también en plural). Por defecto, todos. Cualquier otro valor es un error. |
| `-H <host>` | Host del que hacer el build en el paso de inputs. Se puede repetir. Por defecto, todas las entradas de `nixosConfigurations`. |
| `--no-build` | Actualiza y enseña los cambios, pero no hace el build: ni `nh os build` para los inputs ni `nix build` para los paquetes. Los pasos de Flatpak y de `nix profile` no construyen nada en ningún caso. |
| `-y` | Escribe el nuevo lock, actualiza los paquetes, instala las actualizaciones de Flatpak y actualiza las entradas de `nix profile` sin preguntar. Necesario en ejecuciones desatendidas. |
| `--report <archivo>` | Añade al final de este archivo un informe en Markdown (inputs movidos, revisiones, resultado de los builds, un bloque por paquete actualizado, la tabla de Flatpak con los comandos para volver atrás, las entradas de `nix profile` actualizadas). Si no existe, lo crea. |

### Inputs

El paso de inputs, en orden:

1. Si `flake.lock` tiene cambios sin commitear, no arranca. Los demás pasos se ejecutan igualmente.
2. Copia el árbol de trabajo a un directorio temporal y ejecuta allí `nix flake update --flake <copia> [input...]`.
3. Si no ha cambiado nada, imprime `flake.lock is already up to date.` y para.
4. Para cada input directo que se ha movido, imprime el commit antiguo y el nuevo y los commits de upstream. Los inputs que no son directos se cuentan en una línea.
5. Salvo con `--no-build`, ejecuta `nh os build <copia> -H <host>` para cada host. El primer fallo para el paso y el repositorio se queda como estaba.
6. Si has pasado `--report`, añade el informe.
7. Pregunta `Write this flake.lock to the repo? [y/N]`, salvo con `-y`. Después copia el nuevo `flake.lock` al repositorio.

### Paquetes

El paso de paquetes los recorre de uno en uno. Para cada paquete desactualizado, nixup:

1. Imprime la versión actual y la nueva, el aviso de salto mayor si lo hay y la revisión de upstream.
2. Pregunta `Update <name> to <version>? [y/N]`, salvo con `-y`. Si dices que no, imprime `Skipped.` y sigue con el siguiente.
3. Si `<dir del paquete>` tiene cambios sin commitear, rechaza este paquete y sigue con el siguiente.
4. Ejecuta `nix-update --flake <name> --version=<la versión revisada>`, más los argumentos extra del `passthru.updateScript` del paquete.
5. Salvo con `--no-build`, ejecuta `nix build --no-link .#<name>`.
6. Si falla el paso 4 o el 5, restaura `<dir del paquete>` con `git checkout`. Si no, imprime `git diff -- <dir del paquete>`.

Los paquetes al día solo se listan si los nombras. Las entradas `info` y `ERROR` se listan siempre. Si no nombras ningún paquete y ninguno está desactualizado, el paso imprime `Every package is up to date.`

### Flatpak

El paso de Flatpak solo se ejecuta si tienes `flatpak` instalado. Si no, se salta sin decir nada. En orden:

1. Lista las actualizaciones pendientes con `flatpak remote-ls --updates` y las versiones instaladas con `flatpak list`, los mismos datos que usa `check`. Con nombres, se queda solo con esos IDs; una app nombrada sin actualización imprime `<id> is up to date.` Sin nombres y sin nada pendiente, imprime `Every Flatpak is up to date.` y para.
2. Imprime una tabla: `SOURCE`, `NAME`, `CURRENT`, `LATEST`. `LATEST` es la versión nueva, `unknown` si el remoto no publica ninguna, o `new build` para los runtimes y las recompilaciones de la misma versión.
3. Lee el commit desplegado de cada ref con `flatpak info --show-commit <ref>`.
4. Pregunta una sola vez, `Update 3 Flatpak refs now? This installs them right away. [y/N]` (`Update 1 Flatpak ref now? This installs it right away.` si es uno), salvo con `-y`. Si dices que no, imprime `Skipped.`
5. Ejecuta `flatpak update -y --noninteractive` con exactamente esos refs.
6. Imprime `Flatpak updated.`, o `flatpak update failed; some refs may have been updated.` y sale con 2. En los dos casos imprime una línea por ref para volver atrás: `flatpak update --commit=<commit anterior> <ref>`.

Los nombres son IDs de Flatpak de apps o de runtimes, como en `nixup update co.hyprlab.Hylki` o `nixup update org.gnome.Platform`, los mismos que enseña `check`. Una app instalada desde un bundle cuyo remoto está desactivado, como un bundle `.flatpak` que construye uno de tus paquetes, nunca tiene actualizaciones en Flatpak. Se actualiza a través de su paquete.

### nix profile

El paso de `nix profile`, en orden:

1. Lee `nix profile list --json`. Con nombres, se queda solo con esas entradas.
2. Deja fuera las entradas fijadas a una revisión (`github:NixOS/nixpkgs/<40 caracteres hex>`), porque `nix profile upgrade` no puede moverlas. Para cada una imprime `<name> is pinned to <short rev>: nix profile upgrade cannot move it. To update it, nix profile remove <name> and install it again.`
3. Si no queda ninguna entrada que siga una referencia móvil (como `flake:nixpkgs`, o `github:owner/repo` sin revisión), para. Sin nombres, antes imprime `No nix profile entry follows a moving reference: nothing to upgrade.`
4. Pregunta `Upgrade 2 nix profile entries (a, b) now? This installs them right away. [y/N]`, salvo con `-y`. Si dices que no, imprime `Skipped.`
5. Ejecuta `nix profile upgrade` con los nombres de esas entradas.
6. Imprime `nix profile upgraded. To undo it: nix profile rollback`. Si falla, imprime `nix profile upgrade failed.` y sale con 2.

Los nombres son los de las entradas de `nix profile`, tal y como los muestra `nix profile list`.

Códigos de salida. Gana el peor código de los cuatro pasos y de todos los paquetes:

| Código | Significado |
|---|---|
| 0 | Se ha actualizado todo, has contestado que no o no había nada que actualizar. |
| 1 | Se ha rechazado algo por cambios sin commitear: `flake.lock` o el directorio de un paquete. Haz commit o stash y vuelve a ejecutarlo. |
| 2 | Algo ha fallado: un nombre o `--source` estaban mal, ha fallado una comprobación, o han fallado `nix flake update`, el listado de hosts, un build, `nix-update`, la escritura del archivo, `flatpak update` o `nix profile upgrade`. |

## list

Lista todo lo que vigila nixup, con la versión que tienes ahora. Lee `flake.lock`, evalúa tus paquetes y pregunta a `nix profile` y a `flatpak` qué hay instalado. No sale nada a la red, así que es rápido; `check --all` da la misma lista con las versiones de upstream.

```bash
nixup list
nixup list --json
```

| Flag | Descripción |
|---|---|
| `-C <dir>` | Directorio del flake. |
| `--source <lista>` | Orígenes separados por comas: `input`, `package`, `flatpak`, `profile`. Por defecto, todos. |
| `--json` | Un array JSON de fuentes, cada una con `source`, `items` (`name`, `version`, `note`, el origen) y, si ha fallado, `error`. |

La salida es una tabla, `SOURCE`, `NAME`, `VERSION` y `ORIGIN`, y un recuento por origen:

```text
SOURCE   NAME                  VERSION        ORIGIN
input    home-manager          4839ed6 · 20d  github:nix-community/home-manager
input    nixpkgs               0bb2eed · 8d   github:NixOS/nixpkgs/nixos-unstable
package  ripgrep               14.0.0         github:BurntSushi/ripgrep
flatpak  md.obsidian.Obsidian  1.15.0         flathub

2 inputs · 1 package · 1 flatpak
```

Los inputs muestran el tag al que están fijados, o el commit bloqueado y su antigüedad. Los orígenes usan la sintaxis de los flakes (`github:owner/repo/ref`); en una entrada de `nix profile` es su flake sin la revisión fijada, y en una app de Flatpak, su remoto. Sale con 0, o con 2 si ha fallado alguna fuente.

## search

Busca un nombre en todos los sitios de los que puede venir un paquete, y marca en `INSTALLED` con un `●` verde lo que está en el sistema en marcha.

```bash
nixup search ripgrep
nixup search obsidian --limit 20
nixup search hex --json
```

| `SOURCE` | Dónde busca |
|---|---|
| `nixpkgs` | La revisión de nixpkgs que fija tu `flake.lock`, así que las versiones son las que instalarías hoy. La revisión aparece en la línea de resumen. Necesita un input llamado `nixpkgs`. |
| `package` | La salida `packages.<system>` de tu propio flake. |
| `input:<nombre>` | La salida `packages.<system>` de cada input. |
| `flatpak` | Todos los remotos de Flatpak que tengas configurados, normalmente Flathub. |

```text
SOURCE   NAME                      VERSION  INSTALLED  DESCRIPTION
nixpkgs  ripgrep                   15.2.0   ●          Utility that combines the usability of The Silver Searcher with the raw…
nixpkgs  ripgrep-all               0.10.10             Ripgrep, but also search in PDFs, E-Books, Office documents, zip, tar.g…
flatpak  de.leopoldluley.Clapgrep  26.09               Search through all your files

3 results (nixpkgs 0bb2eed)
```

La búsqueda compara con los nombres de los paquetes (nombres de atributo, IDs y nombres de Flatpak), no con las descripciones, sin distinguir mayúsculas. Primero salen las coincidencias exactas, luego los nombres que empiezan así y luego el resto. Puede tener letras, dígitos y `. _ + -`.

| Flag | Descripción |
|---|---|
| `-C <dir>` | Directorio del flake. |
| `--limit <n>` | Resultados por fuente (10 por defecto). Se indica cuántos se han quedado fuera. |
| `--source <lista>` | Orígenes separados por comas: `nixpkgs`, `package`, `input`, `flatpak`. `input` abarca todos los inputs. Los orígenes que no pides ni se consultan. |
| `--json` | El mismo formato que `list --json` (`note` es la descripción), más `rev` para nixpkgs, `more` (resultados que no se muestran) e `in_system`. |

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

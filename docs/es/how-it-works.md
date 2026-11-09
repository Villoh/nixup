---
title: Cómo funciona
description: Qué comprueba cada sección de nixup, cómo revisa los cambios de upstream, la copia temporal y qué se escribe en disco.
lang: es
---

# Cómo funciona

[English](../how-it-works.md) | **Español**

## Las secciones de check

`nixup check` ejecuta sus cuatro secciones a la vez, con hasta seis consultas en paralelo por sección. Ninguna escribe en tu repositorio.

### inputs

nixup lee `flake.lock` y mira los inputs directos de tu flake cuyo tipo original es `github`. Se saltan los inputs que solo hacen `follows` de otro, y también los de cualquier otro tipo.

- **Inputs fijados a un tag de release.** Si el `ref` del input parece una versión (`v1.6.2`, `1.2.0`, `dms/1.11.6`), nixup lista los tags del repositorio con `git ls-remote --tags` y se queda con el más alto que tenga el mismo prefijo. Las prereleases solo cuentan si el tag actual ya es una prerelease. `nix flake update` nunca mueve un tag, así que la nota dice `pinned tag: edit flake.nix`.
- **Inputs de rama.** Cualquier otro `ref`, o ninguno, es una rama. nixup pide a `git ls-remote` el último commit de esa rama (o `HEAD` si no hay `ref`) y lo compara con el commit bloqueado. La columna actual muestra el commit bloqueado y su antigüedad en días.

### packages

nixup evalúa `.#packages.<system>` para la máquina en la que se ejecuta y lee de cada paquete `version`, `src.urls`, `meta.homepage` y `passthru.updateScript`.

- El repositorio de origen es el primer `github.com/<owner>/<repo>` que aparezca en las URLs del código fuente y, si no, en la homepage. Si no hay ninguno, el paquete sale como `info` con `no GitHub upstream: check by hand`.
- Sin `passthru.updateScript`, el paquete sale como `info` con `no passthru.updateScript`, porque `nixup pkg` no podría actualizarlo.
- Si el script de actualización pasa `--use-github-releases`, nixup compara con las releases publicadas en GitHub, sin contar los borradores. Si no, compara con los tags. La idea es elegir la misma versión que elegiría `nix-update`.
- Si el script de actualización pasa `--version-regex`, nixup lee las versiones igual que nix-update: solo cuentan los tags en los que la regex encaja desde el principio, y la versión son sus grupos no vacíos unidos con `.`. Así se sigue un repositorio con varias series de tags (`desktop-v0.6.1` junto a `desktop-minimal-v0.6.1`). Sin ella, solo cuentan los tags que empiezan por un dígito o por `v` más un dígito.
- Las prereleases solo cuentan si el script de actualización pasa `--version=unstable` o si la versión actual ya es una prerelease.
- Si el flake tiene un input llamado `nixpkgs`, nixup busca también cada paquete en ese nixpkgs fijado, por su nombre y, en el caso de `foo-bin`, como `foo`. Si nixpkgs tiene el mismo proyecto (mismo repositorio de GitHub o misma homepage) con la misma versión o una más nueva, la fila lleva una línea de pista, `→ nixpkgs has foo 1.2.0: pkgs.foo could replace this package`, y una fila al día sale como `info` en vez de ocultarse. La pista nunca cambia el código de salida.

Un flake sin salida `packages` no tiene nada en esta sección.

### profile

Si tienes `nix` instalado, nixup lee `nix profile list --json`. Para cada entrada cuya URL original contiene una revisión de nixpkgs de 40 caracteres, compara esa revisión con la que tiene bloqueada el input `nixpkgs` de tu flake. Si es distinta, sale como `pinned to an old nixpkgs`. Las entradas que siguen una referencia de flake en vez de una revisión fija salen como `ok`, con `nix profile upgrade <name>` como nota.

### flatpak

Si tienes `flatpak` instalado, nixup ejecuta `flatpak remote-ls --updates` y lista una vez cada aplicación o runtime, con la versión instalada (sacada de `flatpak list`) y la nueva. Los runtimes no tienen versión, así que se muestra su rama; una actualización que mantiene la misma versión sale como `new build`. Solo informa; el `flatpak update` lo haces tú.

## Versiones

Los tags y las versiones se comparan por sus números, ignorando una `v` inicial o un prefijo como `dms/`. Una prerelease va por debajo de su versión final. Palabras como `alpha`, `beta`, `rc`, `pre`, `preview`, `dev`, `nightly` y `snapshot` marcan una prerelease cuando empiezan un segmento, así que `1.2.3-rc.1` es una prerelease y `1.2.3-arch` no.

Un salto lleva aviso cuando cambia el primer número (de `1.x` a `2.x`), o cuando cambia el segundo mientras el primero es `0` (de `0.21` a `0.22`).

## La revisión de upstream

Con `check --review`, y siempre en `lock` y `pkg`, nixup monta una revisión para cada actualización pendiente:

- `source`: el repositorio de GitHub.
- `changes`: el número de commits entre las dos versiones, sacado de la API de comparación de GitHub, y un enlace de comparación.
- Hasta ocho mensajes de commit, del más nuevo al más antiguo.
- Para los paquetes, y para los inputs fijados en `check --review`, las release notes publicadas entre las dos versiones. Los borradores se saltan. Las prereleases se saltan y se cuentan. En la terminal salen las primeras líneas de hasta tres releases; el resto está en el enlace de comparación.

Los inputs de rama, y todos los inputs en `lock`, solo llevan commits, sin release notes. Si la API de GitHub falla, la revisión dice `review unavailable` con el motivo, y la actualización puede seguir adelante.

## lock, paso a paso

1. Si `git diff --quiet -- flake.lock` detecta cambios, para con código de salida 1.
2. Crea un directorio temporal y copia en él todos los archivos que lista `git ls-files --cached --others --exclude-standard`: los archivos versionados con sus cambios sin commitear, más los archivos sin versionar que no están ignorados. Se saltan los enlaces simbólicos y los archivos borrados del árbol de trabajo. Esta es la copia temporal.
3. Ejecuta `nix flake update --flake <copia>`, con los nombres de input que hayas dado.
4. Compara el lock antiguo con el nuevo. Para cada input directo cuyo commit se ha movido, imprime los commits de upstream. Cuenta los demás nodos que se han movido.
5. Salvo con `--no-build`, coge los hosts de `-H`, o los lista con `nix eval <copia>#nixosConfigurations --apply builtins.attrNames`.
6. Salvo con `--no-build`, hace el build de cada host con `nh os build <copia> -H <host> -o <copia>/result`. No se activa nada. El primer build que falla para la ejecución con código de salida 2 y tu `flake.lock` se queda como estaba.
7. Si has pasado `--report`, añade el informe en Markdown. Esto pasa antes de la pregunta, así que el informe se escribe aunque contestes que no.
8. Pregunta, salvo con `-y`. Si dices que sí, escribe el nuevo `flake.lock` en tu repositorio.
9. Borra la copia temporal.

Como la copia incluye los archivos sin commitear y sin versionar, el build prueba tu árbol de trabajo tal y como está ahora, no el último commit.

## pkg, paso a paso

Para cada paquete, en orden alfabético o en el orden en que los hayas nombrado. `<dir del paquete>` es el directorio del archivo que define el paquete, leído de su `meta.position` igual que hace `nix-update` (`packages/prebuilt/kopuz` para un paquete en `packages/prebuilt/kopuz/package.nix`); si el paquete se define en un archivo en la raíz del flake, como `flake.nix`, es solo ese archivo, nunca el repositorio entero. Si el archivo no es parte de este flake, el paquete se rechaza antes de ejecutar nada.

1. Hace la misma comprobación que `nixup check` para ese paquete, con la revisión.
2. Si está desactualizado, imprime la revisión y pregunta, salvo con `-y`.
3. Si `git diff --quiet -- <dir del paquete>` detecta cambios, rechaza este paquete con código de salida 1 y sigue.
4. Ejecuta `nix-update --flake <name> --version=<versión revisada>` más los argumentos extra del script de actualización. Si `nix-update` no está en el `PATH`, usa `nix run nixpkgs#nix-update --`.
5. Ejecuta `nix build --no-link .#<name>`, salvo con `--no-build`.
6. Si falla el paso 4 o el 5, ejecuta `git checkout -- <dir del paquete>`. Si todo va bien, imprime `git diff -- <dir del paquete>`.

Aquí `nix-update` edita tu repositorio directamente, no una copia. El `git checkout` del paso 6 es lo que deja los archivos como estaban.

## Comandos que ejecuta nixup

Esta es la lista completa. Todo pasa por un único paquete (`internal/run`), así que no se lanza nada más.

| Comando | Dónde |
|---|---|
| `git ls-remote --tags --refs https://github.com/<repo>` | inputs, packages, revisiones |
| `git ls-remote https://github.com/<repo> <ref>` | inputs de rama |
| `git ls-files -z --cached --others --exclude-standard` | lock, copia temporal |
| `git diff --quiet -- <path>` | lock, pkg |
| `git checkout -- <dir del paquete>` | pkg, tras un fallo |
| `git diff -- <dir del paquete>` | pkg, tras un éxito |
| `nix eval --json .#packages.<system> --apply ...` | packages, pkg |
| `nix eval --json --inputs-from . nixpkgs#legacyPackages.<system> --apply ...` | packages, si el flake tiene un input `nixpkgs` |
| `nix eval --json <copy>#nixosConfigurations --apply builtins.attrNames` | lock |
| `nix flake update --flake <copy> [input...]` | lock |
| `nh os build <copy> -H <host> -o <copy>/result` | lock |
| `nix-update --flake <name> --version=<v> [args...]` | pkg |
| `nix build --no-link .#<name>` | pkg |
| `nix profile list --json` | profile |
| `nix eval --json --impure --expr <getFlake del flake>` | search |
| `nix-store --query --requisites /run/current-system` | search (qué hay en el sistema) |
| `flatpak list --app --columns=...` | list, search |
| `flatpak search --columns=... <nombre>` | search |
| `flatpak remote-ls --updates --columns=application,version,branch` | flatpak |
| `flatpak list --columns=application,version,branch` | flatpak (installed versions) |
| `gh auth token` | token de GitHub, si no hay ninguna variable definida |

Las peticiones HTTP propias de nixup solo van a `api.github.com`. `git`, `nix` y `nix-update` salen a la red por su cuenta.

## Qué se escribe y qué nunca

Se escribe:

- Con `lock`: tu `flake.lock`, tras un sí o con `-y`. Una copia temporal, que se borra al terminar.
- Con `pkg`: los archivos que cambia `nix-update`, normalmente en `<dir del paquete>`. Se restauran si falla la actualización o el build.
- Con `--report <archivo>`: Markdown añadido al final de ese archivo.
- Los builds acaban en el Nix store, como cualquier build.

Nunca:

- `git add`, `git commit` ni `git push`.
- `nixos-rebuild`, `nh os switch`, `nh os boot`, `nh os test` ni nada que active una generación.
- `sudo`.
- Un demonio, un timer o un proceso en segundo plano.

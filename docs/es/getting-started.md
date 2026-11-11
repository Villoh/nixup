---
title: Primeros pasos
description: Instala nixup, apúntalo a tu flake de NixOS y lee el primer informe.
lang: es
---

# Primeros pasos

[English](../getting-started.md) | **Español**

## Requisitos

- Una configuración de NixOS en un flake, dentro de un repositorio git, con `flake.nix` y `flake.lock` commiteados.
- `git` y `nix` en el `PATH`.
- `nh` si quieres que `nixup update` haga el build de tus hosts, o que active uno con `--test`, `--switch` o `--boot`. Con `--no-build` y sin ninguno de esos tres no hace falta.
- `nix-update` para actualizar tus propios paquetes con `nixup update`. Si no lo tienes, nixup ejecuta `nix run nixpkgs#nix-update --` en su lugar.
- `flatpak` es opcional. Sin él, se saltan la sección `flatpak` y el paso de Flatpak de `nixup update`.
- Acceso de red a github.com y api.github.com.

El flake trae builds para `x86_64-linux` y `aarch64-linux`. En [Configuración](configuration.md) tienes la lista completa de herramientas y para qué se usa cada una.

## Instalación

Ejecútalo una vez sin instalar nada:

```bash
nix run github:Villoh/nixup -- check
```

Para instalarlo en tu sistema, añádelo como input del flake:

```nix
{
  inputs.nixup = {
    url = "github:Villoh/nixup";
    inputs.nixpkgs.follows = "nixpkgs";
  };
}
```

Luego añade el paquete en un módulo de NixOS. Esto da por hecho que pasas `inputs` a tus módulos con `specialArgs`:

```nix
{ inputs, pkgs, ... }:
{
  environment.systemPackages = [
    inputs.nixup.packages.${pkgs.stdenv.hostPlatform.system}.default
  ];
}
```

El flake también tiene un overlay que añade `pkgs.nixup`:

```nix
{ inputs, pkgs, ... }:
{
  nixpkgs.overlays = [ inputs.nixup.overlays.default ];
  environment.systemPackages = [ pkgs.nixup ];
}
```

El paquete también instala el autocompletado para bash, zsh y fish: en una shell nueva, `nixup update <TAB>` te ofrece tus inputs, paquetes y apps de Flatpak. Ver [completion](commands.md#completion).

Comprueba que se ha instalado:

```bash
nixup version
```

## Primera ejecución

Ve a tu flake y ejecuta `nixup` sin argumentos. Es lo mismo que `nixup check`:

```bash
cd ~/nixos-config
nixup
```

No hace falta estar dentro del flake. `nixup -C ~/nixos-config` funciona desde cualquier sitio, y si `nh` ya conoce tu flake por `NH_FLAKE` o `NH_OS_FLAKE`, nixup usa el mismo. Lo tienes en [Configuración](configuration.md).

## Cómo leer el informe

```text
SOURCE   NAME          CURRENT        LATEST   STATUS
input    home-manager  4839ed6 · 20d  2c0c1f0  update
input    lanzaboote    v0.4.1         v1.2.0   major
input    nixpkgs       0bb2eed · 8d   f4a03c0  update
package  ripgrep       14.0.0         15.2.0   major

4 to update (2 likely breaking) · 1 up to date (--all to show them)
```

Una fila por entrada: de dónde viene (`SOURCE`: `input`, `package`, `flatpak`, `profile`), su nombre, la versión actual y la más nueva, y un estado. Toda actualización se aplica con `nixup update <nombre>` (o `nixup update` para todo), así que la tabla no lo repite; la columna `NOTE` solo aparece en las filas que necesitan otra cosa. La última línea lo resume todo.

`STATUS` puede ser:

- `update`: hay una versión más nueva.
- `major` o `0.x-minor`: hay una versión más nueva y el salto pinta a cambio incompatible, una versión mayor nueva o una versión menor nueva cuando todavía estás en `0.x`. Lee primero el changelog.
- `ok`: está al día. Estas filas solo se cuentan; con `--all` se listan.
- `info`: nixup no puede comprobar esta entrada, y `NOTE` dice por qué (por ejemplo `no GitHub upstream: check by hand`).
- `error`: la comprobación ha fallado, y `NOTE` trae el motivo.

En los inputs de rama, `CURRENT` muestra el commit bloqueado y cuántos días tiene: `4839ed6 · 20d` se hizo upstream hace 20 días.

## Lee qué ha cambiado

```bash
nixup check --review
```

Después de la tabla, para cada actualización, añade el repositorio de origen, el número de commits con el tamaño del cambio y un enlace de comparación, los últimos mensajes de commit y, si las versiones son tags, las release notes entre medias:

```text
ripgrep package  14.0.0 → 15.2.0
  source   https://github.com/BurntSushi/ripgrep
  changes  45 commits · 38 files (+1,204 −311)  https://github.com/BurntSushi/ripgrep/compare/14.0.0...15.2.0
    · release: 15.2.0
    · changelog: prepare for 15.2.0
  notes    15.2.0
           ripgrep 15.2.0 is a minor release with bug fixes.
```

GitHub lista como mucho 300 archivos cambiados, así que un cambio más grande dice `300+ files` sin contar líneas.

## Aplica las actualizaciones

```bash
nixup update                  # inputs, tus paquetes, Flatpak, nix profile
nixup update --source input   # solo los inputs
nixup update home-manager     # un solo input
nixup update ripgrep          # uno de tus paquetes
nixup update co.hyprlab.Hylki # una app de Flatpak
```

Con los inputs, `update` te enseña los commits de upstream de cada input que se ha movido, hace el build de cada host desde una copia temporal y pregunta antes de escribir `flake.lock`. Después pregunta antes de cada uno de tus paquetes. Estos dos pasos solo editan archivos de tu repositorio, y nixup no hace commit. Cada paquete se comprueba compilando solo el paquete (no el sistema) y termina con `herdr-app updated to 0.1.14.`; si nix-update ha editado en tus archivos algo más que la versión y el hash, esas ediciones se listan debajo.

Al final van Flatpak y `nix profile`. Cada uno enseña qué actualizaría y pregunta una vez, y la pregunta dice `This installs them right away.` Si dices que sí, instalan y después te dicen cómo volver atrás: una línea `flatpak update --commit=<commit anterior> <ref>` por cada ref de Flatpak, o `nix profile rollback`. Usa `--source input,package` para no tocar la máquina.

Por defecto nixup no activa nada: revisa con `git diff` y aplica tú con nh. Para que nh active el resultado al final, añade `--test`, `--switch` o `--boot`:

```bash
nixup update --test           # y después nh os test en el único host
nixup update -H desktop --test
```

Después de los cuatro pasos, si se ha escrito `flake.lock` o algún paquete, nixup imprime `Activating desktop (nh os test)…` y ejecuta `nh os test <repo> -H desktop`. nh pide sudo y activa. Si no se ha escrito nada, o ha fallado algún paso, no se ejecuta nh. Un flake con varios hosts necesita `-H`. Ver [Activación](commands.md#activación).

Los tags fijados (`lanzaboote` en el ejemplo) no los mueve `nix flake update`, así que los mueve nixup. En la copia temporal cambia el tag de `flake.nix` por la release más nueva, y si dices que sí escribe `flake.nix` junto con `flake.lock`. Un `nixup update` sin nombres deja donde está un salto que pinta a cambio incompatible, como `v0.4.1` a `v1.2.0`, y lo avisa. `nixup update lanzaboote` lo mueve.

Siguiente: [Comandos](commands.md) para ver todos los flags, o [Cómo funciona](how-it-works.md) para saber qué pasa por debajo.

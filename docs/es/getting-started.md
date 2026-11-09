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
- `nh` si quieres que `nixup lock` haga el build de tus hosts. Con `--no-build` no hace falta.
- `nix-update` para `nixup pkg`. Si no lo tienes, nixup ejecuta `nix run nixpkgs#nix-update --` en su lugar.
- `flatpak` es opcional. Sin él, la sección `flatpak` se salta.
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
inputs
  UPDATE home-manager               4839ed6 (20d) -> 2c0c1f0       nixup lock home-manager
  UPDATE lanzaboote                 v0.4.1 -> v1.2.0               pinned tag: edit flake.nix
         ⚠ MAJOR version jump: read the changelog, expect breaking changes
  UPDATE nixpkgs                    0bb2eed (8d) -> f4a03c0        nixup lock nixpkgs
  1 up to date (use --all to list)

packages
  UPDATE ripgrep                    14.0.0 -> 15.2.0               nixup pkg ripgrep
         ⚠ MAJOR version jump: read the changelog, expect breaking changes
```

Cada línea tiene un estado, un nombre, la versión actual y la más nueva, y el comando que aplica la actualización.

- `UPDATE`: hay una versión más nueva.
- `ok`: está al día. Estas líneas solo se cuentan; con `--all` se listan.
- `info`: nixup no puede comprobar esta entrada, y la nota dice por qué (por ejemplo `no GitHub upstream: check by hand`).
- `ERROR`: la comprobación ha fallado, y la nota trae el motivo.

En los inputs de rama, la columna actual muestra el commit bloqueado y cuántos días tiene: `4839ed6 (20d)` se hizo upstream hace 20 días.

La línea con `⚠` sale cuando el salto pinta a cambio incompatible: una versión mayor nueva, o una versión menor nueva cuando todavía estás en `0.x`.

## Lee qué ha cambiado

```bash
nixup check --review
```

Debajo de cada línea `UPDATE` añade el repositorio de origen, el número de commits con un enlace de comparación, los últimos mensajes de commit y, si las versiones son tags, las release notes entre medias.

## Aplica las actualizaciones

```bash
nixup lock                 # todos los inputs
nixup lock home-manager    # un solo input
nixup pkg ripgrep          # uno de tus paquetes
```

`lock` te enseña los commits de upstream de cada input que se ha movido, hace el build de cada host desde una copia temporal y pregunta antes de escribir `flake.lock`. `pkg` pregunta antes de cada paquete. Ninguno hace commit ni activa nada. Cuando `lock` termina, te dice los siguientes pasos:

```text
flake.lock updated (not committed, nothing activated).
Next: git diff flake.lock && nix flake check && nh os test . -H <host>
```

Los tags fijados (`lanzaboote` en el ejemplo) no los mueve `nix flake update`. Cambia el tag en `flake.nix` a mano y después ejecuta `nixup lock lanzaboote`.

Siguiente: [Comandos](commands.md) para ver todos los flags, o [Cómo funciona](how-it-works.md) para saber qué pasa por debajo.

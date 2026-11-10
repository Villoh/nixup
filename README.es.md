<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="brand/header-dark.svg">
  <img alt="nixup" src="brand/header-light.svg" width="420">
</picture>

Actualizaciones seguras para NixOS con flakes. Primero revisas, luego decides.

[![CI](https://img.shields.io/github/actions/workflow/status/Villoh/nixup/ci.yml?branch=main&label=ci)](https://github.com/Villoh/nixup/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/Villoh/nixup?color=4A6FC4)](https://github.com/Villoh/nixup/releases/latest)
[![Go](https://img.shields.io/github/go-mod/go-version/Villoh/nixup?color=4A6FC4)](go.mod)
[![License: MIT](https://img.shields.io/github/license/Villoh/nixup?color=4A6FC4)](LICENSE)

[English](README.md) | **Español**

</div>

nixup busca actualizaciones para una configuración de NixOS basada en flakes y solo las aplica cuando ya has leído qué ha cambiado upstream. Compara con su origen los inputs del flake, los paquetes que define tu flake, las entradas de `nix profile` y las apps de Flatpak. Para cada actualización pendiente te enseña los commits y las release notes que hay entre medias, y hace el build en una copia temporal antes de escribir nada. También puede instalar las actualizaciones de Flatpak pendientes y actualizar entradas de `nix profile`, tras preguntarte. Nunca activa una generación de NixOS: ni `switch`, ni `boot`, ni `sudo`, ni demonios. Está escrito en Go, solo con la biblioteca estándar.

## Ejemplo

```text
$ nixup check --source input,package
SOURCE   NAME          CURRENT        LATEST   STATUS  ACTION
input    home-manager  4839ed6 · 20d  2c0c1f0  update  nixup update home-manager
input    lanzaboote    v0.4.1         v1.2.0   major   pinned tag: edit flake.nix
input    nixpkgs       0bb2eed · 8d   f4a03c0  update  nixup update nixpkgs
package  ripgrep       14.0.0         15.2.0   major   nixup update ripgrep

4 to update (2 likely breaking) · 1 up to date (--all to show them)
```

Con `--review` verás, después de la tabla y para cada actualización, el repositorio de origen, el número de commits con un enlace de comparación, los últimos mensajes de commit y, si la versión es un tag, las release notes entre ambas versiones.

## Instalación

Pruébalo sin instalar nada:

```bash
nix run github:Villoh/nixup -- check
```

O añádelo a tu flake y mete `inputs.nixup.packages.${system}.default` en `environment.systemPackages`:

```nix
{
  inputs.nixup = {
    url = "github:Villoh/nixup";
    inputs.nixpkgs.follows = "nixpkgs";
  };
}
```

## Primeros pasos

```bash
nixup                    # igual que `nixup check`: qué está desactualizado, solo lectura
nixup check --review     # además, commits y release notes de upstream
nixup update             # inputs: actualiza flake.lock en una copia temporal, hace el build, pregunta y escribe;
                         # después tus propios paquetes uno a uno, tras revisar cada uno;
                         # después Flatpak y nix profile, con una pregunta cada uno
nixup update nixpkgs     # solo ese input
nixup update ripgrep     # solo ese paquete
nixup update co.hyprlab.Hylki   # solo esa app de Flatpak
nixup update --source input,package   # solo lo que vive en el repositorio
nixup list               # todo lo que vigila, con las versiones actuales, sin red
nixup search ripgrep     # en tu nixpkgs, tus paquetes, tus inputs y Flatpak
```

Trabaja sobre el flake en el que estás; en cualquier otro sitio lee `NIXUP_FLAKE`, `NH_OS_FLAKE` o `NH_FLAKE` (las dos últimas son las de nh). `-C <dir>` tiene prioridad sobre ambos.

## Por qué es seguro

- `check` solo lee. No escribe nada en tu repositorio.
- `update` ejecuta `nix flake update` y los builds de los hosts en una copia de tu árbol de trabajo. Tu `flake.lock` solo cambia si el build pasa y contestas que sí.
- Con tus propios paquetes, `update` ejecuta `nix-update` fijado a la versión que acabas de revisar y luego hace el build del paquete. Si falla cualquiera de los dos pasos, los archivos del paquete vuelven a quedar como estaban, con los cambios sin commitear incluidos.
- `update` pregunta antes de actualizar sobre cambios sin commitear en `flake.lock` o en un paquete, y los conserva.
- Los pasos de inputs y de paquetes solo editan archivos del repositorio; los aplicas tú con nh. Los pasos de Flatpak y de `nix profile` instalan en el momento, así que su pregunta lo dice, y te indican cómo deshacerlo (`flatpak update --commit=...`, `nix profile rollback`). Una entrada de `nix profile` fijada a una revisión nunca se actualiza.
- Sin terminal, todas las preguntas se responden que no, así que en ejecuciones desatendidas tienes que pasar `-y`.
- nixup nunca hace commit ni activa una generación de NixOS. Probar y hacer el switch sigue siendo cosa tuya.

## Documentación

| Página | Qué cubre |
|---|---|
| [Primeros pasos](docs/es/getting-started.md) | Requisitos, formas de instalarlo, primera ejecución |
| [Comandos](docs/es/commands.md) | Todos los flags y códigos de salida |
| [Configuración](docs/es/configuration.md) | Qué flake usa, variables de entorno, token de GitHub |
| [Cómo funciona](docs/es/how-it-works.md) | Qué comprueba cada sección y qué se escribe |
| [Seguridad](docs/es/safety.md) | Las garantías y hasta dónde llegan |
| [CI](docs/es/ci.md) | Comprobaciones programadas y pull requests de actualización |
| [Limitaciones](docs/es/limitations.md) | Lo que nixup no ve |
| [Desarrollo](docs/es/development.md) | Estructura del código y tests, para quien contribuye |
| [Releases](docs/es/releases.md) | Cómo se publica una versión, para mantenedores |

Licencia MIT, ver [LICENSE](LICENSE).

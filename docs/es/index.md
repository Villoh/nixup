---
title: Documentación de nixup
description: Comprobador de actualizaciones y actualizador seguro para NixOS con flakes. Resumen y mapa de la documentación.
lang: es
---

# Documentación de nixup

[English](../index.md) | **Español**

nixup es una herramienta de línea de comandos para quien gestiona NixOS con un flake. Responde a dos preguntas: qué está desactualizado y qué ha cambiado upstream desde la versión que tienes. Después actualiza `flake.lock` y tus propios paquetes, pero solo tras un sí por tu parte. Tus paquetes se construyen antes; tus hosts, en una copia temporal cuando lo pides. También puede instalar las actualizaciones de Flatpak pendientes y actualizar entradas de `nix profile`, tras preguntarte.

No activa ninguna generación de NixOS salvo que le pases `--test`, `--switch` o `--boot` a `nixup update`, y entonces lo hace nh. En nixup no hay código de activación ni `sudo`, ni demonios ni timers. Cuando nixup escribe un archivo, tú revisas el diff, pruebas el sistema con tus herramientas de siempre y haces el commit. Los pasos de Flatpak y de `nix profile` son la excepción a "nixup solo escribe archivos": instalan en el momento y te dicen cómo deshacerlo.

## Qué comprueba

| Sección | Qué compara | Cómo se actualiza |
|---|---|---|
| `inputs` | Los inputs de rama con el último commit de la rama remota; los fijados a un tag de release con el tag más nuevo | `nixup update`, que también mueve un tag fijado en `flake.nix` |
| `packages` | Cada paquete de la salida `packages.<system>` de tu flake con su tag o release más reciente en GitHub | `nixup update` (ejecuta `nix-update`) |
| `profile` | Entradas de `nix profile` fijadas a una revisión de nixpkgs distinta de la del input `nixpkgs` de tu flake; las fijadas a otro repositorio solo se listan | `nixup update` para las entradas que siguen una referencia móvil; una entrada fijada necesita `nix profile remove` y volver a instalarla |
| `flatpak` | Aplicaciones y runtimes con una actualización pendiente | `nixup update` |

## Los comandos

- `nixup check` lista lo que está desactualizado. Es el comando por defecto y solo lee.
- `nixup update` primero actualiza `flake.lock` en una copia temporal, te enseña qué ha cambiado upstream y escribe el nuevo lock cuando confirmas. Con `--build` hace el build de todos los hosts en la copia antes de preguntar. Después actualiza los paquetes que define tu flake, de uno en uno, tras enseñarte qué ha cambiado upstream. Por último instala las actualizaciones de Flatpak pendientes y actualiza las entradas de `nix profile` que siguen una referencia móvil, con una pregunta cada uno. Con nombres o `--source` lo limitas a algunos inputs, paquetes, apps de Flatpak o entradas del perfil. Con `--test`, `--switch` o `--boot`, termina ejecutando `nh os test`, `switch` o `boot` en un host, si ha escrito algo.
- `nixup list` enseña todo lo que vigila con las versiones que tienes, sin tocar la red.
- `nixup search <nombre>` busca un paquete en tu nixpkgs fijado, tus paquetes, tus inputs y Flatpak.

## Páginas

| Página | Qué cubre |
|---|---|
| [Primeros pasos](getting-started.md) | Requisitos, formas de instalarlo y una primera ejecución |
| [Comandos](commands.md) | Todos los comandos, flags, formatos de salida y códigos de salida |
| [Configuración](configuration.md) | Cómo encuentra nixup tu flake, variables de entorno, token de GitHub, herramientas necesarias |
| [Cómo funciona](how-it-works.md) | Qué comprueba cada sección, cómo se monta la revisión de upstream, qué se escribe y qué nunca |
| [Seguridad](safety.md) | Las garantías y sus límites |
| [CI](ci.md) | Ejecutar nixup desde un workflow programado de GitHub Actions |
| [Limitaciones](limitations.md) | Lo que nixup no ve ni hace |
| [Desarrollo](development.md) | Estructura del código, tests y cómo se prueba |
| [Releases](releases.md) | Cómo se publica una versión |

El proyecto está en [github.com/Villoh/nixup](https://github.com/Villoh/nixup). La versión corta de todo esto está en el [README](../../README.es.md).

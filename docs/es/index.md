---
title: Documentación de nixup
description: Comprobador de actualizaciones y actualizador seguro para NixOS con flakes. Resumen y mapa de la documentación.
lang: es
---

# Documentación de nixup

[English](../index.md) | Español

nixup es una herramienta de línea de comandos para quien gestiona NixOS con un flake. Responde a dos preguntas: qué está desactualizado y qué ha cambiado upstream desde la versión que tienes. Después actualiza `flake.lock` y tus propios paquetes, pero solo tras un build en un sitio seguro y un sí por tu parte.

No activa nada. En el código no hay ningún `switch`, `boot` ni `sudo`, ni demonios ni timers. Cuando nixup escribe un archivo, tú revisas el diff, pruebas el sistema con tus herramientas de siempre y haces el commit.

## Qué comprueba

| Sección | Qué compara | Cómo se actualiza |
|---|---|---|
| `inputs` | Los inputs de rama con el último commit de la rama remota; los fijados a un tag de release con el tag más nuevo | `nixup lock`, o editando `flake.nix` si es un tag fijado |
| `packages` | Cada paquete de la salida `packages.<system>` de tu flake con su tag o release más reciente en GitHub | `nixup pkg` (ejecuta `nix-update`) |
| `profile` | Entradas de `nix profile` fijadas a una revisión de nixpkgs distinta de la del input `nixpkgs` de tu flake | `nix profile upgrade` |
| `flatpak` | Aplicaciones y runtimes con una actualización pendiente | `flatpak update` |

## Los comandos

- `nixup check` lista lo que está desactualizado. Es el comando por defecto y solo lee.
- `nixup lock` actualiza `flake.lock` en una copia temporal, te enseña qué ha cambiado upstream, hace el build de todos los hosts y escribe el nuevo lock cuando confirmas.
- `nixup pkg` actualiza los paquetes que define tu flake, de uno en uno, después de enseñarte qué ha cambiado upstream.

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

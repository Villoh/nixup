---
title: Configuración
description: Cómo encuentra nixup tu flake, qué variables de entorno lee, el token de GitHub y las herramientas que necesita.
lang: es
---

# Configuración

[English](../configuration.md) | **Español**

nixup no tiene archivo de configuración. Se maneja con un flag, unas pocas variables de entorno y el `passthru.updateScript` de tus paquetes.

## Qué flake usa

nixup trabaja sobre un flake: un directorio con `flake.nix` y `flake.lock`. Se queda con lo primero de esta lista que esté definido:

1. `-C <dir>`
2. `NIXUP_FLAKE`
3. `NH_OS_FLAKE`
4. `NH_FLAKE`
5. Si no hay nada, busca hacia arriba desde el directorio actual.

`NH_OS_FLAKE` y `NH_FLAKE` son las variables que lee [nh](https://github.com/nix-community/nh), así que si `nh` ya conoce tu flake, nixup no necesita nada más. En NixOS, `programs.nh.flake` define `NH_FLAKE` por ti:

```nix
{
  programs.nh = {
    enable = true;
    flake = "/home/alice/nixos-config";
  };
}
```

`NH_HOME_FLAKE` y `NH_DARWIN_FLAKE` no se leen. La lectura de las variables de entorno es posterior a la release v0.1.1; esa versión solo conoce `-C` y la búsqueda hacia arriba.

### -C

`-C <dir>` empieza la búsqueda hacia arriba en `<dir>`, así que `-C ~/nixos-config/hosts` encuentra `~/nixos-config`. Además tiene prioridad sobre todas las variables de entorno, y nixup no imprime nada extra.

### Variables de entorno

El valor de `NIXUP_FLAKE`, `NH_OS_FLAKE` o `NH_FLAKE` se usa tal cual:

- Se quita el sufijo `#host`: `~/nixos-config#desktop` equivale a `~/nixos-config`.
- Se quita el prefijo `path:`.
- Un `~` al principio se expande a tu directorio personal.
- Las referencias remotas como `github:owner/repo` se rechazan, porque nixup tiene que escribir en el flake.
- El propio directorio tiene que contener `flake.nix` y `flake.lock`. nixup nunca tira de un directorio padre ni del actual.

Un valor no válido es un error (código de salida 2), aunque una variable posterior hubiera servido:

```text
nixup: $NIXUP_FLAKE: "github:foo/bar" is not a local directory
nixup: $NH_FLAKE=/tmp: no flake.nix + flake.lock there
```

Cuando el flake sale de una variable, nixup lo indica por stderr, y así la salida de `--json` en stdout sigue limpia:

```text
nixup: using /home/alice/nixos-config ($NH_FLAKE)
```

## Color

La salida solo lleva color cuando stdout es una terminal. Cualquier valor no vacío en `NO_COLOR` quita el color, y `TERM=dumb` también.

## Preguntas

`lock` y `pkg` preguntan antes de escribir. Las preguntas solo se contestan si stdin es una terminal, y solo `y` (en mayúscula o minúscula) cuenta como sí. Sin terminal, todas las preguntas se responden que no, así que en ejecuciones desatendidas tienes que pasar `-y`.

## Token de GitHub

nixup lee los tags y los últimos commits de cada rama con `git ls-remote` por HTTPS, y para eso no hace falta token. Usa la API REST de GitHub para comparar commits, para las release notes y para la lista de releases de los paquetes que siguen las releases de GitHub. Para esas llamadas coge el primer token que encuentre:

1. `GH_TOKEN`
2. `GITHUB_TOKEN`
3. La salida de `gh auth token`, si tienes `gh` instalado.

Sin token, las llamadas a la API van sin autenticar y GitHub solo permite 60 por hora. Un `check --review` sobre un flake con muchas entradas desactualizadas puede agotarlas. Cuando falla una llamada a la API, la revisión muestra `review unavailable` con el motivo y el resto sigue adelante.

## Scripts de actualización de los paquetes

`nixup check` y `nixup pkg` solo tienen en cuenta los paquetes que definen `passthru.updateScript`. nixup lee esa línea de comandos y usa algunos de sus argumentos:

- `--use-github-releases`: compara con las releases publicadas en GitHub en vez de con todos los tags.
- `--version=unstable`: permite que la versión más nueva sea una prerelease. Cualquier otro `--version=...` se ignora, porque nixup siempre pasa la versión que has revisado.
- El resto de argumentos se pasan tal cual a `nix-update`.

Por ejemplo:

```nix
{
  passthru.updateScript = nix-update-script {
    extraArgs = [ "--use-github-releases" ];
  };
}
```

## Herramientas necesarias

| Herramienta | Para qué se usa | Si falta |
|---|---|---|
| `git` | `ls-remote` para tags y ramas, la copia temporal, comprobar cambios sin commitear, restaurar un paquete | Fallan casi todas las comprobaciones |
| `nix` | `nix eval` de paquetes y hosts, `nix flake update`, `nix build`, `nix profile list` | La sección `profile` se salta; `packages`, `lock` y `pkg` fallan |
| `nh` | `nh os build` en `nixup lock` | `lock` falla salvo con `--no-build` |
| `nix-update` | `nixup pkg` | nixup ejecuta `nix run nixpkgs#nix-update --` en su lugar |
| `flatpak` | La sección `flatpak` | La sección se salta |
| `gh` | Solo para leer un token con `gh auth token` | Las llamadas a la API van sin token |

nixup no toca el `PATH`. Si lo instalas desde el flake, usa el `nix`, el `nh` y el `git` que ya tengas en el sistema.

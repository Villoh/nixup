---
title: Releases
description: Cómo publicar una versión de nixup con el workflow manual de release y las notas de git-cliff.
lang: es
---

# Releases

[English](../releases.md) | **Español**

Las releases se hacen con un workflow manual de GitHub Actions. No cambies la versión ni crees un tag a mano.

## Publicar una release

Desde la pestaña Actions, o con la CLI de GitHub:

```bash
gh workflow run release -f version=0.3.0                  # sube la versión, hace commit, tag y publica
gh workflow run release -f version=0.3.0 -f dry_run=true  # lo comprueba todo, no publica nada
```

El workflow solo se ejecuta desde `main`.

## Qué hace el workflow

`.github/workflows/release.yml`, en orden:

1. Comprueba que la versión tiene la forma `X.Y.Z`, que es mayor que la actual y que el tag `vX.Y.Z` todavía no existe.
2. Cambia la versión en `flake.nix`. La versión vive en una sola línea, marcada con el comentario `# the one place the version lives`. El workflow encuentra la línea por ese comentario, así que no lo quites.
3. Ejecuta `go vet ./...` y `go test -race -count=1 ./...`.
4. Genera las release notes con [git-cliff](https://git-cliff.org/) a partir de los commits desde el último tag, y las imprime en el log.
5. Salvo con `dry_run`, hace commit del nuevo `flake.nix` en `main` como `chore(release): vX.Y.Z` a través de la API de GitHub, así que GitHub lo firma y sale como Verified. El commit se rechaza si `main` se ha movido después de arrancar el workflow.
6. Publica la release `vX.Y.Z` de GitHub sobre ese commit, con las notas generadas y marcada como latest.

No se adjuntan binarios. Se instala desde el flake, y `nix run github:Villoh/nixup/vX.Y.Z` ejecuta una release concreta.

## Release notes

`cliff.toml` agrupa los commits por su tipo de Conventional Commit:

| Tipo | Sección |
|---|---|
| `feat` | Features |
| `fix` | Bug fixes |
| `perf` | Performance |
| `refactor` | Refactoring |
| `docs` | Documentation |
| `test`, `ci`, `build` | Tests, CI and build |
| `chore` | Maintenance |
| cualquier otro | Other |

Los commits `chore(release)` no aparecen. Cada entrada muestra el scope en negrita, el asunto y un enlace al commit, y las notas acaban con un enlace de comparación al changelog completo.

Para ver en local las notas de la próxima release:

```bash
nix run nixpkgs#git-cliff -- --unreleased --tag v0.3.0 --strip header
```

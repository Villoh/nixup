---
title: CI
description: Ejecuta nixup desde un workflow programado de GitHub Actions para ver las actualizaciones pendientes y abrir una pull request de actualización.
lang: es
---

# CI

[English](../ci.md) | **Español**

nixup no trae un workflow para tu configuración. Aquí tienes cómo montarlo con los flags que existen para ello: `--markdown`, `--json`, `--report` y `-y`.

## Flags para ejecuciones desatendidas

- `check --markdown` imprime en Markdown las entradas desactualizadas o con error. Añade `--review` para tener commits y release notes.
- `check --json` imprime todas las entradas en JSON, para tus propios scripts.
- `update --report <archivo>` añade a un archivo Markdown lo que ha cambiado, la revisión de upstream y el resultado del build. Varias ejecuciones pueden compartir archivo.
- `-y` contesta que sí. Un job de CI no tiene terminal, así que sin `-y` todas las preguntas se responden que no y no se escribe nada.

`check` sale con 1 cuando hay algo desactualizado. En un paso de CI eso cuenta como fallo, así que acepta el 1 de forma explícita y deja que el 2 tumbe el job:

```bash
nixup check --markdown --review >> "$GITHUB_STEP_SUMMARY" || [ $? -eq 1 ]
```

## Una pull request de actualización semanal

Este workflow pone las actualizaciones pendientes en el resumen del job, actualiza `flake.lock` y tus paquetes, y abre una pull request con el informe como descripción. Ponlo en el repositorio de tu configuración, por ejemplo como `.github/workflows/updates.yml`:

```yaml
name: updates

on:
  schedule:
    - cron: "0 6 * * 1" # los lunes a las 06:00 UTC
  workflow_dispatch:

permissions:
  contents: write
  pull-requests: write

jobs:
  updates:
    runs-on: ubuntu-latest
    env:
      GH_TOKEN: ${{ github.token }} # para las llamadas a la API de GitHub de las revisiones
      NIXUP: github:Villoh/nixup
    steps:
      - uses: actions/checkout@v4
      - uses: DeterminateSystems/nix-installer-action@v23

      - name: pending updates
        run: |
          # el código de salida 1 solo significa "hay algo desactualizado"
          nix run "$NIXUP" -- check --source input,package --markdown --review \
            >> "$GITHUB_STEP_SUMMARY" || [ $? -eq 1 ]

      - name: update flake.lock and packages
        run: |
          touch "$RUNNER_TEMP/report.md"
          nix run "$NIXUP" -- update --source input -y --no-build --report "$RUNNER_TEMP/report.md"
          nix run "$NIXUP" -- update --source package -y --report "$RUNNER_TEMP/report.md"

      - uses: peter-evans/create-pull-request@v7
        with:
          branch: nixup/updates
          commit-message: "chore: update flake inputs and packages"
          title: "chore: update flake inputs and packages"
          body-path: ${{ runner.temp }}/report.md
```

Detalles de este workflow:

- `--source input,package` se salta `profile` y `flatpak`, que describen la máquina donde corre nixup, no tu configuración.
- Los dos comandos de update pasan `--source input` y `--source package` por lo mismo: sin `--source`, `update -y` ejecutaría también los pasos de Flatpak y de `nix profile`, que instalan en el runner en vez de editar el repositorio.
- Los dos pasos van en comandos separados para que solo el de inputs se salte el build.
- El runner no tiene `nix-update`, así que `update` lo ejecuta con `nix run nixpkgs#nix-update`.
- `update --source package -y` hace el build de cada paquete actualizado en el runner. Si alguno falla, sus archivos se restauran, nixup sale con 2 y el job para antes de la pull request.
- Si quieres usar el mismo nixup todas las semanas, fija un tag de release en `NIXUP`, por ejemplo `github:Villoh/nixup/v0.2.0`.
- El commit y la pull request los hace `create-pull-request`. nixup nunca hace commit.

## Builds del sistema completo

El workflow pasa `--no-build` al paso de inputs, así que el nuevo lock se escribe sin construir ningún host. Un sistema NixOS completo suele necesitar más disco y más tiempo de los que da un runner alojado por GitHub, y `update` construye todos los hosts de `nixosConfigurations` salvo que pases `-H`.

Tienes tres opciones:

- Dejar `--no-build` en CI y ejecutar `nixup update --source input` (que sí hace el build) o `nh os build` en tu máquina antes de mergear la pull request.
- Construir un solo host con `update --source input -y -H <host> --report ...`. Entonces el runner necesita `nh` en el `PATH` y espacio suficiente para ese host.
- Usar un runner propio (self-hosted) donde quepa el build.

Elijas lo que elijas, nixup nunca activa el resultado. Prueba la configuración mergeada con `nh os test` en la propia máquina.

---
title: CI
description: Ejecuta nixup desde un workflow programado de GitHub Actions para mantener una issue con las actualizaciones pendientes, o para abrir una pull request de actualización.
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

## Una issue semanal

Si actualizas en tu propia máquina, donde se pueden compilar los hosts, basta con un recordatorio. Este workflow mantiene las actualizaciones pendientes en una issue y no cambia nada del repositorio:

```yaml
name: updates

on:
  schedule:
    - cron: "0 6 * * 1" # Mondays at 06:00 UTC
  workflow_dispatch:

permissions:
  contents: read
  issues: write

concurrency: updates

jobs:
  check:
    runs-on: ubuntu-latest
    env:
      GH_TOKEN: ${{ github.token }} # gh, and nixup's GitHub API calls
      TITLE: Pending updates
      LABEL: updates
      NIXUP: github:Villoh/nixup/v0.8.0 # or the rev your flake.lock pins
    steps:
      - uses: actions/checkout@v7
      - uses: DeterminateSystems/nix-installer-action@v23

      - name: update the issue
        run: |
          set -euo pipefail
          nixup=$NIXUP
          sources=input,package

          code=0
          nix run "$nixup" -- check --source "$sources" --json > rows.json ||
            code=$?
          if [ "$code" -ge 2 ]; then # a check failed: red run
            cat rows.json
            exit 1
          fi

          read -r num state < <(
            gh issue list --label "$LABEL" --state all --limit 1 \
              --json number,state --jq '.[0] | "\(.number) \(.state)"'
          ) || true
          open=""
          if [ "${state:-}" = OPEN ]; then open=$num; fi

          if [ "$code" -eq 0 ]; then # everything is up to date
            if [ -n "$open" ]; then
              gh issue close "$open" --comment "Everything is up to date."
            fi
            exit 0
          fi

          # What "something new" means: a name that was not pending before.
          # Versions are left out: a branch or a nightly moves every day, and
          # an entry already pending is already known.
          pending=$(jq -r '[.[] | select(.status == "outdated") | .name]
            | sort | join(" ")' rows.json)
          nix run "$nixup" -- check --source "$sources" --markdown \
            --review > review.md || [ $? -eq 1 ]
          {
            echo "Pending updates as of $(date -u +%F)."
            echo "Apply them locally with \`nixup update\`."
            echo
            # The ACTION column is left out: it is "nixup update <name>"
            # but for the rows that need something else, kept as Notes.
            jq -r '
              def kind: {inputs: "input", packages: "package"}[.] // .;
              def status: if .status != "outdated" then .status
                elif (.warning // "") | test("MAJOR") then "**major**"
                elif (.warning // "") != "" then "**0.x-minor**"
                else "update" end;
              def notes: [(.note // "" | select(startswith("nixup update")
                | not)), (.hint // empty)] | map(select(. != ""))
                | join("; ");
              "| Source | Name | Current | Latest | Status | Notes |",
              "|---|---|---|---|---|---|",
              (.[] | select(.status != "ok")
                | "| \(.section | kind) | \(.name) | `\(.current)` " +
                  "| `\(.latest // "")` | \(status) | \(notes) |")
            ' rows.json
          } > body.md
          # Issue bodies stop at 65536 characters: drop the details if
          # they do not fit.
          size=$(($(wc -c < body.md) + $(wc -c < review.md)))
          if [ "$size" -lt 60000 ]; then
            {
              printf '\n<details><summary>What changed upstream</summary>\n\n'
              cat review.md
              printf '\n</details>\n'
            } >> body.md
          fi

          printf '\n<!-- nixup: %s -->\n' "$pending" >> body.md

          if [ -n "$open" ]; then
            before=$(gh issue view "$open" --json body --jq .body |
              sed -n 's/^<!-- nixup: \(.*\) -->$/\1/p')
            new=$(comm -23 <(tr ' ' '\n' <<< "$pending" | sort) \
              <(tr ' ' '\n' <<< "$before" | sort))
            if [ -z "$new" ]; then # nothing new: refresh it quietly
              gh issue edit "$open" --body-file body.md
              exit 0
            fi
          fi

          gh label create "$LABEL" --color 4A6FC4 --force \
            --description "Pending flake updates (nixup)"
          url=$(gh issue create --title "$TITLE ($(date -u +%F))" \
            --label "$LABEL" --body-file body.md)
          if [ -n "$open" ]; then
            gh issue close "$open" --comment "Superseded by #${url##*/}."
          fi
```

Notas sobre este workflow:

- Como mucho hay una issue abierta. Una issue nueva, que te notifica, sustituye a la abierta solo cuando aparece un input o paquete que antes no estaba pendiente; si no, la abierta se actualiza sin avisar. Cuando no queda nada pendiente, se cierra.
- "Nuevo" compara nombres, no versiones: un input de rama o una nightly se mueve cada día, y lo que ya estaba pendiente ya lo conoces. Los nombres se guardan en un comentario HTML al final de la issue.
- La tabla sale de `check --json`. No tiene columna de comando: todas las filas se aplican con `nixup update <nombre>`.
- `--review` añade los commits y las release notes de upstream en una sección plegada, que se omite si el cuerpo pasaría del límite de 65536 caracteres de GitHub.
- `GH_TOKEN` sirve tanto para `gh` como para las llamadas de nixup a la API de GitHub. Sin token, nixup tiene un límite de 60 llamadas a la hora y las revisiones fallan.
- Si un check falla (código de salida 2), la ejecución sale en rojo y GitHub te avisa por correo.
- Si tu flake tiene nixup como input, coge la versión del `flake.lock`, así la issue coincide con lo que dice `nixup check` en tu máquina: `nixup="github:Villoh/nixup/$(jq -r .nodes.nixup.locked.rev flake.lock)"`.

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
      - uses: actions/checkout@v7
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
- Si quieres usar el mismo nixup todas las semanas, fija un tag de release en `NIXUP`, por ejemplo `github:Villoh/nixup/v0.8.0`.
- El commit y la pull request los hace `create-pull-request`. nixup nunca hace commit.

## Builds del sistema completo

El workflow pasa `--no-build` al paso de inputs, así que el nuevo lock se escribe sin construir ningún host. Un sistema NixOS completo suele necesitar más disco y más tiempo de los que da un runner alojado por GitHub, y `update` construye todos los hosts de `nixosConfigurations` salvo que pases `-H`.

Tienes tres opciones:

- Dejar `--no-build` en CI y ejecutar `nixup update --source input` (que sí hace el build) o `nh os build` en tu máquina antes de mergear la pull request.
- Construir un solo host con `update --source input -y -H <host> --report ...`. Entonces el runner necesita `nh` en el `PATH` y espacio suficiente para ese host.
- Usar un runner propio (self-hosted) donde quepa el build.

Elijas lo que elijas, no pases `--test`, `--switch` ni `--boot` en CI: activarían la configuración en el runner. Sin ellos nixup nunca activa el resultado. Prueba la configuración mergeada con `nh os test` en la propia máquina.

---
title: Limitaciones
description: Lo que nixup no ve, no comprueba y no hace.
lang: es
---

# Limitaciones

[English](../limitations.md) | **Español**

## Solo upstreams de GitHub

- Inputs: solo se comprueban los inputs directos de tipo `github`. Los `gitlab:`, `git+https:`, `path:`, tarballs y demás no aparecen en el informe.
- Paquetes: un paquete cuyas URLs de código fuente y homepage no apuntan a github.com sale como `info` con `no GitHub upstream: check by hand`.
- Las revisiones (commits, enlaces de comparación, release notes) salen solo de la API de GitHub.

## Solo lo que expone el flake

- La sección `packages` lee la salida `packages.<system>` del propio flake, para el sistema en el que se ejecuta nixup. Un `fetchurl` dentro de un módulo, un `rev` fijado en un overlay o un paquete definido solo para otro sistema son invisibles. Mételo en la salida `packages` si quieres que se compruebe.
- Un paquete necesita `passthru.updateScript` para que se compruebe. Si no lo tiene, nixup lo marca como `info` con `no passthru.updateScript`.
- `nixup pkg` protege y restaura el directorio del archivo que define cada paquete, sacado de su `meta.position`. Un paquete sin `meta.position`, o definido fuera de este flake, se rechaza.
- Los inputs transitivos no se comprueban. `lock` los actualiza junto con todo lo demás y solo dice cuántos se han movido.

## lock solo hace el build de hosts NixOS

`lock` hace el build de cada entrada de `nixosConfigurations` con `nh os build`. Home Manager, nix-darwin y otras salidas no se construyen. En un flake sin `nixosConfigurations`, usa `lock --no-build`; si no, el listado de hosts falla con código de salida 2.

## Algunas secciones solo informan

- `flatpak`: nixup lista las actualizaciones pendientes. El `flatpak update` lo ejecutas tú.
- `profile`: nixup compara la revisión de nixpkgs de las entradas de `nix profile` con el input de tu flake llamado `nixpkgs`. El `nix profile upgrade` lo ejecutas tú.
- Inputs fijados a un tag: `nix flake update` no mueve un tag, así que `lock` tampoco. Cambia el tag en `flake.nix`.

## Heurísticas de versiones

- Un `ref` que empieza por un dígito, por `v` más un dígito, o por un prefijo `palabra/` seguido de eso (`v1.2`, `1.2`, `dms/1.2`) se trata como tag de release. Todo lo demás es una rama.
- Las versiones basadas en fechas, como `20240101.1`, no reciben aviso de salto mayor: un primer número de 1000 o más se interpreta como año o fecha, así que una versión mayor real tan alta tampoco se avisaría.
- La pista de nixpkgs solo encuentra un paquete por su nombre o, en el caso de `foo-bin`, como `foo`. Si en nixpkgs se llama de otra forma, no lo ve. Además necesita pruebas de que es el mismo proyecto (repositorio u homepage): nunca sugiere un homónimo, pero tampoco el paquete correcto si sus metadatos no coinciden.
- Las release notes se leen de las 50 releases más recientes de GitHub, y nixup enseña como mucho tres por actualización.

## search

- Solo busca en los nombres, no en las descripciones: buscar en las descripciones obliga a evaluar todos los paquetes de nixpkgs, y eso tarda cerca de medio minuto (es lo que hace `nix search`).
- Solo atributos de primer nivel: no busca dentro de `python3Packages.requests` ni de otros conjuntos de paquetes.
- `●` compara nombre y versión con la clausura del sistema en marcha. Una biblioteca que arrastra otro paquete cuenta como instalada, y no mira Home Manager fuera de NixOS.
- `search` evalúa el flake con `--impure`, porque `builtins.getFlake` sobre una ruta local lo necesita. Solo lee: no construye ni escribe nada.

## Otros límites

- nixup no calcula hashes. Eso lo hace `nix-update`.
- No hay archivo de configuración, demonio, timer ni notificaciones. Prográmalo tú, por ejemplo [en CI](ci.md).
- Sin token de GitHub, la API permite 60 peticiones por hora, y un `--review` grande puede pasarse.
- El flake trae builds solo para `x86_64-linux` y `aarch64-linux`.

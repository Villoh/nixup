---
title: Seguridad
description: Qué garantiza nixup cuando actualiza tu flake y dónde terminan esas garantías.
lang: es
---

# Seguridad

[English](../safety.md) | **Español**

nixup está hecho para que una actualización fallida o que no quieres deje tu repositorio como estaba, y para que nada llegue al sistema en marcha sin pasar por ti. Aquí tienes lo que se cumple y lo que no.

## Qué garantiza nixup

- **`check` no escribe nada.** Lee `flake.lock`, evalúa tus paquetes y consulta a git, GitHub, `nix profile` y Flatpak.
- **`lock` no toca tu repositorio hasta el final.** La actualización y los builds se hacen en una copia temporal. Tu `flake.lock` solo se sustituye cuando todos los builds pasan y contestas que sí, o si has pasado `-y`.
- **Si falla un build de `lock`, `flake.lock` no se toca.** nixup imprime `build of <host> FAILED: flake.lock left untouched.` y sale con 2.
- **Contestar que no a `lock` no cambia nada.** nixup imprime `Discarded. Nothing changed.`
- **`lock` no arranca con un `flake.lock` modificado.** Si `flake.lock` tiene cambios sin commitear, para con código de salida 1 antes de hacer nada.
- **`pkg` actualiza a la versión que has revisado.** Le pasa `--version=<esa versión>` a `nix-update`, así que una release publicada mientras leías no se cuela.
- **Si falla la actualización o el build de `pkg`, se deshace.** nixup ejecuta `git checkout -- <dir del paquete>` e imprime `nix-update failed; <dir del paquete> restored.` o `build FAILED; <dir del paquete> restored.`
- **`pkg` rechaza un directorio de paquete modificado.** Si `<dir del paquete>` tiene cambios sin commitear, ese paquete se salta con código de salida 1, porque al deshacer se perderían esos cambios.
- **Sin terminal, la respuesta es no.** Si stdin no es una terminal, todas las preguntas se responden que no. En ejecuciones desatendidas tienes que pasar `-y` a propósito.
- **No se hace commit ni se activa nada.** Ni `git commit`, ni `switch`, ni `boot`, ni `test`, ni `sudo`. Después de actualizar, `git diff`, `nix flake check` y `nh os test` los sigues ejecutando tú.

## Dónde terminan las garantías

- **Que el build pase no significa que el sistema arranque.** `nh os build` demuestra que la configuración se evalúa y se construye. No demuestra que la nueva generación funcione. Pruébala con `nh os test` antes de hacer el switch.
- **`--no-build` quita la comprobación del build.** Con `lock --no-build -y` el nuevo lock se escribe sin construir nada. Con `pkg --no-build` un paquete roto no se nota hasta más tarde.
- **`-y` se salta tu revisión.** La revisión se sigue imprimiendo y, con `--report`, se guarda. Pero nadie la lee antes de escribir.
- **`pkg` edita tu copia de trabajo real.** A diferencia de `lock`, `nix-update` se ejecuta en tu repositorio. Al deshacer solo se recuperan los archivos versionados del directorio del archivo que define el paquete (sacado de `meta.position`). Los archivos que `nix-update` cambie en otros sitios ni se comprueban ni se restauran. Los archivos nuevos sin versionar no se borran.
- **Si falla la restauración, se avisa pero no se reintenta.** Si `git checkout` falla, nixup imprime `nixup: restore failed:` con el motivo y te toca limpiar a mano.
- **La copia temporal es tu árbol de trabajo, no tu último commit.** Los archivos sin commitear y los sin versionar (pero no ignorados) entran en el build de `lock`. Los enlaces simbólicos no se copian.
- **La revisión depende de GitHub.** Si la API no responde o has agotado el límite, verás `review unavailable` y podrás aceptar la actualización igualmente.
- **El aviso de cambio incompatible es una suposición a partir de los números de versión.** Una versión de parche puede romper cosas. Las versiones basadas en fechas (`20261106.1`, `2026.10.14`) se reconocen y no reciben aviso, porque sus números no indican compatibilidad.
- **`--report` se escribe antes de la pregunta de `lock`.** El informe describe el cambio propuesto aunque luego contestes que no.

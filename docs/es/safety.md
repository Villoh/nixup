---
title: Seguridad
description: Qué garantiza nixup cuando actualiza tu flake y dónde terminan esas garantías.
lang: es
---

# Seguridad

[English](../safety.md) | **Español**

nixup está hecho para que una actualización fallida o que no quieres deje tu repositorio como estaba, y para que nada llegue al sistema en marcha sin pasar por ti. Los pasos de inputs y de paquetes de `update` solo editan archivos de tu repositorio. Los pasos de Flatpak y de `nix profile` instalan en la máquina en el momento, tras un sí. Aquí tienes lo que se cumple y lo que no.

## Qué garantiza nixup

- **`check` no escribe nada.** Lee `flake.lock`, evalúa tus paquetes y consulta a git, GitHub, `nix profile` y Flatpak.
- **El paso de inputs de `update` no toca tu repositorio hasta el final.** La actualización y los builds se hacen en una copia temporal. Tu `flake.lock` solo se sustituye cuando todos los builds pasan y contestas que sí, o si has pasado `-y`.
- **Si falla el build de un host, `flake.lock` no se toca.** nixup imprime `build of <host> FAILED: flake.lock left untouched.` y sale con 2.
- **Contestar que no al nuevo `flake.lock` no cambia nada.** nixup imprime `Discarded. Nothing changed.`
- **`update` pregunta antes de actualizar sobre un `flake.lock` con cambios sin commitear, y los conserva.** La pregunta nombra los inputs que tu `flake.lock` de trabajo ya ha movido. Si dices que no, imprime `Skipped.` y el paso de inputs termina sin ejecutar nada; el paso de paquetes se ejecuta igualmente. Si dices que sí, la actualización parte de tu `flake.lock` de trabajo, y los cambios que enseña son solo los que ha movido nixup.
- **`update` lleva cada paquete a la versión que has revisado.** Le pasa `--version=<esa versión>` a `nix-update`, así que una release publicada mientras leías no se cuela.
- **Si falla la actualización o el build de un paquete, se deshace.** Antes de ejecutar `nix-update`, nixup guarda todos los archivos de `<dir del paquete>`. Si algo falla, los vuelve a escribir tal y como estaban, con los cambios sin commitear incluidos, borra los archivos que ha añadido `nix-update` e imprime `nix-update failed; <dir del paquete> restored.` o `build FAILED; <dir del paquete> restored.`
- **`update` pregunta antes de actualizar sobre un directorio de paquete con cambios sin commitear, y los conserva.** Si `<dir del paquete>` tiene cambios sin commitear, nixup pregunta antes de la pregunta de actualizar. Si dices que no, imprime `Skipped.` y sigue con el siguiente paquete. `--allow-dirty` y `-y` se saltan esta pregunta.
- **Los pasos de Flatpak y de `nix profile` preguntan antes de instalar.** La pregunta lo dice: `Update 2 Flatpak refs now? This installs them right away. [y/N]`. Si dices que no, no se instala nada.
- **El paso de Flatpak solo actualiza lo que te ha enseñado.** Le pasa a `flatpak update` exactamente los refs de su tabla. Antes apunta el commit desplegado de cada ref, y después imprime `flatpak update --commit=<commit anterior> <ref>` para cada uno, también tras un fallo.
- **El paso de `nix profile` te dice cómo deshacerlo.** Después de actualizar imprime `To undo it: nix profile rollback`.
- **Las entradas de `nix profile` fijadas nunca se tocan.** Una entrada fijada a una revisión se deja fuera de `nix profile upgrade`. nixup solo te dice cómo actualizarla a mano.
- **Sin terminal, la respuesta es no.** Si stdin no es una terminal, todas las preguntas se responden que no. En ejecuciones desatendidas tienes que pasar `-y` a propósito.
- **No se activa ninguna generación ni se hace commit.** Ni `git commit`, ni `switch`, ni `boot`, ni `test`, ni `sudo`. Después de actualizar, `git diff`, `nix flake check` y `nh os test` los sigues ejecutando tú.

## Dónde terminan las garantías

- **Que el build pase no significa que el sistema arranque.** `nh os build` demuestra que la configuración se evalúa y se construye. No demuestra que la nueva generación funcione. Pruébala con `nh os test` antes de hacer el switch.
- **`--no-build` quita la comprobación del build.** Con `update --no-build -y` el nuevo lock se escribe sin construir ningún host, y un paquete roto no se nota hasta más tarde.
- **`-y` se salta tu revisión.** La revisión se sigue imprimiendo y, con `--report`, se guarda. Pero nadie la lee antes de escribir. `-y` también actualiza sobre cambios sin commitear, igual que `--allow-dirty`.
- **El paso de paquetes edita tu copia de trabajo real.** A diferencia del paso de inputs, `nix-update` se ejecuta en tu repositorio. Al deshacer se recuperan todos los archivos normales del directorio del archivo que define el paquete (sacado de `meta.position`), versionados o no. Los archivos que `nix-update` cambie en otros sitios ni se comprueban ni se restauran. Los enlaces simbólicos no se guardan, y los directorios que añada `nix-update` se quedan, vacíos.
- **Si falla la restauración, se avisa pero no se reintenta.** Si falla la escritura de los archivos, nixup imprime `nixup: restore failed:` con el motivo y te toca limpiar a mano.
- **La copia temporal es tu árbol de trabajo, no tu último commit.** Los archivos sin commitear y los sin versionar (pero no ignorados) entran en los builds de los hosts. Los enlaces simbólicos no se copian.
- **La revisión depende de GitHub.** Si la API no responde o has agotado el límite, verás `review unavailable` y podrás aceptar la actualización igualmente.
- **El aviso de cambio incompatible es una suposición a partir de los números de versión.** Una versión de parche puede romper cosas. Las versiones basadas en fechas (`20261106.1`, `2026.10.14`) se reconocen y no reciben aviso, porque sus números no indican compatibilidad.
- **`--report` se escribe antes de la pregunta sobre `flake.lock`.** El informe describe el cambio propuesto aunque luego contestes que no.
- **Los pasos de Flatpak y de `nix profile` cambian la máquina en el acto.** No hay copia temporal, ni comprobación de build, ni diff que revisar: lo que aceptas queda instalado al terminar el paso. Con `-y`, sin pregunta.
- **Volver atrás depende de Flatpak y de Nix.** `flatpak update --commit` necesita que el commit anterior siga disponible en el remoto, y un ref cuyo commit no se ha podido leer no tiene línea. `nix profile rollback` vuelve a la generación anterior del perfil, que solo es la de antes de la actualización si nada ha cambiado el perfil desde entonces.
- **Las actualizaciones de Flatpak no tienen revisión de upstream.** El paso de Flatpak solo enseña versiones, no commits ni release notes.

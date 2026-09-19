#!/bin/bash
# Devbox: full toolchain container for working on Podland (Go, Node,
# pre-cloned Podland + podman-fork repos). Built from Containerfile.dev —
# a bare `podman build .` at the repo root builds the app image instead.
if ! podman image exists podland-builder; then
    podman build -t podland-builder -f Containerfile.dev .
fi
podman run -it --network host \
     --privileged --systemd=always --security-opt label=disable \
     --device /dev/fuse \
     podland-builder

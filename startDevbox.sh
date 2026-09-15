#!/bin/bash
podman run --name podland-builder-worker -it \
     --privileged --systemd=always --security-opt label=disable \
     --device /dev/fuse \
     podland-builder

#!/bin/bash
podman run -it --network host \
     --privileged --systemd=always --security-opt label=disable \
     --device /dev/fuse \
     podland-builder

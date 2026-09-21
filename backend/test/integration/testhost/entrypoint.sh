#!/bin/sh
# Podland integration-test host boot — PID 1 without systemd.
#
# The host running the suite is often itself a container without cgroup
# delegation; any nested systemd manager dies there at boot ("Failed to
# create /init.scope"). The boot contract the harness relies on is
# therefore provided by plain processes:
#
#   - core's rootless podman API socket at /run/user/1000/podman/podman.sock
#     (served by `podman system service`, standing in for systemd's user
#     podman.socket activation)
#   - sshd in the foreground on :22 (this script execs it and becomes its
#     lifetime)
set -eu

# Host keys land in the /etc/ssh volume: generated at first boot, stable
# across runs afterwards (`ssh-keygen -A` only creates missing keys), so
# the bindings' known_hosts auto-append happens exactly once.
ssh-keygen -A >/dev/null

# sshd's privilege-separation directory (systemd-tmpfiles would create
# this on a systemd host).
mkdir -p /run/sshd

# core's runtime dir; the socket path is /run/user/1000/podman/podman.sock
# and core's uid must be exactly 1000. The podman/ subdirectory is
# pre-created because `podman system service` expects its parent to
# exist (systemd's RuntimeDirectory does this on a systemd host).
install -d -m 700 -o core -g core /run/user/1000
install -d -m 700 -o core -g core /run/user/1000/podman

# Rootless podman API, served for the container's lifetime. --time=0
# means no idle shutdown — the socket is as reliable as the container.
runuser -u core -- env HOME=/home/core XDG_RUNTIME_DIR=/run/user/1000 \
    podman system service --time=0 unix:///run/user/1000/podman/podman.sock &

exec /usr/sbin/sshd -D -e

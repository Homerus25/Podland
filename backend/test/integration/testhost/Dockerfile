# Podland integration-test host — a podman machine stand-in.
#
# Built by `make testhost-build` (backend/Makefile); the harness boots
# it with the flags documented next to that target (see
# test/integration/testhost.go for the automated run contract).
#
# The image is the full machine-parity stack the suite relies on:
# system systemd, sshd, a lingering `core` user (uid/gid 1000) whose
# user systemd manager exists from boot, and rootless podman behind
# /run/user/1000/podman/podman.sock using fuse-overlayfs — inner native
# overlay would be overlay-on-overlay, which the kernel denies.
#
# Deliberately NOT baked in: SSH host keys (generated into the /etc/ssh
# volume at first boot, so they stay stable across runs and the
# bindings' known_hosts auto-append happens exactly once) and any
# authorized_keys (each run injects its own fresh key via podman exec).
FROM fedora:43

RUN dnf -y --setopt=install_weak_deps=False install \
        podman fuse-overlayfs passt openssh-server systemd \
    && dnf clean all

# core with uid/gid 1000 and subuid/subgid ranges — the rootless podman
# identity. uid must be exactly 1000: the suite registers and asserts
# the rootless socket path /run/user/1000/podman/podman.sock.
#
# The ranges are deliberately 1-999 + 1001-65536, not the traditional
# 100000+ single range, for two reasons:
#   - When the HOST podman is itself rootless, only ~64k ids are mapped
#     into the outer container; a 100000+ range would not exist there
#     and every inner `podman run` would fail at userns setup with
#     EPERM. 1..65536 sits inside every mapping podman produces by
#     default, on rootful and rootless hosts alike.
#   - The two ranges around core's own uid cover the ENTIRE image uid
#     space (inner 0..65535). OCI images routinely carry uid 65534
#     (nobody) — e.g. busybox's /home — and unpacking fails with
#     "lchown: invalid argument" when that uid is unmappable.
# The files are written wholesale so the image owns the full contents
# (no stray ranges from base-image defaults).
RUN useradd --create-home --uid 1000 --user-group core \
    && printf 'core:1:999\ncore:1001:64536\n' > /etc/subuid \
    && printf 'core:1:999\ncore:1001:64536\n' > /etc/subgid
# newuidmap/newgidmap ship with filecaps (cap_setuid/cap_setgid), but
# the security.capability xattr is dropped by rootless podman builds —
# restore privilege the xattr-free way: plain setuid mode bits, which
# always survive layer commits. Without this, every inner rootless
# `podman run` dies at userns setup with EPERM.
RUN chmod 4755 /usr/bin/newuidmap /usr/bin/newgidmap

# Linger: core's user systemd manager must exist from boot without a
# login session. No systemd runs during build, so create the marker
# directly instead of invoking a live `loginctl enable-linger`.
RUN mkdir -p /var/lib/systemd/linger && touch /var/lib/systemd/linger/core

# Activate podman.socket for every user, so core's user manager exposes
# the rootless socket at boot. Again no live systemctl: create the
# symlink that `systemctl --global enable podman.socket` would.
RUN mkdir -p /etc/systemd/user/sockets.target.wants \
    && ln -s /usr/lib/systemd/user/podman.socket \
        /etc/systemd/user/sockets.target.wants/podman.socket

# Rootless podman config for core: force fuse-overlayfs. Events go to
# journald — the same backend a podman machine's Fedora defaults pick —
# because `podman logs --follow` refuses to run with the journald
# log-driver over a non-journald events backend, and journald works
# here (systemd is PID 1; rootless container-root maps to core's uid,
# so the entries are readable without extra group membership).
RUN mkdir -p /home/core/.config/containers \
    && printf '[storage]\ndriver = "overlay"\nmount_program = "/usr/bin/fuse-overlayfs"\n\n[engine]\nevents_logger = "journald"\n' \
        > /home/core/.config/containers/containers.conf \
    && chown -R core:core /home/core/.config

# Mountpoint for the persistent image-cache volume. Must be core-owned
# so the rootless store survives first-use volume initialization.
RUN mkdir -p /home/core/.local/share/containers \
    && chown core:core /home/core/.local/share/containers

# sshd on at boot; host keys generate at first boot into /etc/ssh. The
# trailing rms pin the no-keys-committed invariant (both are no-ops on
# a clean base image — they exist so a base-image change that starts
# shipping keys fails loudly here instead of at runtime).
RUN systemctl enable sshd.service \
    && rm -f /etc/ssh/ssh_host_*_key* \
    && rm -f /home/core/.ssh/authorized_keys

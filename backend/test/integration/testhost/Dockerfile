FROM registry.access.redhat.com/ubi9/ubi-init

RUN dnf -y install podman openssh-server  && dnf clean all
RUN systemctl enable sshd.service
RUN systemctl enable podman.socket

# ubi9's storage.conf defaults to the overlay driver. Nested one level
# deep — this image's own rootfs is overlay/fuse-overlayfs wherever the
# suite's host is itself a container — overlay container starts fail
# with `crun: mkdirat "proc": Operation not permitted`, because the
# nested overlay upper sits on a filesystem the kernel refuses. The vfs
# driver has no mounting tricks, so the test host works on every
# layering; the suite's inner workload (busybox) is tiny, so the copy
# cost is moot.
RUN sed -i 's/^driver = "overlay"/driver = "vfs"/' /etc/containers/storage.conf

# The default podman network ships with subnet 10.88.0.0/16 — the same
# subnet the outer podman (the one running this very container) uses for
# its own bridge. The first nested container start therefore installs a
# conflicting 10.88.0.0/16 route inside the test host and the published
# sshd port stops answering. Move the default network to a subnet no
# outer podman deployment uses by default.
RUN mkdir -p /etc/containers/containers.conf.d && \
    printf '[network]\ndefault_subnet = "10.244.0.0/24"\n' \
    > /etc/containers/containers.conf.d/10-default-subnet.conf

RUN printf '\nPermitRootLogin yes\n' >> /etc/ssh/sshd_config 

CMD [ "/sbin/init" ]

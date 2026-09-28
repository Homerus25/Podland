FROM registry.access.redhat.com/ubi9/ubi-init

RUN dnf -y install podman openssh-server  && dnf clean all
RUN systemctl enable sshd.service
RUN systemctl enable podman.socket

RUN printf '\nPermitRootLogin yes\n' >> /etc/ssh/sshd_config 

CMD [ "/sbin/init" ]

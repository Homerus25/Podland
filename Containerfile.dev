#FROM quay.io/podman/stable
FROM localhost/podland-testhost

RUN dnf -y install golang nodejs npm btrfs-progs-devel gpgme-devel && dnf clean all
RUN npm install -g @charmland/crush

#USER podman

ARG GIT_USER=your_username
ARG GIT_TOKEN=your_token_or_password
#RUN git clone http://${GIT_USER}:${GIT_TOKEN}@http://rack.home:9250/homerus/podman.git /app

RUN git config --global credential.helper store

# Pre-seed the credentials file so git pull/push never prompt
# RUN printf 'https://${GIT_USER}:${GIT_TOKEN}@rack.home:9250\n' > /home/podman/.git-credentials && \
#     chmod 600 /home/podman/.git-credentials

RUN git clone http://${GIT_USER}:${GIT_TOKEN}@rack.home:9250/homerus/Podland.git /root/Podland
RUN git clone http://${GIT_USER}:${GIT_TOKEN}@rack.home:9250/homerus/podman.git /root/podman

#WORKDIR /home/podman/Podland
WORKDIR /root/Podland

#RUN printf 'go 1.26.8\n\nuse .\n\nreplace go.podman.io/podman/v6 => /home/podman/podman\n' > go.work
RUN cd backend && sed -i 's|/home/homerus/repos/podman/|/root/podman|' go.mod && make tidy
#&& git checkout go.mod
#make test-integration
RUN cd frontend && npm install

# RUN cd backend/ && make tidy

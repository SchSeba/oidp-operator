FROM quay.io/fedora/fedora:latest

USER 0

RUN curl -fsSL https://code-server.dev/install.sh | sh

RUN dnf update -y && dnf -y install dnf-plugins-core && dnf config-manager --add-repo https://download.docker.com/linux/fedora/docker-ce.repo && \
    dnf install -y gzip rust cargo procps-ng bash-completion iproute zsh htop sudo openssh-clients man dumb-init openssl vim git golang pip docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin && \
    curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.19.0/kind-linux-amd64 && \
    chmod +x ./kind && \
    mv ./kind /usr/local/bin/kind && curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash && \
    curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" && mv ./kubectl /usr/local/bin/kubectl && chmod 777 /usr/local/bin/kubectl && \
    curl -sLO https://github.com/argoproj/argo-workflows/releases/download/v3.5.5/argo-linux-amd64.gz && \
    gunzip argo-linux-amd64.gz && \
    chmod +x argo-linux-amd64 && \
    mv ./argo-linux-amd64 /usr/bin/argo

# go packages
RUN curl -Lo /usr/local/bin/operator-sdk https://github.com/operator-framework/operator-sdk/releases/download/v1.32.0/operator-sdk_linux_amd64 && \
    chmod +x /usr/local/bin/operator-sdk && \
    dnf install -y https://github.com/golangci/golangci-lint/releases/download/v1.55.2/golangci-lint-1.55.2-linux-amd64.rpm

# python packages
RUN pip3 install ansible

COPY hack/code-server-entrypoint.sh /usr/bin/entrypoint.sh

# Allow users to have scripts run on container startup to prepare workspace.
# https://github.com/coder/code-server/issues/5177
ENV ENTRYPOINTD=${HOME}/entrypoint.d
ENV GOPATH=/root/go


EXPOSE 8080
# This way, if someone sets $DOCKER_USER, docker-exec will still work as
# the uid will remain the same. note: only relevant if -u isn't passed to
# docker-run.
WORKDIR /root
ENTRYPOINT ["/usr/bin/entrypoint.sh", "--bind-addr", "0.0.0.0:8080", "."]
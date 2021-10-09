FROM ubuntu:21.04

ARG VERSION=v1.61.0

ARG USERNAME=code
ARG USER_UID=1000
ARG USER_GID=1000

RUN apt update && \
    apt install -y tini git wget sudo && \
    rm -rf /var/lib/apt/lists/*

RUN mkdir -p /opt/code && \
    wget https://github.com/gitpod-io/openvscode-server/releases/download/openvscode-server-${VERSION}/openvscode-server-${VERSION}-linux-x64.tar.gz && \
    tar -xzf openvscode-server-${VERSION}-linux-x64.tar.gz -C /opt/code --strip 1 && \
    chown -R root:root /opt/code && \
    rm -f openvscode-server-${VERSION}-linux-x64.tar.gz

RUN groupadd --gid $USER_GID $USERNAME \
    && useradd --uid $USER_UID --gid $USERNAME -m $USERNAME \
    && echo $USERNAME ALL=\(root\) NOPASSWD:ALL > /etc/sudoers.d/$USERNAME \
    && chmod 0440 /etc/sudoers.d/$USERNAME

RUN mkdir -p /workspace && \
    chown -R $USERNAME:$USERNAME /workspace

USER $USERNAME

ENV PATH /opt/code/bin:$PATH

ENV LANG C.UTF-8
ENV LC_ALL C.UTF-8

WORKDIR /workspace

ENV EDITOR=code
ENV VISUAL=code
ENV GIT_EDITOR="code --wait"

EXPOSE 3000

ENTRYPOINT [ "/usr/bin/tini", "--" ]
CMD [ "/opt/code/server.sh" ]
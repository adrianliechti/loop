FROM debian:bullseye-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    tini \
    netcat \
    python3 \
    openssh-server \
    sshfs \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /run/sshd \
    && passwd -d root

COPY --chown=root:root ssh_config /etc/ssh/ssh_config
COPY --chown=root:root sshd_config /etc/ssh/sshd_config

ENTRYPOINT ["tini", "--"]
CMD [ "/usr/sbin/sshd", "-4", "-D" ]
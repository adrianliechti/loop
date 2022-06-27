FROM debian:bullseye-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    tini \
    netcat \
    libcap2-bin \
    python3 \
    openssh-server \
    sshfs \
    && rm -rf /var/lib/apt/lists/*

RUN passwd -d root \
    && mkdir -p /run/sshd \
    && chmod 777 /run/sshd \
    && chmod 644 /etc/ssh/ssh_host_*

COPY --chown=root:root ssh_config /etc/ssh/ssh_config
COPY --chown=root:root sshd_config /etc/ssh/sshd_config

RUN setcap 'cap_net_bind_service=+ep' /usr/sbin/sshd

ENTRYPOINT ["tini", "--"]
CMD [ "/usr/sbin/sshd", "-4", "-D", "-E", "/dev/stdout" ]
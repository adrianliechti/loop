FROM alpine:3.15

RUN apk add --no-cache \
    tini \
    shadow \
    libcap \
    bash \
    python3 \
    openssh \
    sshfs

RUN passwd -d root \
    && mkdir -p /run/sshd \
    && chmod 775 /run/sshd \
    && chmod 644 /etc/shadow /etc/shadow-

COPY --chown=root:root ssh_config /etc/ssh/ssh_config
COPY --chown=root:root sshd_config /etc/ssh/sshd_config

COPY --chown=root:root docker-entrypoint.sh /usr/bin/docker-entrypoint.sh

RUN setcap 'cap_net_bind_service=+ep' /usr/sbin/sshd

ENTRYPOINT ["tini", "--"]
CMD [ "/usr/bin/docker-entrypoint.sh" ]
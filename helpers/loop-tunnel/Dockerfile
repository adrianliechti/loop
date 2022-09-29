FROM alpine:3.16

RUN apk add --no-cache \
    tini \
    shadow \
    libcap \
    bash \
    python3 \
    openssh \
    sshfs

RUN useradd --system -u 1000 -g 0 --no-user-group --shell /bin/bash loop && \
    passwd -d loop && \
    usermod -U loop

RUN mkdir -p /run/sshd && \
    chown -R 0 /run/sshd && \
    chmod g+w /etc/passwd /etc/group && \
    chmod g+rwX /home && \
    chmod g+rwX /run/sshd

COPY --chown=root:root docker-entrypoint.sh /usr/bin/docker-entrypoint.sh

COPY --chown=root:root ssh_config /etc/ssh/ssh_config
COPY --chown=root:root sshd_config /etc/ssh/sshd_config

RUN setcap 'cap_net_bind_service=+ep' /usr/sbin/sshd

USER 1000

ENTRYPOINT ["tini", "--"]
CMD [ "/usr/bin/docker-entrypoint.sh" ]
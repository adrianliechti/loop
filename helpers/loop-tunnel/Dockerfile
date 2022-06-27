FROM alpine:3.15

RUN apk add --no-cache \
    tini \
    libcap \
    bash \
    python3 \
    openssh \
    sshfs

RUN passwd -d root \
    && chmod 755 /var/empty \
    && mkdir -p /run/sshd \
    && chmod 777 /run/sshd \
    && chmod u+s /usr/sbin/sshd \
    && ssh-keygen -A

COPY --chown=root:root ssh_config /etc/ssh/ssh_config
COPY --chown=root:root sshd_config /etc/ssh/sshd_config

RUN setcap 'cap_net_bind_service=+ep' /usr/sbin/sshd

ENTRYPOINT ["tini", "--"]
CMD [ "/usr/sbin/sshd", "-D", "-d", "-e" ]
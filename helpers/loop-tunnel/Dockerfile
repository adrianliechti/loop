FROM alpine:3.14

RUN apk add  --no-cache \
    tini \
    bash \
    python3 \
    openssh \
    sshfs

RUN ssh-keygen -A \
 && passwd -d root \
 && echo -e "NoHostAuthenticationForLocalhost yes\n" > /etc/ssh/ssh_config \
 && echo -e "ListenAddress 127.0.0.1\nPort 22\nAllowTcpForwarding yes\nAllowStreamLocalForwarding yes\nGatewayPorts yes\nPermitEmptyPasswords yes\nPermitRootLogin yes\nSubsystem sftp /usr/lib/ssh/sftp-server\n" > /etc/ssh/sshd_config

EXPOSE 22

ENTRYPOINT ["tini", "--"]
CMD ["/usr/sbin/sshd", "-4", "-D"]
#!/bin/sh

if [ ! -f /run/sshd/ssh_host_rsa_key ]; then
    /usr/bin/ssh-keygen -q -N "" -t rsa -f /run/sshd/ssh_host_rsa_key
fi

if [ ! -f /run/sshd/ssh_host_ecdsa_key ]; then
    /usr/bin/ssh-keygen -q -N "" -t ecdsa -f /run/sshd/ssh_host_ecdsa_key
fi

if [ ! -f /run/sshd/ssh_host_ed25519_key ]; then
    /usr/bin/ssh-keygen -q -N "" -t ed25519 -f /run/sshd/ssh_host_ed25519_key
fi

exec /usr/sbin/sshd -D -d -e
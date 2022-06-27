#!/bin/sh

/usr/bin/ssh-keygen -q -N "" -t rsa -f /run/sshd/ssh_host_rsa_key
/usr/bin/ssh-keygen -q -N "" -t ecdsa -f /run/sshd/ssh_host_ecdsa_key
/usr/bin/ssh-keygen -q -N "" -t ed25519 -f /run/sshd/ssh_host_ed25519_key

/usr/sbin/sshd -D -d -e
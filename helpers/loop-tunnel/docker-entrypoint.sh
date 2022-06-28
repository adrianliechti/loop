#!/bin/bash

USER_ID="$(id -u)"
GROUP_ID="$(id -g)"

cp /etc/passwd /tmp/passwd
sed -i '/loop/d' /tmp/passwd
cat /tmp/passwd > /etc/passwd

echo "loop::${USER_ID}:${GROUP_ID}::/home/loop:/bin/bash" >> /etc/passwd

mkdir -p /home/loop /home/loop/.ssh
chmod 700 /home/loop
chmod 700 /home/loop/.ssh

if [ ! -f /run/sshd/ssh_host_rsa_key ]; then
    /usr/bin/ssh-keygen -q -N "" -t rsa -f /run/sshd/ssh_host_rsa_key
fi

if [ ! -f /run/sshd/ssh_host_ecdsa_key ]; then
    /usr/bin/ssh-keygen -q -N "" -t ecdsa -f /run/sshd/ssh_host_ecdsa_key
fi

if [ ! -f /run/sshd/ssh_host_ed25519_key ]; then
    /usr/bin/ssh-keygen -q -N "" -t ed25519 -f /run/sshd/ssh_host_ed25519_key
fi

exec /usr/sbin/sshd -D -e
FROM nginxinc/nginx-unprivileged:1

COPY --chown=1000:1000 public /usr/share/nginx/html
COPY --chown=1000:1000 nginx.conf /etc/nginx/conf.d/default.conf
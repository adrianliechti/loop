FROM alpine:3.16

RUN apk add --no-cache tini dante-server

COPY --chown=root:root sockd.conf /etc/sockd.conf

EXPOSE 1080

ENTRYPOINT [ "/sbin/tini", "--" ]
CMD [ "/usr/sbin/sockd" ]
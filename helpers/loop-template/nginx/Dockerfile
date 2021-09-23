FROM golang:1.17-alpine3.14

RUN apk add --no-cache ca-certificates bash curl

RUN curl -fsSL "https://github.com/a8m/envsubst/releases/download/v1.2.0/envsubst-Linux-x86_64" -o /usr/local/bin/envsubst && chmod +x /usr/local/bin/envsubst

WORKDIR /src
COPY --chown=root:root app /app
COPY --chown=root:root template /template
COPY --chown=root:root generate /generate

CMD [ "/generate" ]
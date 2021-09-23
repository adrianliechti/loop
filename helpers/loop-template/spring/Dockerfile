FROM maven:3-openjdk-11-slim

RUN curl -fsSL "https://github.com/a8m/envsubst/releases/download/v1.2.0/envsubst-Linux-x86_64" -o /usr/local/bin/envsubst && chmod +x /usr/local/bin/envsubst

WORKDIR /src
COPY --chown=root:root template /template
COPY --chown=root:root generate /generate

CMD [ "/generate" ]
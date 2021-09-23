FROM node:14-buster-slim

RUN apt-get update -qq && apt-get install -qq -y --no-install-recommends \
  curl ca-certificates \
  && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL "https://github.com/a8m/envsubst/releases/download/v1.2.0/envsubst-Linux-x86_64" -o /usr/local/bin/envsubst && chmod +x /usr/local/bin/envsubst

WORKDIR /src
COPY --chown=root:root app /app
COPY --chown=root:root template /template
COPY --chown=root:root generate /generate

CMD [ "/generate" ]
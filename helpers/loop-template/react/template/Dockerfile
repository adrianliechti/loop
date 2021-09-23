FROM node:14-buster-slim as builder

WORKDIR /src
COPY . .

RUN set -- build/index.html && if [ ! -f "$$1" ]; then \
        npm i --no-optional; \
        npm run build; \
    fi


FROM nginxinc/nginx-unprivileged

USER root

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --chown=nginx:nginx --from=builder /src/build /dist

USER 101
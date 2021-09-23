FROM node:14-buster-slim as builder

WORKDIR /src

COPY package*.json .
RUN npm install

COPY . .
RUN npm run build


FROM nginxinc/nginx-unprivileged

USER root

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --chown=nginx:nginx --from=builder /src/build /dist

USER 101
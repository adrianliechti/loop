FROM golang:1.17-alpine3.14 as builder

WORKDIR /src

COPY go.mod .
RUN go mod download

COPY . .
RUN go build -o ${APP_NAME}


FROM alpine:3.14

RUN apk add --no-cache ca-certificates tini

WORKDIR /app

COPY --from=builder /src/${APP_NAME} /app/${APP_NAME}
COPY --from=builder /src/public/ /app/public/

RUN addgroup -g 1000 -S app \
 && adduser -u 1000 -S -G app app \
 && chown -R app:app /app

USER 1000

ENTRYPOINT ["/sbin/tini", "--"]
CMD ["/app/${APP_NAME}"]

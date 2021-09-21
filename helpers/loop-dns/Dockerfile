# builder
FROM golang:1.17-alpine3.14 as builder

WORKDIR /src
COPY . .

RUN go mod download
RUN go build -o dns


# runner
FROM alpine:3.14

RUN apk add --no-cache -X http://dl-cdn.alpinelinux.org/alpine/edge/testing \
    tini \
    coredns

WORKDIR /app/

ENV PATH "/app:${PATH}"

COPY --from=builder /src/dns .
COPY --chown=root:root Corefile .

ENTRYPOINT ["tini", "--"]
CMD [ "dns" ]


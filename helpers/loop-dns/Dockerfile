# builder
FROM golang:1.18-alpine3.15 as builder

WORKDIR /src
COPY . .

RUN go mod download
RUN go build -o dns


# runner
FROM alpine:3.15

RUN apk add --no-cache -X http://dl-cdn.alpinelinux.org/alpine/edge/community \
    tini \
    coredns

WORKDIR /app/

ENV PATH "/app:${PATH}"

COPY --from=builder /src/dns .
COPY --chown=root:root Corefile .

ENTRYPOINT ["tini", "--"]
CMD [ "dns" ]


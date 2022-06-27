# builder
FROM golang:1.18-alpine3.15 as builder

WORKDIR /src
COPY . .

RUN go mod download
RUN go build -o dns


FROM alpine:3.15

RUN apk add --no-cache tini libcap

WORKDIR /app
COPY --from=builder /src/dns .
RUN setcap 'cap_net_bind_service=+ep' /app/dns

ENTRYPOINT ["tini", "--"]
CMD [ "/app/dns" ]
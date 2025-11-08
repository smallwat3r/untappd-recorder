FROM golang:1.24-alpine AS builder

WORKDIR /src

RUN apk add --no-cache vips-dev \
    && rm -rf /var/cache/apk/*

COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/record ./cmd/record

FROM alpine:3.20

RUN apk add --no-cache vips \
    && rm -rf /var/cache/apk/*

COPY --from=builder --chown=nobody:nogroup /out/record /usr/local/bin/record
COPY --chown=nobody:nogroup img/ /img/

USER nobody:nogroup

CMD ["/usr/local/bin/record"]

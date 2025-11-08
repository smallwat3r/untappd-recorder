FROM golang:1.24-alpine AS builder

WORKDIR /src

RUN apk add --no-cache \
      build-base \
      pkgconfig \
      vips-dev \
      git \
    && rm -rf /var/cache/apk/*

ENV CGO_ENABLED=1 \
    GO111MODULE=on

COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN go install github.com/cshum/vipsgen/cmd/vipsgen@latest && \
    cd internal && vipsgen

RUN go build -o /out/record ./cmd/record

FROM golang:1.24-alpine

RUN apk add --no-cache vips \
    && rm -rf /var/cache/apk/*

COPY --from=builder --chown=nobody:nogroup /out/record /usr/local/bin/record
COPY --chown=nobody:nogroup img/ /img/

USER nobody:nogroup
CMD ["/usr/local/bin/record"]

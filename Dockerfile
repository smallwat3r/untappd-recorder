FROM alpine:3.20 AS builder

WORKDIR /src

RUN apk add --no-cache \
      go \
      git \
      build-base \
      pkgconfig \
      vips-dev \
    && rm -rf /var/cache/apk/*

ENV CGO_ENABLED=1 GO111MODULE=on

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go install github.com/cshum/vipsgen/cmd/vipsgen@latest && \
    cd internal && vipsgen

RUN go build -o /out/record ./cmd/record

FROM alpine:3.20

RUN apk add --no-cache vips \
    && rm -rf /var/cache/apk/*

COPY --from=builder --chown=nobody:nogroup /out/record /usr/local/bin/record
COPY --chown=nobody:nogroup img/ /img/

USER nobody:nogroup

CMD ["/usr/local/bin/record"]

# builder and runtime must share the same alpine release so the vips
# runtime package matches the vips-dev headers the binary was built against
FROM golang:1.24-alpine3.22 AS builder

WORKDIR /src

RUN apk add --no-cache \
      build-base \
      pkgconfig \
      vips-dev \
      git

ENV CGO_ENABLED=1

COPY go.mod go.sum ./
RUN go mod download
COPY . .

# vipsgen version is pinned by go.mod
RUN cd internal && go run github.com/cshum/vipsgen/cmd/vipsgen

RUN go build -o /out/record ./cmd/record

FROM alpine:3.22

RUN apk add --no-cache vips

COPY --from=builder --chown=nobody:nogroup /out/record /usr/local/bin/record
COPY --chown=nobody:nogroup img/ /img/

USER nobody:nogroup
CMD ["/usr/local/bin/record"]

# builder and runtime must share the same debian release so the vips runtime
# package matches the vips-dev headers the binary was built against
FROM golang:1.25-bookworm AS builder

WORKDIR /src

RUN apt-get update && apt-get install -y --no-install-recommends \
      libvips-dev \
      pkg-config \
      git \
    && rm -rf /var/lib/apt/lists/*

ENV CGO_ENABLED=1

# ONNX Runtime shared library, used for face detection (BLUR_FACES)
ARG ORT_VERSION=1.29.0
ARG ORT_BASE=https://github.com/microsoft/onnxruntime/releases/download
ADD ${ORT_BASE}/v${ORT_VERSION}/onnxruntime-linux-x64-${ORT_VERSION}.tgz /tmp/ort.tgz
RUN tar xzf /tmp/ort.tgz -C /tmp \
    && cp /tmp/onnxruntime-linux-x64-${ORT_VERSION}/lib/libonnxruntime.so.${ORT_VERSION} \
         /usr/lib/libonnxruntime.so

COPY go.mod go.sum ./
RUN go mod download
COPY . .

# vipsgen version is pinned by go.mod
RUN cd internal && go run github.com/cshum/vipsgen/cmd/vipsgen

RUN go build -o /out/record ./cmd/record

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
      libvips42 \
      ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /usr/lib/libonnxruntime.so /usr/lib/libonnxruntime.so
COPY --from=builder --chown=nobody:nogroup /out/record /usr/local/bin/record
COPY --chown=nobody:nogroup img/ /img/

USER nobody:nogroup
CMD ["/usr/local/bin/record"]

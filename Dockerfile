FROM golang:1.24-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o /out/record ./cmd/record

FROM gcr.io/distroless/base

COPY --from=builder --chown=nonroot:nonroot /out/record /usr/local/bin/record

COPY --chown=nonroot:nonroot img/ /img/

USER nonroot

CMD ["/usr/local/bin/record"]

# syntax=docker/dockerfile:1
FROM golang:1.26-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/nodevitals ./cmd/nodevitals

FROM golang:1.26-bookworm AS builder-gpu
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ARG TARGETARCH=amd64
RUN CGO_ENABLED=1 GOOS=linux GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -tags gpu -o /out/nodevitals ./cmd/nodevitals

FROM gcr.io/distroless/cc-debian12:nonroot AS gpu
# Links the ghcr package to this repository, so the image shows up under the
# repo's Packages and its provenance is traceable from the registry alone.
LABEL org.opencontainers.image.source=https://github.com/KeiaiLab/nodevitals
COPY --from=builder-gpu /out/nodevitals /nodevitals
USER nonroot
ENTRYPOINT ["/nodevitals"]

FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.source=https://github.com/KeiaiLab/nodevitals
COPY --from=builder /out/nodevitals /nodevitals
USER nonroot
ENTRYPOINT ["/nodevitals"]

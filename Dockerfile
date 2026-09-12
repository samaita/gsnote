ARG GO_VERSION=1.25.7
ARG WHISPER_CPP_VERSION=v1.9.4

FROM golang:${GO_VERSION}-bookworm AS gsnote-builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal

ARG VERSION=dev
RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/gsnote ./cmd/bot

FROM debian:bookworm-slim AS whisper-builder

ARG WHISPER_CPP_VERSION
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates cmake curl g++ make \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
RUN curl --fail --location --silent --show-error \
        "https://github.com/ggml-org/whisper.cpp/archive/refs/tags/${WHISPER_CPP_VERSION}.tar.gz" \
        -o /tmp/whisper.cpp.tar.gz \
    && tar --extract --gzip --file /tmp/whisper.cpp.tar.gz --strip-components=1 \
    && cmake -S . -B build \
        -DCMAKE_BUILD_TYPE=Release \
        -DBUILD_SHARED_LIBS=OFF \
        -DWHISPER_BUILD_TESTS=OFF \
        -DWHISPER_BUILD_SERVER=OFF \
    && cmake --build build --config Release --parallel --target whisper-cli

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates ffmpeg libgomp1 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=gsnote-builder /out/gsnote /usr/local/bin/gsnote
COPY --from=whisper-builder /src/build/bin/whisper-cli /usr/local/bin/whisper-cli

# Compose overrides both application paths with bind-mounted container paths.
ENV HOME=/tmp/gsnote \
    GSNOTE_ROOT=/data \
    TRANSCRIBER_BINARY=whisper-cli \
    TRANSCRIBER_MODEL=/model/model.bin

ENTRYPOINT ["gsnote"]

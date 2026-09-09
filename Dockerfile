FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/gsnote ./cmd/bot

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates whisper.cpp && rm -rf /var/lib/apt/lists/*
COPY models/ggml-small-q5_1.bin /models/ggml-small-q5_1.bin
COPY --from=build /out/gsnote /usr/local/bin/gsnote
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/gsnote"]

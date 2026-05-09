# syntax=docker/dockerfile:1.7

FROM golang:1.26-alpine AS build

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
ARG COMMIT=unknown

WORKDIR /src
COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -C plugins/go -trimpath -ldflags="-s -w -X main.version=$VERSION -X main.commit=$COMMIT" \
    -o /out/bu1ld-go-plugin ./cmd/bu1ld-go-plugin

FROM golang:1.26-alpine

RUN apk add --no-cache ca-certificates git openssh-client tzdata

COPY --from=build /out/bu1ld-go-plugin /usr/local/bin/bu1ld-go-plugin

WORKDIR /workspace
ENTRYPOINT ["bu1ld-go-plugin"]

# syntax=docker/dockerfile:1.7

FROM eclipse-temurin:21-jdk AS build

WORKDIR /src
COPY plugins/java ./plugins/java

RUN --mount=type=cache,target=/root/.gradle \
    chmod +x ./plugins/java/gradlew && \
    ./plugins/java/gradlew -p plugins/java assemble --no-daemon --no-configuration-cache --console=plain

FROM debian:bookworm-slim

ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates tzdata && \
    rm -rf /var/lib/apt/lists/*

COPY --from=build /src/plugins/java/build/plugin /opt/bu1ld-java-plugin

RUN chmod +x /opt/bu1ld-java-plugin/bin/bu1ld-java-plugin

WORKDIR /workspace
ENTRYPOINT ["/opt/bu1ld-java-plugin/bin/bu1ld-java-plugin"]

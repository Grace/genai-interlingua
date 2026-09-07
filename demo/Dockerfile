# Builds a Collector containing this repository's processor.
#
# The build context is the repository root, because ocb needs both modules: the
# processor module and the core module it replaces to ../.. Keeping the replace
# directives means this image is built from the working tree rather than from a
# published version, which is the point of a demo.

FROM golang:1.26-bookworm AS build

WORKDIR /src

# The core module has no go.sum: it has no dependencies.
COPY go.mod ./
COPY internal ./internal
COPY cmd ./cmd
COPY processor ./processor
COPY demo/builder-config.yaml ./demo/

RUN go install go.opentelemetry.io/collector/cmd/builder@v0.160.0
RUN builder --config demo/builder-config.yaml

FROM debian:bookworm-slim
RUN useradd --uid 10001 --no-create-home collector
COPY --from=build /src/demo/build/interlingua-collector /usr/local/bin/interlingua-collector
USER 10001
ENTRYPOINT ["/usr/local/bin/interlingua-collector"]
CMD ["--config", "/etc/otelcol/config.yaml"]

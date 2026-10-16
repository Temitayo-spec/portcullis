# Two programs in one small image: the server (the default) and gatectl.
ARG GO_VERSION=1.27.2

# Compiled on the builder's own platform for each target: no emulation.
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN export CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
 && go build -trimpath -ldflags="-s -w" -o /out/bin/portcullis ./cmd/portcullis \
 && go build -trimpath -ldflags="-s -w" -o /out/bin/gatectl ./cmd/gatectl \
 && mkdir -p /out/data /out/state

# No operating system, just the two programs, running as user 65532.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/bin/ /usr/local/bin/
COPY --from=build --chown=65532:65532 /out/data /data
COPY --from=build --chown=65532:65532 /out/state /state
VOLUME ["/data", "/state"]
EXPOSE 8080
ENTRYPOINT ["portcullis"]

# syntax=docker/dockerfile:1
# Cairn: one static binary that also serves the built UI.
# Front and binary are built on the build machine (cross-compiled, no emulation).

FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS server
ARG TARGETOS TARGETARCH VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web/embed.go ./web/
COPY --from=web /web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/cairn ./cmd/cairn
# The image has no shell: the data folder is made here.
RUN mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /out/cairn /cairn
# The database (cairn.db), writable by nonroot (65532).
COPY --from=server --chown=65532:65532 --chmod=700 /out/data /data
ENV CAIRN_ADDR=:8080 CAIRN_DATA=/data
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s CMD ["/cairn", "health"]
USER nonroot:nonroot
ENTRYPOINT ["/cairn"]

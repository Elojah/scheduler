# Pure Go, SQLite included: the builder runs on the build platform and cross-compiles to the target's, such as
# docker buildx build --platform linux/arm64.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

RUN apk add --no-cache \
	ca-certificates \
	git \
	make

WORKDIR /scheduler
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS TARGETARCH
RUN env GOOS=$TARGETOS GOARCH=$TARGETARCH make build
# scratch has no mkdir: the database's directory, whose owner a new volume takes.
RUN mkdir /data

FROM scratch AS runner

# For a tls:// broker. TZIDs need no zoneinfo: the binary embeds time/tzdata.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder --chown=65532:65532 /data /data
WORKDIR /app
COPY --from=builder --chmod=755 /scheduler/bin/scheduler .
USER 65532:65532
ENTRYPOINT ["/app/scheduler", "/app/docker.json"]

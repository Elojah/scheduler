# The client, built once whatever the target: it is static files.
FROM --platform=$BUILDPLATFORM node:24-alpine AS client

WORKDIR /client
COPY cmd/client/package.json cmd/client/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY cmd/client/ ./
RUN npm run build

# Pure Go: the builder runs on the build platform and cross-compiles to the target's, such as
# docker buildx build --platform linux/arm64.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

RUN apk add --no-cache \
	git \
	make

WORKDIR /scheduler
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS TARGETARCH
RUN env GOOS=$TARGETOS GOARCH=$TARGETARCH make web_client

FROM scratch AS runner

# TZIDs need no zoneinfo: the binary embeds time/tzdata.
WORKDIR /app
COPY --from=client /client/dist ./static
COPY --from=builder --chmod=755 /scheduler/bin/web_client .
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/app/web_client", "/app/docker.json"]

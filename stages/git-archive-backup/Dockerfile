# Build stage: compile the pure-Go binary with CGO disabled for a static binary.
FROM golang:1.27-alpine AS build

WORKDIR /src

# Copy go.mod/go.sum and the vendored dependencies first for layer caching.
# The vendor directory makes the build hermetic and removes the need to resolve
# the sibling manifest-lib module over the network.
COPY go.mod go.sum ./
COPY vendor ./vendor
COPY . ./

RUN CGO_ENABLED=0 go build -mod=vendor -o /out/app -ldflags="-s -w" ./

# Test stage: run unit tests inside the build environment.
FROM build AS test
RUN CGO_ENABLED=0 go test -mod=vendor -short ./...

# Runtime stage: distroless static — no shell, no package manager, minimal surface.
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=build /out/app /app/app

# Distroless "nonroot" already runs as an unprivileged user.
ENTRYPOINT ["/app/app"]

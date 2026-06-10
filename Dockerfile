# This Dockerfile is only used as an example to build the application locally with Docker.
# Production images are built with https://ko.build/

FROM golang:1.24 AS build

ARG APP_VERSION=unknown
ARG COMMIT_HASH=unknown

WORKDIR /app

# Install dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build application
COPY . ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags " \
    -s -w \
    -X github.com/peng-mj/scproxy/internal/version.appVersion=${APP_VERSION} \
    -X github.com/peng-mj/scproxy/internal/version.commitHash=${COMMIT_HASH}" \
    -o bin/scproxy

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /app/bin/scproxy .

ENTRYPOINT ["/scproxy"]

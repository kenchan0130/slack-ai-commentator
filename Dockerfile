# Build on the runner's native platform and cross-compile, instead of emulating the target platform.
FROM --platform=$BUILDPLATFORM golang:1.27.3 AS builder

ARG TARGETOS
ARG TARGETARCH
ENV CGO_ENABLED=0
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w" -o /out/slack-ai-commentator .

FROM alpine:3.24.3
COPY --from=builder /out/slack-ai-commentator /app/slack-ai-commentator

# The rules are stored here. A named volume mounted here takes over this ownership.
RUN mkdir -p /opt/data && chown 65534:65534 /opt/data

# nobody:nobody. A numeric ID lets Kubernetes verify runAsNonRoot.
USER 65534:65534

ENTRYPOINT ["/app/slack-ai-commentator"]

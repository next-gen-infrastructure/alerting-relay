FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /alerting-relay ./cmd/alerting-relay

FROM gcr.io/distroless/static-debian12
COPY --from=build /alerting-relay /alerting-relay
USER nonroot:nonroot
ENTRYPOINT ["/alerting-relay"]

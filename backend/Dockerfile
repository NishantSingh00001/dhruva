# Build natively on the builder's CPU and cross-compile for the target, so one image serves amd64 and arm64.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /geofence .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /geofence /geofence
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/geofence"]

# Build stage: compile one static binary.
FROM golang:1.22 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /gopherdis ./cmd/gopherdis

# Run stage: just the binary on a minimal image.
FROM gcr.io/distroless/static-debian12
COPY --from=build /gopherdis /gopherdis
VOLUME /data
EXPOSE 6379
# Inside a container, listen on every interface (Docker's port mapping
# decides who can reach it) and keep the save file in the /data volume.
ENTRYPOINT ["/gopherdis", "--bind", "0.0.0.0", "--dir", "/data"]

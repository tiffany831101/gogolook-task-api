# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.22-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/task-api ./cmd/server

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/task-api /task-api

ENV GIN_MODE=release \
    PORT=8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/task-api"]

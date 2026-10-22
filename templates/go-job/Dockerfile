# Stage 1: build a static binary
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bin/app .

# Stage 2: distroless static runtime, runs as non-root
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /bin/app /app
EXPOSE 8080
ENTRYPOINT ["/app"]

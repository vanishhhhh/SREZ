FROM golang:1.27.2-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/server ./cmd/app
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/seed ./cmd/seed

FROM scratch

COPY --from=build /out/server /server
COPY --from=build /out/seed /seed

ENV HTTP_ADDR=:8080
ENV GIN_MODE=release

EXPOSE 8080
USER 65532:65532

ENTRYPOINT ["/server"]

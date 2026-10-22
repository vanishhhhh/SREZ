FROM golang:1.27.2-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/server ./cmd/app

FROM scratch

COPY --from=build /out/server /server

ENV HTTP_ADDR=:8080
ENV GIN_MODE=release

EXPOSE 8080
USER 65532:65532

ENTRYPOINT ["/server"]

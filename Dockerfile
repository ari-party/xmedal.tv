FROM golang:1.22-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY src ./src

ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN go build -o /app/bin/xmedaltv ./src

FROM gcr.io/distroless/base-debian12 AS runtime

WORKDIR /app

COPY --from=build /app/bin/xmedaltv ./xmedaltv

ENTRYPOINT ["/app/xmedaltv"]

EXPOSE 3000

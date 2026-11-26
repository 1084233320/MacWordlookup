FROM golang:1.22-alpine AS build
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /app/server .

FROM alpine:3.20
WORKDIR /app
COPY --from=build /app/server ./
ENV DATA_DIR=/tmp/mwl-data
EXPOSE 8080
CMD ["./server"]

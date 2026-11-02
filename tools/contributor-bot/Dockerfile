FROM golang:1.27-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bot .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 bot && mkdir /data && chown bot /data
COPY --from=build /bot /bot
USER bot
ENV LINKS_FILE=/data/links.json
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["/bot"]

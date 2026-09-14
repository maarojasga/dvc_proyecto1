FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/worker ./cmd/worker

FROM alpine:3.20 AS api
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/api /usr/local/bin/api
EXPOSE 8080
ENTRYPOINT ["api"]

FROM alpine:3.20 AS worker
# ffmpeg transcodifica a HLS; libreoffice convierte las presentaciones a PDF.
#
# Se instala solo el modulo de presentaciones (libreoffice-impress) y no la
# suite entera: el worker no abre textos ni hojas de calculo, y arrastrarlos
# multiplicaria el tamano de la imagen sin usarse.
#
# Las fuentes no son opcionales. Sin ellas LibreOffice convierte igual, pero
# sustituye cada tipografia por un sustituto y el PDF sale con los saltos de
# linea descolocados respecto a lo que vio el profesor.
RUN apk add --no-cache \
        ca-certificates \
        ffmpeg \
        libreoffice-impress \
        openjdk17-jre-headless \
        font-noto \
        font-noto-cjk \
        ttf-dejavu \
        ttf-liberation
COPY --from=builder /out/worker /usr/local/bin/worker
# LibreOffice escribe su perfil bajo HOME. Sin un HOME escribible arranca y
# falla al crear el perfil, con un error que no menciona el permiso.
ENV HOME=/tmp
ENTRYPOINT ["worker"]

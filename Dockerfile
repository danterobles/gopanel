FROM node:22.23.2-bookworm-slim@sha256:48e4b67d85f87bd551df43704e24d252f56cc5f8e9718841aace50f19948f0f9 AS assets
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY scripts/ scripts/
COPY web/ web/
COPY internal/ui/ internal/ui/
RUN npm run build

FROM golang:1.26.10-bookworm@sha256:632dff7351799a5a9954a48dc06670ade72ee44e8c351cdc0e5a57af2fea21af AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=assets /src/web/static/ web/static/
RUN go run github.com/a-h/templ/cmd/templ@v0.3.1070 generate
RUN CGO_ENABLED=0 go build -trimpath -o /out/gopanel ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/gopanel /gopanel
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/gopanel"]

# GoPanel Pilot

SaaS administrativo multiempresa en construcción. Etapas 0 y 1: especificaciones conciliadas y bootstrap ejecutable. Todavía no hay autenticación, recursos de negocio, esquema ni seeds.

## Arranque con Docker Compose

Requisitos: Docker Engine/Desktop activo, Compose v2 o superior y puertos locales 8080/3306 disponibles. No requiere Go ni Node en el host.

```sh
cp .env.example .env
```

Completar `DB_PASSWORD` y `MYSQL_ROOT_PASSWORD` en `.env` con valores aleatorios distintos. Por ejemplo, generar cada valor con `openssl rand -hex 24`. `.env` está excluido de Git; nunca reutilizar estas credenciales en producción.

```sh
make dev
```

Visitar [GoPanel local](http://localhost:8080). Compose compila Templ/Tailwind, espera un MySQL saludable y arranca una sola instancia Go con usuario no root. MySQL conserva datos en el volumen `mysql_data`; no se ejecutan migraciones al arrancar. Los puertos se publican solo en loopback. Si el puerto MySQL está ocupado, cambiar `DB_PORT` en `.env`; `APP_PORT` cambia el puerto HTTP publicado.

```sh
curl -i http://localhost:8080/health/live
curl -i http://localhost:8080/health/ready
docker compose logs app
docker compose down
```

`down` conserva el volumen. Para reiniciar después, usar `make dev`. Cambiar las contraseñas de `.env` no cambia las de un volumen MySQL existente: hay que rotarlas en MySQL y sincronizar el entorno. No eliminar un volumen con datos que se necesiten.

## Desarrollo sin contenerizar Go

Versiones fijadas en `.go-version`, `.node-version`, `go.mod`, `tools/versions.mk` y `package-lock.json`. Go 1.21+ puede descargar el toolchain 1.26.10 con `GOTOOLCHAIN`; Makefile lo selecciona explícitamente. Node 22.23.2 es la versión validada. Se necesita red la primera vez para descargar módulos, toolchain y paquetes npm.

```sh
make generate
make lint test build
docker compose up -d mysql --wait
set -a
. ./.env
set +a
./bin/gopanel
```

El binario lee variables de entorno, no carga `.env`. En el host, `DB_HOST=127.0.0.1`; Compose lo reemplaza por `mysql` dentro de app. Detener con Ctrl+C o SIGTERM: el servidor drena solicitudes activas hasta el plazo configurado y después cierra el pool. La compilación incluye los assets en el binario; no depende del directorio desde el cual se ejecute.

## Comandos

| Comando | Comportamiento actual |
|---|---|
| `make dev` | Compilar y arrancar Compose en segundo plano, esperando health checks |
| `make generate` | Instalar desde lockfile, compilar Tailwind, copiar HTMX/SSE locales y generar Templ |
| `make fmt` | Formatear Go |
| `make lint` | Comprobar gofmt y ejecutar `go vet ./...` |
| `make test` | Pruebas unitarias/HTTP y de conectividad fallida, sin MySQL externo |
| `make test-integration` | MySQL real; requiere variables DB exportadas y servidor disponible; falla si no hay conexión |
| `make build` | Compilar `bin/gopanel` sin CGO |
| `make smoke` | Con Compose activo en puerto 8080: probar assets, health, caída/recuperación de MySQL y apagado/reinicio Go; interrumpe esos servicios locales temporalmente |
| `make migrate-up`, `make migrate-down`, `make seed` | Salen con error explícito hasta etapa 2; no simulan trabajo |

Prueba de carreras: `GOTOOLCHAIN=go1.26.10 go test -race ./...`. CI ejecuta generación, detección de diferencias, formato, vet, pruebas, detector de carreras, integración MySQL, build de imagen y smoke de Compose. Una ejecución local no prueba por sí sola que el workflow remoto ya haya pasado.

## Configuración

| Variable | Default / regla |
|---|---|
| `APP_ENV` | `development`; acepta también `test`/`production` |
| `HTTP_ADDR` | `:8080`; host:puerto válido, distinto de cero |
| `DB_HOST`, `DB_PORT` | `127.0.0.1`, `3306` |
| `DB_NAME`, `DB_USER` | `gopanel`, `gopanel` |
| `DB_PASSWORD` | Obligatoria; no se registra |
| `DB_TLS` | `false` en desarrollo; producción exige `true` con validación del certificado |
| `HEALTH_TIMEOUT` | `2s`; positivo y como máximo `1m` |
| `SHUTDOWN_TIMEOUT` | `10s`; positivo y como máximo `1m` |
| `MYSQL_ROOT_PASSWORD` | Solo inicialización Compose; app usa usuario de privilegios limitados a su base |
| `APP_PORT` | Solo Compose, `8080` |

Pool: máximo 10 conexiones abiertas y 5 ociosas, vida máxima 3 minutos, conexión ociosa máxima 1 minuto; timeouts de conexión/lectura/escritura MySQL de 3 segundos. Cada conexión configura UTC, `utf8mb4`, modo estricto y parseo de fechas. HTTP tiene timeout de cabeceras de 5 segundos y lectura/escritura de 15 segundos. SSE revisará este último antes de incorporarse (D-08).

## Contrato operativo de esta etapa

- GET `/health/live`: 200 mientras el proceso sirve HTTP.
- GET `/health/ready`: 200 si puede hacer ping a MySQL dentro del plazo; 503 genérico ante fallo/timeout. No certifica migraciones ni esquema. Ambos endpoints responden JSON no cacheable y no mutan datos.
- Logs JSON: request ID generado por el servidor, patrón de ruta, método, estado y latencia. No registrar query strings, cuerpos, cabeceras, contraseñas ni valores de panic.
- GET `/`: página mínima de verificación Templ/Tailwind; no es el dashboard del MVP.
- HTMX y SSE se distribuyen localmente; su funcionalidad se habilitará en etapas posteriores. No hay CDN, caché, Redis ni endpoints comerciales sin autorización.

Para probar manualmente una caída de DB en el entorno local, detener `mysql` y consultar ambos health checks: live debe seguir en 200 y ready pasar a 503. Volver a arrancar MySQL debe recuperar readiness sin reiniciar Go.

Evidencia local y límites: [validación de etapa 1](docs/validacion-etapa-1.md).

## Especificaciones y seguimiento

Leer [PRD](PRD.md), [arquitectura](ARQUITECTURA.md), [datos](DATA.md) y [AGENTS](AGENTS.md). Consultar [plan](docs/plan-de-implementacion.md), [aceptación](docs/matriz-de-aceptacion.md), [decisiones pendientes](docs/decisiones-pendientes.md) y [ADR de herramientas](docs/adr/0002-herramientas-y-bootstrap.md).

Siguiente etapa: esquema MySQL, migraciones versionadas, repositorios y seeds. Antes de exponer el servicio públicamente se debe cerrar D-09 sobre TLS/proxy, métricas y despliegue; Compose aquí configura desarrollo local.

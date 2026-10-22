# Validación de la etapa 1

Fecha: 2026-10-21. Entorno: macOS ARM64, Go toolchain 1.26.10, Node 22.23.2, Docker Desktop Engine 29.6.1 y Compose 5.3.0. MySQL 8.0.45 en contenedor. No es una medición de rendimiento ni una aceptación del MVP completo.

## Entregables

- Servidor Go/Chi con configuración validada por entorno, logs JSON, recuperación de panic sin registrar su contenido y apagado ordenado.
- Pool MySQL con ajustes por conexión y health checks separados para proceso/conectividad.
- Página mínima Templ y assets Tailwind/HTMX/SSE locales, embebidos en el binario, con licencias de distribución.
- Toolchain, módulos y paquetes fijados; imágenes por digest. Dockerfile multi-stage y runtime sin root ni shell.
- Compose local, Makefile, CI, README y ADR-0002. Sin migraciones de negocio ni seeds.

## Evidencia ejecutada

| Verificación | Resultado |
|---|---|
| `make generate` | Correcto: Tailwind 4.3.3, assets locales y Templ 0.3.1070. Hashes de archivos generados previos idénticos tras regenerar. |
| `make fmt` / `make lint` | gofmt y `go vet ./...` correctos. |
| `make test` | Configuración válida/inválida, health correcto/fallo/timeout, logs correlacionados sin secretos, assets, panic y apagado con solicitud activa/plazo agotado aprobados. |
| `GOTOOLCHAIN=go1.26.10 go test -race ./...` | Aprobado. |
| `make build` | Binario sin CGO compilado. |
| `GOTOOLCHAIN=go1.26.10 go mod verify` | Módulos verificados. |
| `make test-integration` con variables exportadas de `.env` | Aprobado contra MySQL real: dos conexiones simultáneas con UTC, modo estricto, utf8mb4 y parseo DATETIME(6) con microsegundos. |
| Compose config/build/up con espera de health | Imagen compilada y servicios saludables. El puerto 3306 existente se preservó; el `.env` local usa 13306 para MySQL y 8080 para HTTP. |
| `make smoke` | Página y assets 200; caída MySQL: live=200/ready=503; recuperación sin recrear app; SIGTERM termina con código 0 y posterior arranque saludable. |
| Logs del contenedor Go | 22 registros inspeccionados, JSON válidos y sin contraseñas locales. |
| Secretos | `.env` generado aleatoriamente y excluido de Git; no se incluyó en el contexto Docker. |

La integración detectó y permitió corregir el charset: se configura con `mysql.Charset`, no como variable SQL genérica. Esta comprobación usa el driver real, no un mock.

## Límites y siguiente etapa

- El workflow está preparado; su ejecución en GitHub no se ha realizado desde esta entrega local.
- Readiness todavía no valida tablas ni versión del esquema. Etapa 2 incorporará migraciones, SQL tipado y seeds, con rollback documentado.
- `make migrate-up`, `make migrate-down` y `make seed` fallan explícitamente hasta etapa 2.
- No hay rutas comerciales, sesiones, autorización ni consultas tenant todavía; sus pruebas siguen planificadas en la matriz de aceptación. US-09 tiene evidencia parcial de bootstrap; observabilidad completa y despliegue siguen pendientes.
- Timeout de escritura HTTP de 15 s debe adaptarse al contrato SSE en etapa 7. TLS/proxy y operación pública quedan en D-09.

El entorno local puede detenerse con `docker compose down`, conservando el volumen de MySQL. La siguiente tarea es la etapa 2 del plan.

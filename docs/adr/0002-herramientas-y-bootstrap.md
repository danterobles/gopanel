# ADR-0002 — Herramientas fijadas y bootstrap

- Estado: aceptado para la etapa 1.
- Fecha: 2026-10-21.

## Contexto y decisión

D-01 exige fijar herramientas compatibles antes de implementar. Se consultaron los registros oficiales Go/npm y documentación de Chi, Templ, Tailwind y el driver MySQL. El Go global del entorno es 1.21.7; el proyecto usa el mecanismo de toolchains de Go sin reemplazar esa instalación.

| Elemento | Versión fijada | Ubicación |
|---|---|---|
| Go | 1.26.10 | `go.mod`, `.go-version`, Makefile y Dockerfile |
| Chi | 5.5.0 | `go.mod`/`go.sum` |
| Driver MySQL | 1.10.1 | `go.mod`/`go.sum` |
| Templ CLI y runtime | 0.3.1070 | Makefile y `go.mod` |
| sqlc | 1.31.1 | `tools/versions.mk`; uso y validación MySQL en etapa 2 |
| golang-migrate | 4.20.1 | `tools/versions.mk`; ejecución con driver MySQL en etapa 2 |
| Node | 22.23.2 | `.node-version`, Dockerfile y CI |
| Tailwind y CLI | 4.3.3 | `package.json`/`package-lock.json` |
| HTMX / extensión SSE | 2.0.11 / 2.2.4 | `package.json`/`package-lock.json` |
| MySQL | 8.0.45 | Compose y CI; versión y digest fijados |

Las imágenes Go, Node y runtime distroless también se fijan por digest en Dockerfile; sus actualizaciones son deliberadas. Los assets generados se versionan para permitir compilar y probar desde un clon sin Node; `make generate` debe reproducirlos.

No se incorporan frameworks adicionales, gestores de configuración, mocks SQL ni librerías de logging. `slog`, `testing`, `httptest` y `database/sql` cubren el bootstrap. npm solo compila/copía assets; no se necesita Node en runtime. HTMX/SSE se empaquetan localmente y se activarán cuando exista su funcionalidad.

## Migraciones: comparación

- Goose permite SQL con anotaciones up/down y migraciones Go, pero estas últimas no son necesarias aquí.
- golang-migrate permite SQL up/down en archivos separados y compilar el CLI solo con MySQL. Se elige por ese contrato explícito y alcance reducido.
- No se ejecutan migraciones al arrancar el servidor. Los targets fallan con explicación hasta etapa 2; tampoco se simula un seed exitoso.

## Contratos operativos iniciales

- Configuración solo por entorno; `.env` se ignora en Git y Compose lo lee. El binario no carga archivos ni imprime DSN/credenciales.
- Pool MySQL con UTC por conexión, `parseTime`, modo estricto, límites de conexiones y timeouts de red. Abrir el pool no exige disponibilidad inmediata: readiness consulta MySQL con deadline; liveness comprueba el proceso.
- Readiness significa conectividad a MySQL en esta etapa; no certifica esquema todavía. Respuestas JSON genéricas, sin secretos, no cacheables. Solo GET para health.
- Logs JSON con ID generado por el servidor, método, patrón de ruta, estado y latencia; no registrar URL/query, cabeceras, cuerpo ni valores de panic. IDs enviados por el cliente no determinan el ID interno.
- Apagado por SIGINT/SIGTERM: dejar de aceptar conexiones, esperar solicitudes hasta el plazo, cerrar conexiones forzosamente si vence y finalmente cerrar el pool.
- Compose publica app/MySQL solo en loopback para desarrollo, usa volumen persistente y contraseñas obligatorias por entorno. El despliegue público y TLS/proxy siguen en D-09.
- Timeout de escritura HTTP inicial de 15 s; SSE requiere revisión por ruta antes de etapa 7 según D-08. No se promete streaming en este bootstrap.

## Consecuencias y verificación

La página raíz es una comprobación del renderizado/compilación local, no un dashboard. No incluye acceso a datos de negocio. Las pruebas cubren configuración, health, timeout/fallo DB, logs sin secretos, panic y apagado con solicitudes activas. Una prueba de integración opcional usa MySQL real; CI y Compose deben ejercitarla. No hay migraciones en esta etapa. D-01 queda resuelta para selección/fijación; compatibilidad del SQL generado y migraciones se verifica en etapa 2.

Fuentes: [Chi](https://github.com/go-chi/docs), [Templ](https://templ.guide/), [driver MySQL](https://github.com/go-sql-driver/mysql), [Tailwind CLI](https://tailwindcss.com/docs/installation/tailwind-cli), [golang-migrate](https://github.com/golang-migrate/migrate), [Goose](https://github.com/pressly/goose).

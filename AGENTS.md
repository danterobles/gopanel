# AGENTS.md — GoPanel Pilot

## Misión
Implementar un SaaS administrativo multiempresa de referencia usando Go, Chi, Templ, HTMX, Tailwind CSS, MySQL 8. El piloto validará una futura biblioteca declarativa tipo Filament; **no** se construirá un framework genérico completo en esta etapa.

## Fuente de verdad
Leer, en este orden: `PRD.md`, `ARQUITECTURA.md`, `DATA.md`, luego este archivo. Ante contradicciones, detenerse y documentar una decisión en `docs/adr/` antes de cambiar contratos. No inventar requisitos.

## Modo de trabajo para Codex
1. Inspeccionar repositorio, herramientas y pruebas existentes antes de modificar.
2. Proponer un plan corto con archivos, migraciones y pruebas para cada cambio no trivial.
3. Implementar en incrementos verticales pequeños y verificables; no mezclar refactors ajenos.
4. Escribir pruebas para aislamiento de tenants, permisos, validación, SQL y rutas HTMX.
5. Ejecutar los comandos disponibles de formato, generación, pruebas y análisis estático; informar lo que no pudo ejecutarse.
6. No modificar migraciones ya aplicadas; agregar nuevas migraciones reversibles.
7. Mantener un registro de decisiones arquitectónicas en `docs/adr/`.
8. No introducir dependencias, servicios externos ni generadores adicionales sin justificar su necesidad.

## Stack obligatorio
- Go (versión estable compatible con dependencias), `net/http`, `github.com/go-chi/chi/v5`.
- `templ` para vistas y componentes; HTMX para interacciones parciales.
- Tailwind CSS compilado a assets locales, sin CDN en producción.
- MySQL 8.0+ (InnoDB) como fuente de verdad; `database/sql` + `go-sql-driver/mysql` y SQL explícito, preferentemente `sqlc` configurado para MySQL.
- **Redis y caché de aplicación fuera de alcance.** Sesiones persistidas en MySQL; eventos SSE mediante broker en memoria, solo una instancia Go en la POC.
- SSE para eventos unidireccionales; no introducir WebSockets sin un caso justificado.
- Migraciones versionadas con herramienta elegida y fijada en el repositorio.
- Docker Compose para desarrollo local y despliegue reproducible.

## Convenciones
- Monolito modular, no microservicios.
- Capas por dominio: HTTP handlers -> services -> repositories -> MySQL 8.
- `context.Context` en operaciones de I/O; errores envueltos con contexto; logs estructurados sin secretos.
- Vistas `templ` sin lógica de negocio ni consultas SQL.
- HTML completo en navegación normal; fragmentos HTML para peticiones HTMX; respuestas HTTP semánticas.
- JavaScript mínimo: HTMX, extensión SSE y, si se justifica, biblioteca de gráficas.
- No utilizar GORM en MVP. No construir abstracciones genéricas prematuras.
- IDs UUID v4 generados en Go y almacenados como `CHAR(36)`; `DATETIME(6)` en UTC; dinero en centavos (`BIGINT`).
- Toda consulta a datos de negocio debe quedar acotada al tenant activo desde el repositorio/servicio.

## Límites de la POC
- Ejecutar una sola instancia Go: SSE en memoria no se comparte entre réplicas.
- Sin Redis, Memcached, caché de consultas ni caché de aplicación; las consultas se hacen directamente a MySQL.
- Sesiones opacas persistidas en tabla `sessions` de MySQL con expiración y revocación.
- MySQL 8: no utilizar sintaxis de PostgreSQL (`RETURNING`, `ILIKE`, `JSONB`, índices parciales o `$1`).

## Seguridad obligatoria
- Autenticación con sesiones server-side, cookie `HttpOnly`, `Secure` en producción y `SameSite=Lax` o más restrictivo según flujo.
- Contraseñas con Argon2id y parámetros revisables; nunca texto plano.
- CSRF en todas las mutaciones, incluidas las iniciadas por HTMX.
- Verificar autorización por acción y por recurso en servidor; ocultar botones no reemplaza autorización.
- Nunca aceptar `tenant_id`, `role` o `user_id` desde formularios para definir el ámbito de seguridad.
- Tenant activo derivado de membresía verificada y sesión; prevenir IDOR.
- Consultas parametrizadas; paginación con límites; allowlists para columnas ordenables y filtrables.
- Secretos solo por variables de entorno o gestor de secretos; `.env` fuera de git.
- Auditoría de operaciones sensibles, con actor, tenant, acción y resultado, sin almacenar contraseñas ni tokens.

## Contrato de funcionalidades MVP
Autenticación, selección de empresa, membresías y RBAC, CRUD de clientes y productos, dashboard, búsqueda/filtros/orden/paginación, SSE para avisos y cambios de métricas, auditoría básica y pruebas multi-tenant.

## Orden de implementación
1. Bootstrap, configuración, Compose, CI, health checks.
2. Migraciones, conexión MySQL 8, repositorios y seeds.
3. Login/logout, sesiones, CSRF y selección de tenant.
4. RBAC y políticas de recursos.
5. Layout y componentes Templ/HTMX.
6. CRUD de clientes y productos con filtros y paginación.
7. Dashboard y SSE con broker en memoria (sin Redis ni caché).
8. Pruebas E2E, observabilidad, documentación y benchmark comparativo.

## Definition of Done por historia
- Criterios de aceptación del PRD cumplidos.
- Pruebas automatizadas positivas, negativas y entre tenants.
- Migración y rollback documentados cuando corresponda.
- Interfaz accesible por teclado, estados vacíos/error/carga y validación visible.
- `go test ./...`, `go vet ./...`, `go fmt`/`gofmt` y generación Templ exitosos cuando aplique.
- Documentación y ejemplo de uso actualizados.

## Comandos esperados (crear Makefile)
`make dev`, `make generate`, `make migrate-up`, `make migrate-down`, `make test`, `make lint`, `make build`, `make seed`.

## Restricciones de alcance
No implementar editor visual de recursos, ORM propio, generador CLI, sistema de plugins, facturación, suscripciones de pago, permisos configurables por campo, offline-first ni microservicios en MVP.

## Formato de reporte de Codex
Al terminar cada tarea: resumen, archivos cambiados, migraciones, pruebas ejecutadas/resultados, riesgos pendientes y siguiente paso recomendado.

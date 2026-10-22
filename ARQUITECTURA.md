# ARQUITECTURA — GoPanel Pilot

**Versión:** 1.0 · **Estilo:** monolito modular server-rendered

## 1. Principios
1. Seguridad y aislamiento por tenant desde la primera migración.
2. Renderizado HTML en servidor con Templ; HTMX solo orquesta interacciones.
3. Contratos explícitos entre handlers, servicios y repositorios.
4. MySQL 8 (InnoDB) como única fuente de verdad; sin Redis ni caché de aplicación.
5. Componentes reutilizables extraídos de necesidades reales, no un framework especulativo.

## 2. Diagrama
```text
Browser (HTML + HTMX + Tailwind)
   | HTTP/HTML, POST/CSRF, SSE
   v
Reverse proxy (TLS, rate limits)
   |
   v
Go / Chi
   |-- middleware: request-id, recovery, logging, session, CSRF
   |-- tenant resolver + RBAC policies
   |-- handlers --> Templ pages/fragments
   |-- services --> repositories --> MySQL 8
   |-- events --> broker en memoria --> SSE subscribers
   `-- health / metrics
```

## 3. Dependencias y decisiones
| Capa | Selección | Motivo |
|---|---|---|
| HTTP | Go `net/http` + Chi | Routing/middleware sin framework pesado |
| Vistas | Templ | Componentes HTML compilados y tipados |
| Interacciones | HTMX | Fragmentos, filtros, modales y navegación parcial |
| CSS | Tailwind CSS | Diseño consistente, compilado localmente |
| SQL | `database/sql` + `go-sql-driver/mysql` + sqlc (MySQL) | SQL visible y métodos tipados |
| Persistencia | MySQL 8 / InnoDB | Transacciones, constraints, índices |
| Sesiones | Tabla MySQL `sessions` | Persistencia y revocación sin servicio adicional |
| Tiempo real | SSE + broker en memoria | Eventos unidireccionales para una sola instancia |
| Gráficas | Librería JS pequeña (opcional) | Visualización, no lógica de negocio |
| Migraciones | Goose o golang-migrate (fijar en ADR) | Migraciones SQL versionadas |

## 4. Organización sugerida
```text
cmd/server/main.go
internal/config/
internal/platform/http/
internal/platform/auth/
internal/platform/tenancy/
internal/platform/rbac/
internal/platform/events/
internal/platform/database/
internal/modules/tenants/{handler,service,repository}/
internal/modules/members/{handler,service,repository}/
internal/modules/customers/{handler,service,repository}/
internal/modules/products/{handler,service,repository}/
internal/modules/dashboard/{handler,service,repository}/
internal/ui/components/*.templ
internal/ui/layouts/*.templ
internal/ui/pages/*.templ
internal/ui/fragments/*.templ
internal/db/queries/*.sql
internal/db/generated/
migrations/
web/static/
web/styles/input.css
tests/integration/
tests/e2e/
docs/adr/
compose.yaml
Makefile
```

## 5. Flujos HTTP
### Página completa
`GET /app/customers` -> middleware sesión/tenant/RBAC -> servicio -> repositorio -> Templ layout + tabla -> HTML.

### Fragmento HTMX
`GET /app/customers/table?search=&status=&sort=&page=` -> misma autorización -> tabla Templ parcial. La respuesta debe mantener filtros, orden y paginación. Usar `HX-Request` como indicio de representación, **no** como autorización.

### Mutación
`POST /app/customers` -> sesión + CSRF + tenant + permiso -> parseo/validación -> servicio/transacción -> auditoría -> respuesta 303 (HTML normal) o fragmento/headers HTMX según contrato. Nunca confiar en campos de control del cliente.

### Tiempo real
Mutación confirmada -> publicar evento `{type, tenant_id, resource, version}` -> broker en memoria -> SSE `GET /app/events` autenticado y vinculado a tenant -> navegador recibe evento -> HTMX vuelve a solicitar widget/tabla. No incluir PII ni payload completo en evento.

## 6. Rutas de referencia
| Método | Ruta | Permiso |
|---|---|---|
| GET/POST | `/login` | Público con rate limit |
| POST | `/logout` | Autenticado + CSRF |
| GET | `/app/tenants/select` | Autenticado |
| POST | `/app/tenants/activate` | Miembro activo + CSRF |
| GET | `/app` | `dashboard.read` |
| GET | `/app/customers` | `customers.read` |
| GET | `/app/customers/table` | `customers.read` |
| GET/POST | `/app/customers/new` | `customers.create` |
| GET/POST | `/app/customers/{id}/edit` | `customers.update` |
| POST | `/app/customers/{id}/delete` | `customers.delete` |
| GET | `/app/products` | `products.read` |
| GET | `/app/products/table` | `products.read` |
| GET/POST | `/app/products/new` | `products.create` |
| GET/POST | `/app/products/{id}/edit` | `products.update` |
| POST | `/app/products/{id}/delete` | `products.delete` |
| GET | `/app/events` | Autenticado + tenant activo |
| GET | `/health/live` | Operación |
| GET | `/health/ready` | Operación, sin detalles internos |

## 7. Seguridad y multi-tenancy
- `users` global; `tenant_memberships` vincula usuarios, tenants y rol.
- Tenant activo se resuelve de sesión y se revalida contra membresía en cada petición sensible.
- Servicios reciben un `ActorContext` (user ID, tenant ID, rol/permisos), construido solo por middleware confiable.
- Repositorios exigen tenant ID explícito y filtran siempre por él; las actualizaciones/bajas usan `WHERE tenant_id = ? AND id = ?`.
- MySQL 8 no ofrece RLS equivalente a PostgreSQL: exigir filtros por `tenant_id` en todos los repositorios, pruebas cruzadas y autorización en servidor.
- Evitar IDs de otro tenant en mensajes de error, eventos, auditoría visible y métricas.
- Roles: Owner/Admin/Viewer por tenant; Platform Admin separado.
- Contraseñas Argon2id, sesiones server-side, CSRF, rate limit en login, cookies seguras y headers de seguridad.
- El broker SSE en memoria no es duradero: al reconectar, el cliente debe refrescar estado desde MySQL.

## 8. Componentes reutilizables
### Table
Contrato: columnas declarativas, datos tipados, filtros allowlisted, orden allowlisted, búsqueda, page size máximo 50, acciones según policy, estados vacíos.

### Form
Contrato: campos, validación en servidor, errores por campo, valores persistidos y protección de campos no permitidos. No mapear arbitrariamente request body a entidad.

### Dashboard
Widgets de estadísticas y serie temporal; consultas agregadas por tenant; refresco manual y por SSE.

**Regla:** extraer un componente compartido solo después de implementarlo en clientes y productos, con pruebas.

## 9. Eventos y consistencia sin Redis
- Publicar eventos SSE **después** de confirmar la transacción MySQL.
- Broker en memoria con canales por tenant y suscriptores protegidos contra concurrencia; limpiar conexiones al desconectarse.
- No almacenar datos sensibles en eventos; solo señales de invalidación.
- Eventos best-effort: un reinicio puede perder notificaciones, nunca datos de negocio.
- **Una sola instancia Go** para esta POC. Si se requieren varias réplicas, diseñar outbox/broker externo en una fase posterior.
- Sin caché de aplicación ni caché de consultas: lecturas directas a MySQL.
- Sesiones server-side en MySQL (`sessions`), con identificadores opacos y hash SHA-256 del token como clave de búsqueda.

## 10. Operación
- Multi-stage Docker build: generar Templ/Tailwind y compilar Go; imagen runtime mínima.
- Compose: `app`, `mysql` y reverse proxy opcional.
- Migraciones como paso controlado de despliegue, no en cada réplica al arrancar.
- Logs JSON con request ID; métricas de latencia, errores, conexiones DB y clientes SSE.
- Backups MySQL con restauración verificada; health readiness dependiente de DB.

## 11. Pruebas
- Unitarias: validación, políticas, parsers de filtros y paginación.
- Integración MySQL 8: constraints, consultas por tenant, transacciones y baja lógica.
- HTTP: sesión, CSRF, HTMX parcial, errores, RBAC.
- E2E: dos tenants, dos roles, CRUD y dashboard.
- SSE: reconexión, aislamiento por tenant, reinicio de aplicación, reconexión y refresco.
- Carga: dataset fijo, concurrencia fija, p50/p95, RPS, CPU y memoria.

## 12. Compatibilidad MySQL
- Motor InnoDB, `utf8mb4`, zona horaria de conexiones UTC y modo SQL estricto.
- Placeholders `?`, `LIKE` con collation definida, JSON nativo y `DATETIME(6)`.
- Sin `RETURNING`, `ILIKE`, `ON CONFLICT`, índices parciales ni `CREATE EXTENSION`.
- Para unicidad de SKU con soft delete, usar columna generada e índice UNIQUE (ver DATA.md).

## 13. Evolución posterior
Extraer `Table`, `Form`, `Resource` y `Policy` como biblioteca pública únicamente tras demostrar reutilización en dos módulos. No prometer API estable antes de validarla.

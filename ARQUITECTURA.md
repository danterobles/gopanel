# ARQUITECTURA — GoPanel Pilot

**Versión:** 1.7 · **Estilo:** monolito modular server-rendered

Conciliación de contratos: [ADR-0001](docs/adr/0001-conciliacion-de-contratos-poc.md). Las decisiones aún no tomadas se registran en [decisiones pendientes](docs/decisiones-pendientes.md).

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
| Migraciones | golang-migrate 4.20.1 (ADR-0002) | SQL up/down separado; ejecución desde etapa 2 |

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
`POST /app/customers/new` -> sesión + CSRF + tenant + permiso -> parseo/validación -> servicio/transacción -> auditoría -> respuesta 303 (HTML normal) o fragmento/headers HTMX según contrato. Nunca confiar en campos de control del cliente.

### Clientes implementados (etapa 5)

[ADR-0006](docs/adr/0006-clientes-validacion-y-flujos-http.md) fija campos y flujos. GET normal devuelve documento completo; HTMX recibe contenido del módulo o solo resultados en `/table`. HX-Push-Url mantiene URL canónica con filtros y paginación. Creación HTMX devuelve 201 + detalle; edición 200 + detalle; baja 200 + listado consultado de MySQL. POST normal devuelve 303. Validación 422 devuelve formulario con valores/errores por campo. GET de baja confirma; POST valida confirmación y CSRF. Sin JavaScript de aplicación adicional.

`internal/modules/customers/{input.go,service.go,handler/,repository/}` separa validación, autorización/transacción, HTTP y SQL. Lecturas usan snapshot consistente; mutaciones bloquean tenant para coordinarse con cambios de miembros. Repositorios acotan UUID y tenant antes de acceder al recurso; errores internos 503 sin SQL ni datos de formulario. El layout y componentes visuales de clientes/productos se comparten en `internal/ui/components/resource.templ` tras validar ambos CRUD. El límite de cuerpo de clientes es 64 KiB por notas Unicode codificadas; autenticación/miembros/plataforma conservan 16 KiB.

### Productos (etapa 6)

[ADR-0007](docs/adr/0007-productos-sku-precios-y-extraccion.md) aplica el mismo contrato HTTP de clientes a productos. Validación monetaria entera y SKU específica del dominio; 1062 se transforma en error de campo sin filtrar SQL. Módulo products conserva servicios/repositorios y consultas explícitas con seis órdenes. Después de pasar ambas baterías CRUD se extrajeron layout, navegación, avisos, campos, formulario, acciones, estructura de tabla y paginación. Filas, modelos de vistas, validación, autorización y SQL siguen por dominio; no hay DSL ni registro genérico de recursos. Las baterías de ambos CRUD también pasan después de la extracción.

### Dashboard y tiempo real implementados (etapa 7)

[ADR-0008](docs/adr/0008-dashboard-utc-y-ciclo-sse.md) define ventana UTC y autorización de streams. `modules/dashboard/{handler,service,repository}` usa SQL/sqlc con tenant explícito y snapshot REPEATABLE READ; genera 30 días con ceros y una gráfica SVG con tabla accesible. `/app` tiene navegación/logout y conexión SSE fuera del fragmento `/app/dashboard/widgets`. Refrescos envían un marcador público tenant:rol; si cambia la sesión en otra pestaña, una discrepancia provoca navegación completa para renovar cabecera y permisos visibles. El marcador nunca define el actor ni las consultas.

`platform/events` mantiene solo suscripciones efímeras acotadas; no guarda datos comerciales. Save/Delete de ambos CRUD publican tras COMMIT. SSE revalida sesión y membresía en MySQL cada 2 s y antes de cada evento; stream/widgets no renuevan inactividad. `ready` fuerza nueva lectura al conectar/reconectar, `closed` cierra la extensión al revocar acceso. Buffer lleno cierra al cliente lento; su cupo sigue ocupado hasta salir del handler. Heartbeat 15 s, retry 3 s y escrituras con deadline 5 s mediante ResponseController; contexto de apagado termina streams antes de drenar HTTP. Máximos: 256 conexiones, 32/tenant, 4/usuario y buffer 16. Una sola instancia, sin replay, caché ni cola MySQL.

Mutación confirmada -> publicar evento interno `{type, tenant_id, resource, version}` -> broker en memoria -> SSE `GET /app/events` autenticado y vinculado a tenant -> navegador recibe solo `{type, resource, version}` -> HTMX vuelve a solicitar widget/tabla. No incluir PII ni entidades completas. `version` no garantiza reproducción duradera ni orden global.

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
| GET | `/app/customers/{id}` | `customers.read` |
| GET/POST | `/app/customers/{id}/edit` | `customers.update` |
| GET/POST | `/app/customers/{id}/delete` | `customers.delete` |
| GET | `/app/products` | `products.read` |
| GET | `/app/products/table` | `products.read` |
| GET/POST | `/app/products/new` | `products.create` |
| GET | `/app/products/{id}` | `products.read` |
| GET/POST | `/app/products/{id}/edit` | `products.update` |
| GET/POST | `/app/products/{id}/delete` | `products.delete` |
| GET | `/app/dashboard/widgets` | `dashboard.read`; sesión pasiva |
| GET | `/app/events` | Sesión/membresía revalidadas y rol comercial; sin redirects HTML |
| GET | `/platform/metrics` | Platform Admin activo; sin acceso comercial implícito |
| GET | `/health/live` | Operación |
| GET | `/health/ready` | Operación, sin detalles internos |

## 7. Seguridad y multi-tenancy
- `users` global; `tenant_memberships` vincula usuarios, tenants y rol.
- Tenant activo se resuelve de sesión y se revalida contra membresía en cada petición sensible.
- Servicios reciben un `ActorContext` (user ID, tenant ID, rol/permisos), construido solo por middleware confiable.
- El tenant candidato del selector y los datos objetivo de gestión de miembros se validan en servidor; nunca definen el `ActorContext`. Ver ADR-0001. Las rutas de miembros y plataforma se fijan en ADR-0005; no se infieren endpoints adicionales de esta tabla de referencia.
- Repositorios exigen tenant ID explícito y filtran siempre por él; las actualizaciones/bajas usan `WHERE tenant_id = ? AND id = ?`.
- MySQL 8 no ofrece RLS equivalente a PostgreSQL: exigir filtros por `tenant_id` en todos los repositorios, pruebas cruzadas y autorización en servidor.
- Evitar IDs de otro tenant en mensajes de error, eventos, auditoría visible y métricas.
- Roles: Owner/Admin/Viewer por tenant; Platform Admin separado. Políticas en `internal/platform/rbac/`, denegación por defecto y revalidación MySQL en servicios. Gestión de miembros/configuración y plataforma implementadas según [ADR-0005](docs/adr/0005-permisos-miembros-y-plataforma.md).
- Servicios de miembros/empresas controlan transacción; repositorios aceptan el `*sql.Tx` sin confirmarlo. Se bloquea el tenant antes de autorizar mutaciones de membresía. Cambio + auditoría + revocación se confirman juntos. Los repositorios de persistencia anteriores no son una API autorizada de aplicación.
- Las rutas protegidas comparten sesión, no-store, CSRF, límite de formulario y control de origen con autenticación. Las vistas administrativas permanecen específicas; clientes y productos componen los elementos compartidos extraídos tras validar ambos CRUD.
- Contraseñas Argon2id, sesiones server-side, CSRF, rate limit en login, cookies seguras y headers de seguridad.
- Autenticación y CSRF implementados según [ADR-0004](docs/adr/0004-autenticacion-sesiones-y-csrf.md): cookies opacas, rotación al cambiar empresa, comprobación de origen y tokens en todos los POST. Rate limit persistido en MySQL, sin caché; mantenimiento en el proceso Go.
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

## 11. Validación y operación local (etapa 8)

[ADR-0009](docs/adr/0009-validacion-local-y-operacion.md) delimita E2E HTTP de binario real, recuperación y medición nativas con bases temporales. `platform/observability` mantiene contadores/histogramas acotados de proceso, no caché de negocio. `/platform/metrics` requiere sesión y flag global activo, con pool SQL, memoria y estado agregado SSE sin etiquetas de tenant/usuario. Los abortos liberan el gauge mediante defer. Logs mantienen request ID/ruta/status/duración sin cuerpo/query/cookie.

`platform/backup` y `cmd/backup` exportan consistentemente a archivos privados y rechazan sobrescritura. RestoreTemporary solo acepta bases temporales vacías para dumps confiables de pruebas; el ensayo compara tablas y autentica sobre la copia. Makefile y CI incluyen el recorrido nativo. La revisión de navegador complementa la suite HTTP y no se presenta como certificación WCAG.

[ADR-0010](docs/adr/0010-orden-reciente-medido.md) selecciona consultas sqlc estáticas para created_desc usando el índice existente; conserva CASE en otros órdenes. Se midió antes/después y verificó paridad de filtros/estado/empates/páginas entre tenants. Sin nuevas migraciones, pool ampliado o caché.

Destino público/TLS/proxy/backup remoto y comparación Laravel/Filament siguen pendientes de datos externos confirmados por el usuario. Runbook propuesto, protocolo y resultados locales no equivalen a despliegue verificado.

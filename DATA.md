# DATA — Modelo de datos GoPanel Pilot

**Versión:** 1.4 · **Motor:** MySQL 8.0+ (InnoDB) · **Convenciones:** UUID v4 generado en Go como `CHAR(36)`, `DATETIME(6)` UTC, `snake_case`. **Sin Redis ni caché de aplicación.**

Conciliación: [ADR-0001](docs/adr/0001-conciliacion-de-contratos-poc.md). El esquema SQL de referencia se implementó en `migrations/000001_initial.up.sql` durante etapa 2; rollback separado y operación nativa según [ADR-0003](docs/adr/0003-persistencia-y-desarrollo-nativo.md).

## 1. Relaciones
```text
users 1---N tenant_memberships N---1 tenants
                 | role: owner/admin/viewer
users 1---N sessions
tenants 1---N customers
tenants 1---N products
tenants 1---N audit_logs
users 1---N audit_logs (actor_user_id opcional)
```

## 2. Invariantes
- El usuario puede pertenecer a múltiples tenants; correo único global, normalizado en minúsculas en la aplicación.
- Cada tenant mantiene al menos un Owner activo: verificar con transacción y bloqueo de la fila de `tenants` (`SELECT ... FOR UPDATE`) en toda mutación de membresías.
- Toda lectura/mutación de clientes y productos filtra por tenant activo obtenido de sesión validada.
- Borrado lógico en `customers` y `products` (`deleted_at`). SKU único por tenant **solo entre productos no borrados**.
- UUID v4 generado en Go (no depende de extensiones SQL); MySQL `CHAR(36)` simplifica interoperabilidad de la POC.
- Almacenar UTC: configurar DSN con `parseTime=true&loc=UTC` y establecer `time_zone='+00:00'` por conexión (configuración de DSN o inicialización controlada).
- Motor InnoDB, `utf8mb4`, modo SQL estricto; no asumir soporte PostgreSQL.
- Sesiones persistidas en MySQL; no hay Redis, pub/sub externo ni caché.

## 3. Migración inicial MySQL 8 (referencia)
```sql
CREATE TABLE users (
  id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
  email VARCHAR(254) NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  display_name VARCHAR(160) NOT NULL,
  is_platform_admin BOOLEAN NOT NULL DEFAULT FALSE,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  CONSTRAINT chk_users_email CHECK (CHAR_LENGTH(TRIM(email)) > 3),
  UNIQUE KEY uq_users_email (email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE tenants (
  id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
  name VARCHAR(180) NOT NULL,
  slug VARCHAR(100) NOT NULL,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  UNIQUE KEY uq_tenants_slug (slug)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE tenant_memberships (
  id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
  tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  user_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  role VARCHAR(20) NOT NULL,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  CONSTRAINT chk_membership_role CHECK (role IN ('owner','admin','viewer')),
  CONSTRAINT fk_membership_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE RESTRICT,
  CONSTRAINT fk_membership_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT,
  UNIQUE KEY uq_membership_tenant_user (tenant_id, user_id),
  KEY ix_membership_user_active (user_id, is_active),
  KEY ix_membership_tenant_role_active (tenant_id, role, is_active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE customers (
  id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
  tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  name VARCHAR(180) NOT NULL,
  email VARCHAR(254) NULL,
  phone VARCHAR(40) NULL,
  status VARCHAR(12) NOT NULL DEFAULT 'active',
  notes TEXT NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  deleted_at DATETIME(6) NULL,
  CONSTRAINT chk_customers_status CHECK (status IN ('active','inactive')),
  CONSTRAINT chk_customers_name CHECK (CHAR_LENGTH(TRIM(name)) > 0),
  CONSTRAINT fk_customers_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE RESTRICT,
  KEY ix_customers_tenant_deleted_created (tenant_id, deleted_at, created_at, id),
  KEY ix_customers_tenant_deleted_status_name (tenant_id, deleted_at, status, name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE products (
  id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
  tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  sku VARCHAR(80) NOT NULL,
  name VARCHAR(180) NOT NULL,
  description TEXT NULL,
  price_cents BIGINT NOT NULL DEFAULT 0,
  currency CHAR(3) NOT NULL DEFAULT 'MXN',
  status VARCHAR(12) NOT NULL DEFAULT 'active',
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  deleted_at DATETIME(6) NULL,
  active_sku VARCHAR(80) GENERATED ALWAYS AS (
    CASE WHEN deleted_at IS NULL THEN sku ELSE NULL END
  ) STORED,
  CONSTRAINT chk_products_price CHECK (price_cents >= 0),
  CONSTRAINT chk_products_currency CHECK (currency = 'MXN'),
  CONSTRAINT chk_products_status CHECK (status IN ('active','inactive')),
  CONSTRAINT chk_products_sku CHECK (CHAR_LENGTH(TRIM(sku)) > 0),
  CONSTRAINT chk_products_name CHECK (CHAR_LENGTH(TRIM(name)) > 0),
  CONSTRAINT fk_products_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE RESTRICT,
  UNIQUE KEY uq_products_active_sku (tenant_id, active_sku),
  KEY ix_products_tenant_deleted_created (tenant_id, deleted_at, created_at, id),
  KEY ix_products_tenant_deleted_status_name (tenant_id, deleted_at, status, name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE audit_logs (
  id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
  tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
  actor_user_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
  action VARCHAR(100) NOT NULL,
  entity_type VARCHAR(80) NOT NULL,
  entity_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
  outcome VARCHAR(20) NOT NULL,
  request_id VARCHAR(100) NULL,
  metadata JSON NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  CONSTRAINT chk_audit_outcome CHECK (outcome IN ('success','denied','error')),
  CONSTRAINT fk_audit_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE RESTRICT,
  CONSTRAINT fk_audit_actor FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE SET NULL,
  KEY ix_audit_tenant_created (tenant_id, created_at),
  KEY ix_audit_actor_created (actor_user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE sessions (
  token_hash BINARY(32) NOT NULL PRIMARY KEY,
  user_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  active_tenant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  last_seen_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  expires_at DATETIME(6) NOT NULL,
  CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_sessions_tenant FOREIGN KEY (active_tenant_id) REFERENCES tenants(id) ON DELETE SET NULL,
  KEY ix_sessions_user (user_id),
  KEY ix_sessions_expiry (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

**Nota MySQL:** `UNIQUE (tenant_id, active_sku)` permite múltiples `NULL`; por eso los SKU de productos eliminados no bloquean nuevos registros. La comparación de SKU sigue la collation elegida (`utf8mb4_0900_ai_ci`, insensible a mayúsculas y acentos). Definir y probar normalización de SKU antes de producción.

## 4. Consultas SQL compatibles con MySQL/sqlc
### Listado de clientes (orden fijo de ejemplo)
```sql
-- name: ListCustomers :many
SELECT id, tenant_id, name, email, phone, status, created_at
FROM customers
WHERE tenant_id = sqlc.arg(tenant_id)
  AND deleted_at IS NULL
  AND (sqlc.arg(search) = ''
       OR name LIKE CONCAT('%', sqlc.arg(search), '%') ESCAPE '='
       OR COALESCE(email, '') LIKE CONCAT('%', sqlc.arg(search), '%') ESCAPE '=')
ORDER BY created_at DESC, id DESC
LIMIT ? OFFSET ?;
```
Con sqlc 1.31.1/MySQL, `sqlc.arg` funciona en filtros; LIMIT/OFFSET requieren placeholders posicionales `?`. El repositorio escapa `=`, `%` y `_` usando `=` como carácter de escape explícito; la barra invertida se conserva literalmente como parte del parámetro. Para orden dinámico, seleccionar entre consultas/columnas allowlisted; nunca interpolar el parámetro `sort` recibido.

### Actualización aislada por tenant
```sql
-- name: UpdateCustomerName :execrows
UPDATE customers
SET name = ?
WHERE tenant_id = ? AND id = ? AND deleted_at IS NULL;
```
El pool configura `clientFoundRows=true` para contar filas coincidentes. Verificar el resultado sin confundir filas modificadas con filas coincidentes: una edición válida sin cambios no debe producir un falso 404. Resolver la existencia con una comprobación acotada al tenant o una configuración del driver verificada para contar coincidencias (ADR-0001). Un recurso ausente, eliminado o de otro tenant recibe 404 genérico. No utilizar `UPDATE ... RETURNING` (no disponible en MySQL 8).

### Dashboard
```sql
SELECT COUNT(*) AS active_customers
FROM customers
WHERE tenant_id = ? AND status = 'active' AND deleted_at IS NULL;
```

## 5. Sesiones sin Redis
- Generar token aleatorio con `crypto/rand` (al menos 32 bytes), entregar solo en cookie segura y guardar `SHA-256(token)` en `sessions.token_hash`.
- Comprobar expiración absoluta e inactividad en servidor. Renovar `last_seen_at` con frecuencia limitada para evitar escrituras en cada request, sin caché.
- Revocar eliminando la fila; rotar token al iniciar sesión o elevar privilegios.
- Revalidar membresía y tenant activo en operaciones sensibles: `active_tenant_id` de la sesión no es autorización suficiente.
- Limpiar sesiones vencidas con job periódico del proceso o tarea SQL programada; no requiere Redis.

### Política implementada en etapa 3

Ver [ADR-0004](docs/adr/0004-autenticacion-sesiones-y-csrf.md): expiración absoluta 12 h, inactividad 30 min, touch condicionado cada 1 min y limpieza cada 5 min por defecto. Cambiar tenant rota token/CSRF conservando creación y expiración absoluta. Solo se persiste SHA-256 del token; cada acceso resuelve membresía/tenant/usuario activos. Etapa 4 implementa revocación transaccional de todas las sesiones del usuario ante altas o cambios efectivos de membresía, incluida elevación de privilegios (ADR-0005). Ediciones sin cambios conservan sesiones. Desactivar empresa revoca sesiones que la tienen seleccionada.

La migración aditiva `000002_login_limits` añade `login_rate_limits` con clave BINARY(32) (hash de IP remota), window_start DATETIME(6), attempts INT UNSIGNED e índice por ventana. Permite 10 intentos/minuto por IP por defecto, con admisión transaccional. No modifica 000001 ni introduce caché. La migración down 2 elimina únicamente estos contadores.

## 6. Matriz de permisos
| Acción | Owner | Admin | Viewer |
|---|---|---|---|
| dashboard.read | Sí | Sí | Sí |
| customers.read | Sí | Sí | Sí |
| customers.create/update/delete | Sí | Sí | No |
| products.read | Sí | Sí | Sí |
| products.create/update/delete | Sí | Sí | No |
| memberships.read | Sí | Sí | No |
| memberships.manage | Sí | No | No |
| tenant.settings | Sí | No | No |

Platform Admin opera con permisos de plataforma separados; no obtiene acceso automático a datos comerciales.

### Política implementada en etapa 4

[ADR-0005](docs/adr/0005-permisos-miembros-y-plataforma.md) fija rutas y campos. La política consulta membresía/usuario/empresa activos desde MySQL y deniega acciones o roles desconocidos; un rol enviado por formulario o desactualizado en contexto no autoriza. Modificar miembros bloquea primero la empresa y comprueba permisos, objetivo acotado e invariante de Owner dentro de la misma transacción que la auditoría y revocación.

Miembros: alta por correo exacto normalizado, cuenta activa existente y rol Admin/Viewer; cambios de estado y rol, sin promover a Owner. Owner existente puede conservar su rol o degradarse si queda otro Owner activo. No hay transferencia desde UI ni administración de usuarios globales. `tenant.settings` cambia solo nombre (trim, 1–180 caracteres Unicode). Slug inmutable; alta de empresa admite 3–100 caracteres ASCII minúsculos/números/guiones, extremos alfanuméricos.

La administración global crea empresa + Owner existente de forma atómica, modifica nombre/estado y lista empresas de 25 en 25. Métricas técnicas limitadas al pool SQL, sin datos comerciales. Éxitos globales se auditan con tenant NULL y entidad objetivo; las denegaciones no guardan correos, formularios ni identificadores ajenos. No hay migración nueva en etapa 4.

## 7. Eventos SSE
- Broker **en memoria del proceso Go**, separado por tenant; no almacenar eventos en MySQL como cola en esta POC.
- Solo señales `{type, resource, version}` sin PII. Publicar después de `COMMIT`.
- SSE verifica sesión y membresía; al reconectar, el cliente vuelve a consultar MySQL.
- Una sola instancia Go: no hay fan-out entre nodos. La pérdida de eventos durante reinicios es aceptable.

## 8. Seeds y pruebas
- Dos tenants (`empresa-norte`, `empresa-sur`), un Owner por tenant, Admin, Viewer y un usuario en ambos con roles distintos.
- 100 clientes y productos por tenant; dataset de carga de 10 000 por tenant.
- Probar SKU duplicado dentro de tenant, mismo SKU en tenants distintos y reutilización tras soft delete.
- Probar que el último Owner no pueda eliminarse incluso con solicitudes concurrentes.
- Probar CSRF, IDOR, aislamiento en listados/ediciones/dashboard/SSE, caducidad y revocación de sesiones.
- Probar reinicio del proceso Go: datos y sesiones válidas persisten en MySQL; eventos en memoria se pierden sin afectar negocio.
- Contraseñas de seed generadas para desarrollo y no publicadas en repositorio.

## 9. Migraciones y operación
- Migraciones SQL versionadas `up/down` donde sea seguro; no editar migraciones aplicadas.
- `mysqldump` o herramienta equivalente para backups y restauración ensayada.
- Usar `EXPLAIN ANALYZE` de MySQL 8 para validar consultas e índices antes de optimizar.
- MySQL 8 no tiene índices parciales ni RLS equivalente a PostgreSQL; reforzar aislamiento por repositorio y pruebas.
- No incorporar Redis ni capa de caché sin una decisión de alcance posterior.

# Validación de la etapa 2

Fecha: 2026-10-21. Entorno local nativo: macOS ARM64, Go 1.26.10, Node 22.23.2 y MySQL 8.0.40 en 127.0.0.1:3306. No se usó Docker para ejecutar las pruebas de esta etapa; los contenedores previos del proyecto se detuvieron para liberar HTTP 8080.

## Cambios y datos de trabajo

- Se verificó que `goconcept` estaba vacía antes de aplicar `000001_initial.up.sql`. Ahora contiene las siete tablas del contrato y `schema_migrations`, versión 1 limpia.
- Seed aplicado: 2 tenants, 8 usuarios, 8 membresías, 200 clientes, 200 productos y 2 marcas de auditoría. Cada tenant tiene 100 clientes/productos; el usuario compartido tiene roles distintos. Platform Admin no obtiene membresía comercial implícita.
- La segunda ejecución del seed informó `already_seeded=true`, sin crear registros, cambiar passwords ni reemplazar el archivo privado.
- Credenciales MySQL solo en `.env`; credenciales seed aleatorias en `.local/seed-credentials.json` con permisos 0600. Ninguno se versiona ni se incluye en contexto Docker. La cuenta MySQL existente no fue alterada.
- `make dev` y `make smoke` usan Go nativo por defecto. Se conserva Compose como alternativa con `.env.compose`, separado de root/local.

## Verificaciones ejecutadas

| Verificación | Resultado |
|---|---|
| sqlc 1.31.1 + MySQL | Generación exitosa desde migraciones y consultas. LIMIT/OFFSET posicionales, parámetros UTC y override de JSON nullable documentados en ADR-0003. |
| Formato, generación, vet, pruebas unitarias y build | Correctos con los targets Makefile; generación Templ/Tailwind conservada. |
| Integración MySQL local | Aprobada; bases desechables `gopanel_test_*`, independientes de `goconcept`. |
| Up/down/up e idempotencia up | Aprobados en base temporal con datos; siete tablas InnoDB recreadas y versión dirty rechazada. No se hizo rollback sobre `goconcept`. |
| Clientes y productos | Lectura, modificación, baja y listado entre tenants rechazados; búsqueda literal, paginación acotada y conteos; edición sin cambios válida. |
| Constraints | Email/membresía duplicados, FK, roles/estado/nombres inválidos, SKU vacío, precio negativo y moneda ajena rechazados. UUID con ascii_bin; SKU por tenant y reutilizable después de baja lógica. |
| Concurrencia | Dos altas simultáneas con mismo SKU: una aceptación/una duplicada. Dos intentos de retirar Owners: uno aceptado/uno rechazado, queda un Owner activo. |
| Identidad y sesiones (persistencia) | Correo normalizado; membresías activas/roles por empresa; tenant inactivo excluido; sesión persistida entre pools y exclusión por revocación, caducidad o usuario inactivo. No equivale a autenticación HTTP. |
| Auditoría/transacciones | Cliente y auditoría se confirman o revierten juntos; auditoría aislada por tenant y metadata SQL NULL legible. |
| Seed | Conteos, idempotencia, salts/hash Argon2id, correspondencia password/hash y permisos privados verificados. Conflicto de archivo revierte inserciones y preserva contenido previo. Cambio de tamaño de fixture existente rechazado. |
| Detector de carreras con integración | `sh scripts/with-env.sh go test -race -tags=integration ./... -count=1` aprobado contra MySQL local. |
| Smoke nativo | Procesos Go temporales: página/assets y health 200 con MySQL; endpoint DB inalcanzable da ready=503/live=200; ambos procesos finalizan por SIGTERM con código 0. Nunca se detuvo el MySQL local. |

## Rollback y límites

El archivo `000001_initial.down.sql` elimina datos de las siete tablas; el comando exige `ALLOW_MIGRATE_DOWN=1` y revierte una versión. Solo se ensayó sobre bases aleatorias de prueba. MySQL DDL no garantiza rollback transaccional de una migración parcialmente fallida: inspeccionar dirty/estado antes de reparar, sin force automático.

El workload de 10 000 registros por recurso/tenant se puede preparar con `SEED_COUNT=10000` en una base vacía separada; esta entrega valida el seed normal de 100, no un benchmark ni la meta de latencia del PRD. No se han ejecutado E2E de CRUD, RBAC/CSRF HTTP, login ni SSE; corresponden a etapas posteriores. Readiness sigue comprobando conectividad, no versión/esquema.

CI y la alternativa Docker se actualizaron pero no se ejecutaron remotamente ni se reconstruyeron para esta etapa, siguiendo la preferencia de pruebas nativas. Las herramientas de SQL se fijaron en ADR-0002; su compatibilidad efectiva se verificó aquí. La política de autenticación/expiración (D-02) y recuperación de contraseña (D-03) siguen pendientes para etapa 3.

Siguiente paso: etapa 3, autenticación con sesiones server-side, CSRF y selección de empresa, conservando el entorno nativo y la base de trabajo.

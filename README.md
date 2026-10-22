# GoPanel Pilot

SaaS administrativo multiempresa en construcción. Etapas 0–8 (validación local): bootstrap, persistencia, identidad, permisos, miembros, CRUD de clientes/productos, componentes Templ compartidos, dashboard, SSE, métricas, E2E y recuperación. El desarrollo local usa Go, Node y MySQL nativos. SSE opera en una única instancia Go, sin Redis ni caché.

## Arranque nativo

Guía paso a paso: [pruebas locales y compilación para macOS, Linux y Windows](docs/guia-local-y-compilacion.md), con destinos Intel/AMD y ARM64.

Requisitos: MySQL 8.0+ (validado localmente con 8.0.40), Go 1.26.10 y Node 22.23.2. El Makefile selecciona el toolchain fijado; npm usa el lockfile.

Crear una base vacía de desarrollo llamada `goconcept`. Copiar `.env.example` a `.env` si todavía no existe y completar `DB_PASSWORD` con la contraseña local. Se usa `root` por la configuración solicitada para este entorno; no se cambia la cuenta del servidor. `.env` es privado, está excluido de Git y sus valores nunca deben copiarse a documentos o comandos versionados. El archivo usa asignaciones shell confiables; entrecomillar valores con caracteres especiales de shell.

```sh
make generate
make migrate-up
make seed
make dev
```

Visitar [GoPanel local](http://localhost:8080). Ingresar en `/login` con las credenciales seed privadas y seleccionar empresa. `/app` muestra el dashboard de la empresa activa, su nombre y rol de membresía verificada. Ctrl+C detiene Go con drenaje de solicitudes; no detiene MySQL. `make dev`, migraciones, seeds e integración cargan `.env` mediante `scripts/with-env.sh`. El binario por sí mismo solo lee entorno; se puede ejecutar desde cualquier directorio porque sus assets están embebidos.

El entorno local ya configurado usa MySQL en `127.0.0.1:3306`, base `goconcept` y HTTP en 8080. No se requieren contenedores. La primera generación puede descargar módulos y herramientas fijadas.

## Migraciones y rollback

`000001_initial.up.sql` crea siete tablas: users, tenants, tenant_memberships, customers, products, audit_logs y sessions. `000002_login_limits.up.sql` añade contadores de intentos de login en MySQL, sin modificar la primera migración. golang-migrate mantiene además `schema_migrations`. No se migra al iniciar Go. Ejecutar `make migrate-up` es repetible; no modifica una versión ya aplicada.

El rollback revierte una versión: down de 000002 elimina solo los contadores de login; down de 000001 elimina las siete tablas y sus datos en orden inverso de dependencias. Debe probarse en una base desechable; no se ejecuta sobre `goconcept` para comprobar el desarrollo. Cuando corresponda operativamente, respaldar y probar restauración antes de una reversión con datos necesarios. El comando requiere selección explícita de la base por entorno y:

```sh
ALLOW_MIGRATE_DOWN=1 make migrate-down
```

Revierte una versión. MySQL DDL hace commits implícitos: un fallo parcial puede dejar `schema_migrations.dirty=1`. Inspeccionar tablas/causa y reparar siguiendo la versión real; no hay `force` ni reset automáticos. Nunca editar una migración aplicada; agregar otra versión.

## Seeds

`make seed` instala transaccionalmente dos empresas (`empresa-norte`, `empresa-sur`), ocho usuarios y ocho membresías. Cada empresa tiene un Owner, Admin y Viewer; el usuario compartido es Admin en Norte y Viewer en Sur. Platform Admin no recibe membresías comerciales. Se crean 100 clientes y 100 productos por empresa, con SKU repetibles entre empresas y auditoría de inicialización.

Las ocho cuentas usan correos `@gopanel.test`; contraseñas aleatorias independientes de MySQL se guardan solo en `.local/seed-credentials.json` con permisos 0600. La base guarda hashes Argon2id con salts independientes. No se imprimen contraseñas. Para probar ambas empresas, iniciar sesión con `shared@gopanel.test` y la contraseña de ese registro privado; tiene rol Admin en Norte y Viewer en Sur. Las credenciales existentes no se rehashan ni se restablecen al desplegar etapa 3.

Repetir el seed completo conserva cuentas, contraseñas y datos existentes. Las marcas de auditoría identifican el fixture. Un fixture parcial, un tamaño distinto o colisiones se rechazan sin sobrescribir. Si falta el archivo privado tras un seed existente, no se recuperan contraseñas desde los hashes ni se cambian automáticamente. Un fallo al escribir un archivo nuevo cancela la transacción y preserva un archivo previo. Si el resultado del commit es incierto por una desconexión, se conserva el archivo privado para inspección; no se descartan las únicas credenciales de una transacción que pudo haberse confirmado.

Para carga, usar `SEED_COUNT=10000 make seed` sobre una base de desarrollo vacía distinta, configurada en un entorno separado; no aumenta silenciosamente un seed de 100 ya existente. Esta opción prepara datos, no demuestra la meta de rendimiento.

## Pruebas y comandos

| Comando | Comportamiento |
|---|---|
| `make dev` | Compilar y ejecutar Go nativo en foreground con `.env` |
| `make generate` | npm ci, Tailwind, HTMX/SSE locales, sqlc y Templ fijados |
| `make generate-sql` | Regenerar consultas Go con sqlc 1.31.1 |
| `make fmt`, `make lint` | gofmt y `go vet ./...` |
| `make test` | Unitarias y HTTP sin requerir MySQL externo |
| `make test-integration` | MySQL real; crea/elimina únicamente bases aleatorias `gopanel_test_*`, nunca resetea la base de trabajo |
| `make build` | Compilar `bin/gopanel` sin CGO |
| `make smoke-rbac` | Proceso Go nativo temporal: Owner/Admin/Viewer, clientes aislados por empresa, panel de plataforma y CSRF con seeds privados; no cambia datos de negocio |
| `make smoke-auth` | Login con el usuario compartido del archivo seed privado, selección/cambio de empresa y logout a través de un reinicio real del proceso Go; requiere seed y migraciones aplicadas mediante `make migrate-up` |
| `make smoke` | Ejecutar procesos Go temporales en puertos libres; verificar assets, health y SIGTERM. Simula fallo de conexión con un endpoint inalcanzable, sin detener MySQL |
| `make migrate-up`, `make migrate-down` | Aplicar pendientes o revertir una versión con protección explícita |
| `make seed` | Fixture de desarrollo transaccional, idempotente y credenciales privadas |
| `make dev-compose` | Alternativa Compose con `.env.compose`, separada del entorno nativo |
| `make compose-migrate-up` | Migrador de la imagen, operación explícita para la base Compose |

Las pruebas de integración requieren permisos para crear/eliminar bases temporales, disponibles con la cuenta local configurada. En CI o entornos con usuario de aplicación restringido, `TEST_DB_USER` y `TEST_DB_PASSWORD` permiten una cuenta separada para esas bases. No usar una base compartida como destino de rollback.

```sh
make lint test build
make test-integration
sh scripts/with-env.sh go test -race -tags=integration ./...
make smoke
```

Se verifican up/down/up y dirty, aislamiento entre tenants en lectura/mutación/listado/auditoría, paginación/búsqueda literal, constraints, SKU concurrente y reutilización tras baja, edición sin cambios, protección concurrente del último Owner, transacciones y persistencia/revocación/caducidad de sesiones. También se verifican sesiones/CSRF HTTP y HTML/HTMX, cookies de producción bajo TLS de prueba, membresías revocadas, límites de login y errores genéricos. La etapa 4 añade matriz RBAC tabular, gestión de miembros y configuración en HTTP, plataforma independiente, revocación al cambiar privilegios y rollback de auditoría.

## Configuración

| Variable | Regla |
|---|---|
| `APP_ENV` | development/test/production; seed solo permite development |
| `HTTP_ADDR` | Host:puerto; default `:8080` |
| `DB_HOST`, `DB_PORT` | Default `127.0.0.1`, `3306` |
| `DB_NAME`, `DB_USER` | `.env.example`: goconcept/root; fallback del binario gopanel/gopanel |
| `DB_PASSWORD` | Obligatoria y privada |
| `DB_TLS` | false local; producción exige true con validación de certificado |
| `HEALTH_TIMEOUT`, `SHUTDOWN_TIMEOUT` | 2s/10s; positivos y hasta 1m |
| `TEST_DB_USER`, `TEST_DB_PASSWORD` | Opcionales, solo pruebas desechables |
| `SESSION_ABSOLUTE_TTL`, `SESSION_IDLE_TTL` | 12h/30m; absoluto 5m–7d, idle 1m–24h y no mayor al absoluto |
| `SESSION_TOUCH_INTERVAL` | 1m; entre 1s y 1h y como máximo la mitad del idle |
| `AUTH_CLEANUP_INTERVAL` | 5m; entre 1s y 1h |
| `LOGIN_ATTEMPTS_PER_MINUTE` | 10; entre 1 y 100 por IP remota, ventana de un minuto |
| `SEED_COUNT` | 100 por defecto, de 1 a 10 000; workload documentado de 10 000 |

Pool de aplicación: máximo 10 conexiones abiertas/5 ociosas, vida 3 minutos, idle 1 minuto y red 3 segundos. UTC, utf8mb4, collation utf8mb4_0900_ai_ci, modo estricto y `clientFoundRows` por conexión. Solo migraciones permiten múltiples sentencias. HTTP: cabeceras 5s, lectura/escritura 15s; SSE renueva el deadline de cada escritura/flush a 5s según ADR-0008.

GET `/health/live` comprueba proceso; GET `/health/ready` comprueba conexión MySQL (todavía no versión/esquema). Devuelven JSON genérico no cacheable y no exponen credenciales. Logs JSON con request ID interno, patrón de ruta, método, estado y latencia; sin query strings, cuerpos ni valores de panic.

## Sesiones y protección de formularios

Login crea token aleatorio de 32 bytes y guarda solo su hash SHA-256 en MySQL. Cookies HttpOnly/SameSite=Lax, Secure y prefijo `__Host-` en producción. Logout revoca en servidor; cambiar empresa rota token/CSRF sin prolongar el vencimiento absoluto. La sesión y el rol se revalidan desde MySQL, incluso tras un reinicio. El touch limitado puede adelantar conservadoramente la caducidad por inactividad hasta un intervalo.

Los POST requieren token CSRF y comprobación de origen, también login y HTMX. Los formularios funcionan sin JavaScript; HTMX mejora errores parciales y redirecciones. Un formulario abierto antes de cambiar empresa se rechaza por CSRF obsoleto. No se aceptan IDs de usuario ni roles del formulario para definir al actor.

Login usa Argon2id 64 MiB/3 iteraciones/2 lanes con verificación PHC acotada, hash dummy para cuentas inexistentes/inactivas y máximo dos verificaciones simultáneas. La IP se obtiene del socket, ignorando cabeceras proxy; los intentos se contabilizan en MySQL, devolviendo 429 con Retry-After al superar el límite. La adaptación a proxy pertenece a D-09.

El proceso limpia sesiones vencidas/ociosas y buckets de intentos de más de 24h; la limpieza no es necesaria para rechazar una sesión vencida en una petición. Mutaciones de sesión y auditoría de éxito son atómicas; no se auditan contraseñas, tokens ni correos de intentos fallidos. Los errores internos devuelven 503 genérico con request ID correlacionable.

La recuperación automática de contraseña está diferida (D-03); no existe ruta de reset ni se añadió correo. Un Platform Admin sin membresías no accede a empresas comerciales.

## Usuarios para probar

Ver [cuentas y recorridos locales](docs/pruebas-locales.md). `shared@gopanel.test` es Admin en Norte y Viewer en Sur; hay Owner/Admin/Viewer individuales por empresa y `platform@gopanel.test` para administración global. Contraseñas en `.local/seed-credentials.json`, distintas de MySQL; no se han restablecido. Para usuarios simultáneos usar contextos/perfiles separados, porque las pestañas comparten cookie.

## Miembros y administración

Desde `/app`, Owner/Admin acceden a `/app/members`; solo Owner agrega cuentas existentes por correo y asigna Admin/Viewer o cambia su estado. Viewer recibe 403 incluso con HTTP manual. El Owner inicial se crea con seed o alta de empresa; la interfaz no promueve a Owner ni transfiere propiedad. No puede degradarse/desactivarse el último Owner activo, aun con solicitudes concurrentes. Los cambios efectivos revocan todas las sesiones del usuario objetivo: deberá volver a ingresar. Guardar sin cambios conserva las sesiones.

Owner puede cambiar el nombre desde `/app/settings`. El slug no se modifica. La cuenta `platform@gopanel.test` tiene un acceso a `/platform` desde el selector: lista/crea empresas con Owner de cuenta activa existente, cambia nombre/estado y ve conexiones del pool MySQL. No obtiene acceso comercial por ser operador. Desactivar empresa revoca las sesiones que la tienen seleccionada; reactivarla vuelve a habilitar su selección para miembros activos.

Los listados administrativos muestran 25 filas por página. Validación 422, permiso denegado 403, recurso ajeno/ausente 404 y conflicto del último Owner 409; HTMX muestra errores junto al formulario sin perder valores ni foco. Mutación, auditoría de éxito y revocación comparten transacción. No se registran correos, nombres ni tokens en auditoría. Sin cambios de esquema ni migraciones nuevas. Contrato detallado: [ADR-0005](docs/adr/0005-permisos-miembros-y-plataforma.md).

## Clientes

Entrar con Owner/Admin, seleccionar empresa y abrir **Clientes** desde `/app`. **Nuevo cliente** permite crear; **Ver detalle** ofrece edición y confirmación de baja lógica. Viewer consulta sin botones de mutación y recibe 403 si intenta POST manual. Datos ajenos, ausentes, eliminados o IDs malformados reciben el mismo 404. La baja no tiene restauración desde la interfaz.

Nombre y estado son obligatorios. Correo, teléfono y notas son opcionales; vacíos se guardan como NULL. Nombre hasta 180 caracteres Unicode, correo simple hasta 254 bytes (trim/minúsculas), teléfono libre hasta 40 caracteres y notas hasta 5000 con saltos LF. No se impone unicidad de correo de clientes ni un país para teléfonos. Errores por campo conservan valores/foco; el servidor rechaza UTF-8 inválido y controles no permitidos. Cuerpo de formulario de clientes y productos limitado a 64 KiB; autenticación y administración mantienen 16 KiB.

Listado con búsqueda literal por nombre/correo, estado, nombre A–Z/Z–A o fecha reciente/antigua y páginas de 10/25/50. Por ejemplo, `/app/customers?search=Cliente&status=active&sort=name_asc&page=1&page_size=10`. Filtros y paginación conservan sus parámetros en la URL. Todos los flujos tienen HTML completo sin HTMX; con HTMX intercambian fragmentos y actualizan la URL sin recarga completa. Cambio y auditoría se confirman en una misma transacción; no hay migraciones nuevas.

Contrato y validación: [ADR-0006](docs/adr/0006-clientes-validacion-y-flujos-http.md), [etapa 5](docs/validacion-etapa-5.md) y [etapa 6](docs/validacion-etapa-6.md). Tras validar ambos CRUD se compartieron layout, navegación, campos, formularios, avisos, tabla y paginación en `internal/ui/components/resource.templ`. Las filas, campos específicos, validación, permisos y SQL permanecen en cada dominio.

## Productos

Owner/Admin pueden abrir **Productos** desde `/app`, crear, consultar, editar y confirmar la baja lógica. Viewer consulta; el servidor rechaza sus mutaciones. Cada operación queda acotada a la empresa activa y comparte transacción con su auditoría.

SKU y nombre son obligatorios (hasta 80 y 180 caracteres Unicode). SKU conserva caso/acentos tras trim; el índice MySQL impide equivalentes dentro de una empresa, incluso concurrentemente, y permite repetirlo en otra empresa o reutilizarlo tras una baja. Un duplicado devuelve 422 junto al campo. Descripción opcional hasta 5000 caracteres; estado activo/inactivo.

Precio en MXN, por ejemplo `1250.50`, sin separadores de miles, signos ni exponentes y con máximo dos decimales. Se convierte a centavos enteros sin float ni redondeo; límite `92233720368547758.07`. La moneda del MVP es MXN.

Búsqueda literal por SKU/nombre, filtro por estado y orden por nombre, fecha o precio en ambas direcciones. Páginas de 10/25/50. Ejemplo: `/app/products?search=SKU&status=active&sort=price_asc&page=1&page_size=10`. HTML completo y fragmentos HTMX con validación visible y URL navegable. Sin migraciones nuevas.

Contrato: [ADR-0007](docs/adr/0007-productos-sku-precios-y-extraccion.md). Pruebas y extracción: [validación de etapa 6](docs/validacion-etapa-6.md); tiempos separados en [registro de esfuerzo](docs/registro-de-esfuerzo.md).

## Dashboard y SSE

Tras seleccionar empresa, `/app` muestra clientes activos, productos activos, clientes recientes y una gráfica diaria accesible con tabla desplegable por teclado. Los últimos 30 días son días calendario UTC desde 00:00 del día actual menos 29 días hasta el instante consultado; clientes recientes incluyen activos/inactivos, excluyendo bajas y fechas futuras. Las tarjetas de activos cuentan todos los registros activos no eliminados, independientemente de su fecha.

**Actualizar métricas** hace GET parcial a `/app/dashboard/widgets`; **Recargar página** funciona con navegación normal. Se muestran estados vacío, error y carga. El dashboard conserva enlaces a recursos, miembros/configuración según rol y logout. Viewer consulta métricas sin permisos de escritura.

La extensión SSE instalada se sirve desde `/static/sse.js`. `/app/events` autentica con cookie y envía señales sin datos comerciales; crear/editar/dar de baja clientes o productos desde otra pestaña actualiza las métricas tras el commit. Cada reconexión refresca desde MySQL, incluso tras reiniciar Go. No hay replay ni garantía de entrega; mantener refresco manual. Cambiar empresa en otra pestaña fuerza recarga del dashboard al detectar que su contexto quedó atrás.

Streams y refrescos de fondo no renuevan la sesión. SSE verifica autorización cada 2 segundos y antes de cada señal; revocación, cambio de empresa, logout o caducidad cierran la conexión. Buffer 16; máximo 256 streams, 32 por empresa y 4 por usuario. Cliente lento se desconecta sin bloquear el CRUD y retiene su cupo hasta salir. Una sola instancia Go; para usar el mismo puerto, detener primero el proceso anterior de la aplicación, sin detener MySQL.

Contrato y pruebas: [ADR-0008](docs/adr/0008-dashboard-utc-y-ciclo-sse.md) y [validación de etapa 7](docs/validacion-etapa-7.md). Sin nuevas dependencias o migraciones.

## Validación operativa local

`make test-e2e` ejecuta el binario real con bases MySQL temporales, incluyendo HTMX/SSE, reinicio, miembros y ensayo de backup/restauración con login. `make bench` mide 50 sesiones independientes sobre 10 000 registros por recurso/empresa; ejecutarlo sin otras pruebas en paralelo. [Resultado Go](docs/benchmark-go.md): p95 clientes 286.38 ms y productos 287.91 ms en una muestra controlada, sin comparación Laravel ni garantía de producción. `python3 scripts/count-loc.py` separa código escrito y generado.

`make backup` crea un snapshot privado nuevo de la base configurada, sin sobrescribir. La restauración automatizada solo se ensaya en destinos temporales vacíos. [Runbook](docs/operacion.md), [protocolo](docs/protocolo-benchmark.md) y [validación de etapa 8](docs/validacion-etapa-8.md) documentan comandos, evidencias y límites. No se ejecutaron down, carga ni restauración contra goconcept.

Platform Admin puede consultar `/platform/metrics` desde el enlace de Administración: histogramas/contadores HTTP acotados, memoria Go, pool SQL y estado global SSE. Requiere autorización en cada petición; no expone PII ni identificadores como etiquetas. La telemetría se reinicia con Go y no sustituye un backend de monitoreo.

## Alternativa Compose

Se conserva para reproducibilidad, sin usarla para validar esta etapa local. Copiar `.env.compose.example` a `.env.compose` y definir contraseñas privadas distintas; no reutilizar el `.env` nativo de root como `MYSQL_USER` del contenedor. El ejemplo usa MySQL 13306 para evitar colisionar con el servidor local.

```sh
make compose-migrate-up
make dev-compose
```

El primer comando compila el migrador y arranca MySQL si hace falta; la migración es explícita. App y MySQL publican solo loopback; imagen sin root, volumen persistente e imágenes fijadas por digest. Para detener sin borrar datos: `docker compose --env-file .env.compose down`. Cambiar contraseñas del archivo no rota las de un volumen existente.

CI está preparado para generación reproducible, formato/vet, tests, integración con credenciales de prueba, detector de carreras y smoke Compose. Su ejecución remota no se ha realizado desde esta entrega.

## Seguimiento

Leer [PRD](PRD.md), [arquitectura](ARQUITECTURA.md), [datos](DATA.md), [AGENTS](AGENTS.md), [plan](docs/plan-de-implementacion.md) y [aceptación](docs/matriz-de-aceptacion.md). Decisiones: [ADR-0003](docs/adr/0003-persistencia-y-desarrollo-nativo.md), [ADR-0004](docs/adr/0004-autenticacion-sesiones-y-csrf.md), [ADR-0005](docs/adr/0005-permisos-miembros-y-plataforma.md), [ADR-0006](docs/adr/0006-clientes-validacion-y-flujos-http.md), [ADR-0007](docs/adr/0007-productos-sku-precios-y-extraccion.md), [ADR-0008](docs/adr/0008-dashboard-utc-y-ciclo-sse.md), [pendientes](docs/decisiones-pendientes.md). Evidencia: [etapa 1](docs/validacion-etapa-1.md) [etapa 2](docs/validacion-etapa-2.md) [etapa 3](docs/validacion-etapa-3.md) [etapa 4](docs/validacion-etapa-4.md) [etapa 5](docs/validacion-etapa-5.md) [etapa 6](docs/validacion-etapa-6.md) [etapa 7](docs/validacion-etapa-7.md) y [etapa 8](docs/validacion-etapa-8.md).

Cierre local de etapa 8 realizado. Siguiente paso: probar con las cuentas locales y acordar destino de despliegue y contraparte cuando existan. D-09/D-10 conservan pendientes externos de operación y comparación; CI remota, Docker y auditoría WCAG formal no se han validado aquí.

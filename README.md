# GoPanel Pilot

SaaS administrativo multiempresa en construcción. Etapas 0–3: bootstrap, persistencia e identidad multiempresa. El desarrollo local usa Go, Node y MySQL nativos. RBAC por acción/recurso y CRUD HTTP siguen en las etapas siguientes.

## Arranque nativo

Requisitos: MySQL 8.0+ (validado localmente con 8.0.40), Go 1.26.10 y Node 22.23.2. El Makefile selecciona el toolchain fijado; npm usa el lockfile.

Crear una base vacía de desarrollo llamada `goconcept`. Copiar `.env.example` a `.env` si todavía no existe y completar `DB_PASSWORD` con la contraseña local. Se usa `root` por la configuración solicitada para este entorno; no se cambia la cuenta del servidor. `.env` es privado, está excluido de Git y sus valores nunca deben copiarse a documentos o comandos versionados. El archivo usa asignaciones shell confiables; entrecomillar valores con caracteres especiales de shell.

```sh
make generate
make migrate-up
make seed
make dev
```

Visitar [GoPanel local](http://localhost:8080). Ingresar en `/login` con las credenciales seed privadas y seleccionar empresa. `/app` confirma nombre y rol de la membresía verificada; el dashboard se incorporará después. Ctrl+C detiene Go con drenaje de solicitudes; no detiene MySQL. `make dev`, migraciones, seeds e integración cargan `.env` mediante `scripts/with-env.sh`. El binario por sí mismo solo lee entorno; se puede ejecutar desde cualquier directorio porque sus assets están embebidos.

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

Se verifican up/down/up y dirty, aislamiento entre tenants en lectura/mutación/listado/auditoría, paginación/búsqueda literal, constraints, SKU concurrente y reutilización tras baja, edición sin cambios, protección concurrente del último Owner, transacciones y persistencia/revocación/caducidad de sesiones. También se verifican sesiones/CSRF HTTP y HTML/HTMX, cookies de producción bajo TLS de prueba, membresías revocadas, límites de login y errores genéricos. RBAC por acción/recurso se incorpora en etapa 4.

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

Pool de aplicación: máximo 10 conexiones abiertas/5 ociosas, vida 3 minutos, idle 1 minuto y red 3 segundos. UTC, utf8mb4, collation utf8mb4_0900_ai_ci, modo estricto y `clientFoundRows` por conexión. Solo migraciones permiten múltiples sentencias. HTTP: cabeceras 5s, lectura/escritura 15s; SSE revisará el timeout de escritura en D-08.

GET `/health/live` comprueba proceso; GET `/health/ready` comprueba conexión MySQL (todavía no versión/esquema). Devuelven JSON genérico no cacheable y no exponen credenciales. Logs JSON con request ID interno, patrón de ruta, método, estado y latencia; sin query strings, cuerpos ni valores de panic.

## Sesiones y protección de formularios

Login crea token aleatorio de 32 bytes y guarda solo su hash SHA-256 en MySQL. Cookies HttpOnly/SameSite=Lax, Secure y prefijo `__Host-` en producción. Logout revoca en servidor; cambiar empresa rota token/CSRF sin prolongar el vencimiento absoluto. La sesión y el rol se revalidan desde MySQL, incluso tras un reinicio. El touch limitado puede adelantar conservadoramente la caducidad por inactividad hasta un intervalo.

Los POST requieren token CSRF y comprobación de origen, también login y HTMX. Los formularios funcionan sin JavaScript; HTMX mejora errores parciales y redirecciones. Un formulario abierto antes de cambiar empresa se rechaza por CSRF obsoleto. No se aceptan IDs de usuario ni roles del formulario para definir al actor.

Login usa Argon2id 64 MiB/3 iteraciones/2 lanes con verificación PHC acotada, hash dummy para cuentas inexistentes/inactivas y máximo dos verificaciones simultáneas. La IP se obtiene del socket, ignorando cabeceras proxy; los intentos se contabilizan en MySQL, devolviendo 429 con Retry-After al superar el límite. La adaptación a proxy pertenece a D-09.

El proceso limpia sesiones vencidas/ociosas y buckets de intentos de más de 24h; la limpieza no es necesaria para rechazar una sesión vencida en una petición. Mutaciones de sesión y auditoría de éxito son atómicas; no se auditan contraseñas, tokens ni correos de intentos fallidos. Los errores internos devuelven 503 genérico con request ID correlacionable.

La recuperación automática de contraseña está diferida (D-03); no existe ruta de reset ni se añadió correo. Un Platform Admin sin membresías no accede a empresas comerciales.

## Alternativa Compose

Se conserva para reproducibilidad, sin usarla para validar esta etapa local. Copiar `.env.compose.example` a `.env.compose` y definir contraseñas privadas distintas; no reutilizar el `.env` nativo de root como `MYSQL_USER` del contenedor. El ejemplo usa MySQL 13306 para evitar colisionar con el servidor local.

```sh
make compose-migrate-up
make dev-compose
```

El primer comando compila el migrador y arranca MySQL si hace falta; la migración es explícita. App y MySQL publican solo loopback; imagen sin root, volumen persistente e imágenes fijadas por digest. Para detener sin borrar datos: `docker compose --env-file .env.compose down`. Cambiar contraseñas del archivo no rota las de un volumen existente.

CI está preparado para generación reproducible, formato/vet, tests, integración con credenciales de prueba, detector de carreras y smoke Compose. Su ejecución remota no se ha realizado desde esta entrega.

## Seguimiento

Leer [PRD](PRD.md), [arquitectura](ARQUITECTURA.md), [datos](DATA.md), [AGENTS](AGENTS.md), [plan](docs/plan-de-implementacion.md) y [aceptación](docs/matriz-de-aceptacion.md). Decisiones: [ADR-0003](docs/adr/0003-persistencia-y-desarrollo-nativo.md), [ADR-0004](docs/adr/0004-autenticacion-sesiones-y-csrf.md), [pendientes](docs/decisiones-pendientes.md). Evidencia: [etapa 1](docs/validacion-etapa-1.md) [etapa 2](docs/validacion-etapa-2.md) y [etapa 3](docs/validacion-etapa-3.md).

Siguiente etapa: RBAC por acción/recurso y gestión de miembros. TLS/proxy y operación pública siguen en D-09.

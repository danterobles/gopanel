# Validación de etapa 4 — Permisos y miembros

Fecha: 2026-10-21. Entorno local: macOS ARM64, Go 1.26.10, Node 22.23.2, MySQL 8.0.40 nativo. Base de desarrollo `goconcept`; credenciales exclusivamente en `.env` privado. No se usó Docker ni se detuvo MySQL.

## Resultado y alcance

Implementada la matriz DATA con denegación por defecto, permisos vigentes por membresía, gestión de miembros Owner/Admin/Viewer, configuración del nombre de empresa y administración global independiente. D-04/D-05 cerradas antes de implementar mediante [ADR-0005](adr/0005-permisos-miembros-y-plataforma.md). US-03 cubierta con pruebas positivas, negativas, aislamiento y concurrencia.

| Ruta | Autorización y comportamiento |
|---|---|
| GET `/app/members` | Owner/Admin; 25 filas por página, datos solo de empresa activa |
| POST `/app/members` | Owner; cuenta existente activa por correo exacto, rol Admin/Viewer |
| POST `/app/members/{id}` | Owner; rol/estado, objetivo acotado a empresa; protege último Owner |
| GET/POST `/app/settings` | Owner; nombre 1–180 caracteres Unicode, trim; slug inmutable |
| GET `/platform` | Flag global activo; empresas paginadas y métricas de conexiones SQL |
| POST `/platform/tenants` | Operador global; empresa y Owner existente atómicos |
| POST `/platform/tenants/{id}` | Operador global; nombre/estado, sin borrar ni cambiar slug |

El actor y ámbito provienen de sesión/membresía verificadas; se ignoran campos falsificados de actor/tenant. Servicios vuelven a resolver el rol desde MySQL, aunque el contexto traiga un rol antiguo. Un operador global sin membresía no supera políticas comerciales. El correo objetivo usa igualdad parametrizada y comprobación binaria adicional para evitar equivalencias de acentos de la collation.

Se bloquea la empresa antes de autorizar/modificar miembros. Altas y cambios efectivos revocan todas las sesiones del usuario objetivo; una edición sin cambios las conserva. Éxito, auditoría y revocación comparten transacción. Desactivar una empresa revoca las sesiones que la tienen activa. No se guardan formularios, correos, nombres ni tokens en auditoría; los IDs de recursos ajenos no se incluyen en denegaciones.

## Archivos y esquema

- `internal/platform/rbac/`: matriz y resolución de permisos vigentes.
- `internal/modules/members/`, `internal/modules/tenants/`: servicios, repositorios transaccionales y handlers.
- `internal/db/queries/{identity,tenants}.sql` y sus archivos generados: consultas parametrizadas; no SQL dinámico recibido del cliente.
- `internal/platform/auth/handler/protected.go`, autenticación, auditoría y `cmd/server/main.go`: registro de rutas protegidas y política compartida de sesión/origen/CSRF.
- `internal/ui/pages/administration.templ`, `auth.templ`, sus generados y CSS local: navegación por permisos y formularios HTML/HTMX.
- `tests/integration/rbac_http_test.go`, fixture HTTP existente y `scripts/smoke-rbac-native.py`: regresión sobre MySQL y proceso nativo.
- Makefile, README, PRD, DATA, ARQUITECTURA, plan, decisiones y matriz de aceptación actualizados.

No hay migraciones nuevas. Se mantienen versiones 000001/000002 intactas. Las pruebas up/down/up y dirty se ejecutaron exclusivamente en bases aleatorias `gopanel_test_*`, eliminadas al terminar. No se ejecutó rollback sobre `goconcept`. Un retorno al binario de etapa 3 conserva esquema 2 y datos, pero no deshace cambios de rol/estado ni recupera sesiones revocadas.

## Verificaciones ejecutadas

| Comando / comprobación | Resultado |
|---|---|
| `make generate` | Correcto: assets locales, sqlc y Templ; generación repetible comprobada por hashes |
| `make fmt`, `make lint` | Correctos: gofmt y go vet |
| `make test`, `make build` | Correctos |
| `make test-integration` | Correcto contra MySQL nativo, incluidas siete pruebas nuevas de etapa 4 |
| `sh scripts/with-env.sh go test -race -tags=integration ./... -count=1` | Batería completa correcta, sin carreras |
| Detector de carreras de las siete pruebas nuevas de etapa 4 | Revalidación de etapa 4 correcta tras endurecer igualdad de correo; sin carreras |
| `make smoke` | Correcto: proceso/assets/health/SIGTERM; endpoint inalcanzable sin detener MySQL |
| `make smoke-auth` | Correcto: sesión persistida a través de reinicio, rotación, CSRF obsoleto y logout |
| `make smoke-rbac` | Correcto: cuentas seed Owner/Admin/Viewer y operador, CSRF y páginas HTML; sin cambiar datos de negocio |
| Navegador local | Login Owner, selección, navegación autorizada, formulario de cuenta inexistente con error HTMX; teclado, valores/foco conservados; escritorio 1280×720 y tablet 768×1024 |
| `git diff --check` y revisión de secretos | Correctos antes del commit |


Las capturas locales no versionadas son `.local/stage4-members-validation.jpg` y `.local/stage4-members-tablet.jpg`. Sesión/pestaña de prueba cerradas; servidor nativo disponible en 8080.

## Escenarios de seguridad y consistencia

- Matriz tabular de todas las acciones DATA para Owner/Admin/Viewer y roles desconocidos/globales; permiso desconocido denegado.
- Mismo usuario Admin en Norte y Viewer en Sur: lectura permitida solo donde corresponde; HTTP manual no eleva permisos.
- Owner agrega cuenta activa, normaliza correo y asigna rol; cuenta inexistente/inactiva/ya vinculada produce error idéntico. Correo con acentos distintos no vincula otra cuenta.
- Promoción a Owner rechazada; Owner existente puede conservar su rol. Último Owner no se degrada ni desactiva. Dos Owners concurrentes degradándose producen un éxito y un conflicto; queda exactamente uno.
- Miembro de otra empresa y UUID inexistente devuelven el mismo 404; campos falsificados de actor/tenant/flag global no cambian ámbito.
- Cada POST administrativo rechaza CSRF ausente; también se comprueba origen ajeno y fragmento HTMX de error.
- Alta/cambio de rol invalida token anterior; edición sin cambios no revoca. Membresía inactiva deja de acceder a rutas protegidas.
- Paginación real de 28 miembros devuelve 25/3; offsets inválidos rechazados y estados vacíos visibles.
- Configuración solo renombra empresa propia, valida longitud Unicode y conserva slug.
- Operador crea empresa + Owner, no se asigna membresía comercial; duplicado rechazado, desactivación revoca sesión seleccionada, reactivación funciona. Revocar flag global bloquea siguientes lecturas/mutaciones.
- Fallo forzado de auditoría sobre base desechable revierte cambios de rol, creación de empresa/Owner y revocación; no deja éxito parcial.
- Logs sin contraseñas, correos, tokens ni datos de formularios. Auditoría global con tenant NULL y comercial con tenant del actor.

## Limitaciones y siguiente etapa

No se ejecutó CI remoto ni Compose en esta entrega. La revisión visual no equivale a auditoría WCAG completa ni a la batería E2E operativa de etapa 8. Alta pública de usuarios, correo y transferencia/promoción de Owner desde UI permanecen fuera del alcance acordado; cuentas iniciales mediante bootstrap. No se implementaron CRUD de clientes/productos, dashboard ni SSE HTTP antes de sus etapas.

Siguiente etapa propuesta: 5, cerrar D-06 para clientes e implementar su CRUD con validación, filtros, orden, paginación, auditoría y aislamiento HTTP/HTMX.

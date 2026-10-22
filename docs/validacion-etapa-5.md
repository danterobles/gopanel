# Validación de etapa 5 — Clientes

Fecha: 2026-10-21. Entorno: macOS ARM64, Go 1.26.10, Node 22.23.2 y MySQL 8.0.40 nativos. Base local `goconcept`, secretos únicamente en `.env` privado. Sin Docker y sin detener MySQL.

## Resultado

CRUD de clientes implementado con listado, alta, detalle, edición y baja lógica confirmada; búsqueda, estado, cuatro órdenes y paginación 10/25/50. US-04 y parte de clientes de US-05 cubiertas localmente. D-06 de clientes cerrada antes de implementar mediante [ADR-0006](adr/0006-clientes-validacion-y-flujos-http.md). Productos/SKU siguen en etapa 6.

Owner/Admin pueden mutar; Viewer solo consulta. El servicio revalida permiso desde MySQL y el repositorio filtra tenant/UUID. Cambios de membresía y de clientes se coordinan bloqueando tenant antes de autorizar mutaciones; modificación y auditoría de éxito comparten transacción. Listado/conteo usan un mismo snapshot de lectura REPEATABLE READ, sin caché.

## Archivos y contratos

- `internal/modules/customers/input.go` y pruebas: normalización de campos, errores por campo y parser de filtros/páginas.
- `internal/modules/customers/service.go`, `handler/handler.go`, `repository/repository.go`: autorización, SQL acotado, transacciones, auditoría y rutas.
- `internal/db/queries/customers.sql` y generado: orden allowlisted parametrizado y desempate estable; búsqueda literal y conteo filtrado.
- `internal/ui/pages/customers.templ` y generado: layout específico, formularios, detalle, confirmación, tabla y fragmentos. CSS compilado localmente; entrada Clientes desde `/app`.
- `internal/platform/auth/handler/`: Render reutilizado y límite de cuerpo configurable por módulo, conservando sesión/origen/CSRF comunes; clientes 64 KiB, demás formularios 16 KiB.
- `internal/platform/id/`: validación canónica de UUID v4 en minúsculas, probada antes de comparar IDs contra columnas ASCII.
- `cmd/server/main.go`, fixture HTTP y `tests/integration/customers_http_test.go`: conexión real de rutas y cinco pruebas nuevas de integración.
- `scripts/smoke-rbac-native.py`: regresión del binario nativo con clientes de Norte/Sur y rechazo de escritura de Viewer; sin alterar datos de negocio locales.
- PRD, ARQUITECTURA, DATA, README, decisiones, plan y matriz de aceptación actualizados.

Campos: nombre trim requerido 1–180 caracteres Unicode; correo opcional normalizado y dirección simple válida de hasta 254 bytes; teléfono libre opcional de hasta 40 caracteres; notas opcionales hasta 5000 caracteres con saltos LF. Opcionales vacíos a NULL, estado active/inactive obligatorio, UTF-8 válido y controles rechazados excepto LF/tab en notas. La UI conserva campos y foco al mostrar errores 422.

GET normal devuelve página completa; HTMX recibe el contenido del módulo y `/table` devuelve solo resultados/paginación. HX-Push-Url conserva URL canónica y parámetros. POST normal responde 303; creación HTMX 201 con detalle, edición 200 con detalle, baja 200 con listado actualizado y aviso. GET `/{id}/delete` confirma sin JavaScript; POST exige confirmación, permiso y CSRF.

IDs ajenos, ausentes, malformados o eliminados reciben el mismo 404; acción prohibida 403 antes de resolver el registro. Se probaron identificadores Unicode y demasiado largos: la validación evita errores de conversión MySQL. El generador sqlc fijado no expandía `sqlc.arg` dentro de CASE/IN; se usaron comparaciones equivalentes =/OR y se verificaron los cuatro órdenes contra MySQL.

## Migraciones y rollback

Sin migraciones nuevas ni cambios a 000001/000002. Se ejecutaron las pruebas de migración existentes exclusivamente en bases `gopanel_test_*`; no hubo rollback sobre `goconcept`. Volver al binario de etapa 4 mantiene esquema 2, clientes y auditoría persistidos; no deshace ediciones/bajas ya confirmadas. No se implementa restauración desde UI ni borrado físico.

## Verificaciones ejecutadas

| Comprobación | Resultado |
|---|---|
| `make generate` y segunda generación comparando hashes | Correcto: Templ, sqlc, Tailwind y assets locales reproducibles |
| `make fmt`, `make lint` | Correcto: gofmt y go vet |
| `make test`, `make build` | Correcto: unitarias y binario sin CGO |
| `make test-integration` | Correcto sobre MySQL nativo, regresión completa incluidas etapas anteriores |
| `sh scripts/with-env.sh go test -race -tags=integration ./... -count=1` | Correcto: batería completa sin carreras |
| Regresión final con race de clientes y UUID tras añadir casos de IDs malformados | Correcta; incluye HTTP/servicios y SQL existente |
| `make smoke`, `make smoke-auth`, `make smoke-rbac` | Correctos: proceso nativo, health, SIGTERM, sesión tras reinicio, permisos y clientes aislados por empresa |
| Navegador en fixture MySQL desechable con binario nativo | Correcto: alta, validación, edición, baja, filtros, paginación, URL y cambio de rol al seleccionar Sur |
| Teclado y vista de escritorio/tablet | Correctos en los recorridos comprobados; 1280×720 y 768×1024 |
| `git diff --check`, referencias de documentos y secretos del diff | Correctos antes del commit |

Comando de regresión final de carreras:

```sh
sh scripts/with-env.sh go test -race -tags=integration \
  ./internal/modules/customers/... ./internal/platform/id ./tests/integration \
  -run 'Test(Customer|NormalizeCustomer|CanonicalUUID)' -count=1
```

## Escenarios cubiertos

- Alta Admin/Owner, detalle, edición sin cambios (sin falso 404), edición HTMX y confirmación/baja lógica; respuestas normales y parciales con avisos.
- Campos inválidos y UTF-8/controles, límites Unicode, correo sin nombre de presentación, opcionales NULL y notas de 5000 caracteres de cuatro bytes dentro del cuerpo permitido.
- Escape HTML de nombres/notas; parámetros de actor/tenant/ID no controlan identidad, ámbito ni ID generado.
- Recursos ajenos, inexistentes, malformados o eliminados indistinguibles; Viewer sin acciones ni escritura aun con HTTP manual y HX-Request; operador sin membresía sin acceso comercial automático.
- Cada mutación rechaza CSRF ausente y origen ajeno; cuerpos excesivos devuelven 413.
- Búsqueda literal de %, _, barra invertida y =; filtros, cuatro órdenes, páginas 10/25/50, parámetros inválidos, estado vacío y URL conservada entre fragmentos.
- Auditoría con actor/tenant/entidad/resultado, sin valores de campos ni tokens; fallo forzado de auditoría revierte alta, edición y baja.
- Una edición espera el bloqueo de tenant mientras el actor es degradado; al confirmar el cambio a Viewer, la edición se rechaza y el cliente permanece intacto.
- Logs sin nombres/correos/contraseñas/CSRF de los formularios; rutas registradas como patrones, sin parámetros de búsqueda.

## Evidencia visual y operación local

La revisión de navegador usó dos empresas con 30 clientes/productos por empresa en una base aleatoria desechable y credenciales propias fuera de Git. No se crearon/editaron/eliminaron clientes de `goconcept` durante ese recorrido. Base y proceso temporales retirados; sesión/pestaña cerradas y viewport restaurado. El servidor de desarrollo en 8080 queda con esta versión.

Se comprobó error 422 con foco en Nombre y valores opcionales conservados; alta/edición/baja por HTMX; búsqueda tras baja sin registros; orden por nombre, páginas de diez y segunda página; misma cuenta Admin en Norte y Viewer en Sur, con correos solo de la empresa seleccionada y sin controles de mutación para Viewer. La tabla admite desplazamiento horizontal accesible por teclado cuando lo requiere el tamaño.

Capturas locales no versionadas: `.local/stage5-validation.jpg`, `.local/stage5-customer-detail.jpg`, `.local/stage5-customers-desktop.jpg`, `.local/stage5-customers-tablet.jpg` y `.local/stage5-viewer-detail.jpg`. No contienen credenciales.

## Límites y siguiente etapa

No se ejecutó CI remoto ni Compose en esta entrega. La comprobación de teclado/etiquetas/estados no equivale a una auditoría WCAG completa ni a la batería E2E/carga operativa de etapa 8. No se midió p95 ni se realizó comparación Filament; no se extraen todavía componentes genéricos de recursos.

Siguiente etapa: 6, cerrar validación de productos/SKU e implementar su CRUD. Registrar inicio/fin del segundo CRUD y, por separado, la extracción posterior de componentes de clientes/productos; el registro de esta etapa es la primera implementación funcional de referencia, no un benchmark.

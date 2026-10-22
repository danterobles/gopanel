# Validación de etapa 6 — Productos y extracción

Fecha: 2026-10-22. Go 1.26.10, Node 22.23.2 y MySQL 8.0.40 nativos sobre macOS ARM64. Secretos en `.env` privado. No Docker ni cambios de esquema. Integración y navegador usan bases desechables `gopanel_test_*`; las pruebas nativas de permisos no escriben datos comerciales en `goconcept`.

## Incremento 1: productos validado antes de extraer

[ADR-0007](adr/0007-productos-sku-precios-y-extraccion.md) cierra D-06. Implementados alta, detalle, edición sin cambios válida, baja lógica confirmada, búsqueda literal SKU/nombre, estado, seis órdenes y paginación 10/25/50. HTML completo/POST 303 y HTMX con fragmentos/avisos/HX-Push-Url. Precio decimal MXN convertido a BIGINT centavos sin float; SKU Unicode trim conservando caso/acentos con unicidad definitiva del índice MySQL. Sin nuevas dependencias.

Archivos: `internal/modules/products/{input.go,service.go,handler/,repository/}`, `internal/db/queries/products.sql` y generado, vistas products, registro en main y navegación, protección CSRF compartida y fixture HTTP. PRD/DATA/ARQUITECTURA y decisiones actualizados. Commit `0c57cdc`.

Verificación previa a la extracción: `make generate`, `make fmt`, `make lint test build` y `make test-integration` correctos. Tres pruebas unitarias nuevas y cuatro de integración HTTP/servicio cubren importe máximo/fracciones/overflow, campos, collation y SKU concurrente, mismo SKU entre tenants y reutilización, permisos Owner/Admin/Viewer/operador, IDOR/UUID, CSRF/origen/cuerpo, filtros/órdenes/páginas, auditoría/rollback y degradación concurrente de rol. La regresión de clientes también pasó.

## Incremento 2: componentes visuales compartidos

Después de validar ambos CRUD se extrajeron en `internal/ui/components/resource.templ` el layout, navegación, avisos de éxito, mensajes de campo, campos comunes, formulario con CSRF, acciones, estructura/cabeceras de tabla y paginación. `customers.templ` y `products.templ` componen estos elementos conservando sus IDs de foco y destinos HTMX. La navegación entre módulos usa documentos completos porque tienen destinos distintos.

Los modelos, filas, filtros, campo monetario, validación, autorización, SQL y transacciones siguen específicos por dominio. No se añadió DSL, registro genérico, generador, dependencia ni framework. El precio describe su error solo cuando existe. El smoke nativo RBAC ahora comprueba productos como Admin, Owner y Viewer.

## Resultados finales

| Comprobación | Resultado |
|---|---|
| `make generate` | Correcto; repetido y comparados SHA-256 de 19 archivos Templ/SQL/assets, sin cambios |
| `make fmt` y `make lint` | Correctos; gofmt y go vet |
| `make test` y `make build` | Correctos tras extracción |
| `make test-integration` | Suite completa correcta antes y después de extraer; MySQL real |
| `sh scripts/with-env.sh go test -race -tags=integration ./... -count=1` | Correcto, incluida integración de ambos CRUD; sin carreras detectadas |
| `make smoke smoke-auth smoke-rbac` | Correcto: assets/health, dependencia inaccesible sin detener MySQL, sesión tras reinicio, CSRF y roles de ambos recursos |
| Navegador local | Correcto: login/selector, precio inválido con datos preservados, alta 1250.50 MXN, edición 0.01 MXN, pantalla de confirmación, orden por precio y siguiente página, formulario de clientes y Viewer en otra empresa sin acciones |
| Revisión visual | Escritorio 1280×720 y listado en 768×1024; captura inspeccionada. Viewport restablecido, logout, pestaña cerrada y fixture/base eliminados |

Evidencias locales ignoradas por Git: `.local/stage6-products-integration.log`, `.local/stage6-extraction-integration.log`, `.local/stage6-race.log`, `.local/stage6-smoke.log`, `.local/stage6-generation.log`, `.local/stage6-generated-hashes.json`, `.local/stage6-product-created.png` y `.local/stage6-products-tablet.png`. Las capturas contienen datos sintéticos. El navegador verificó la pantalla de confirmación; la ejecución de baja/reutilización se verifica en integración HTTP.

No hay migraciones: se conserva la versión 2. Para retirar la aplicación de esta etapa, volver al binario anterior compatible con ese esquema; no ejecutar rollback destructivo en `goconcept`. Cambiar binario no revierte operaciones de negocio.

Esfuerzo separado en [registro de esfuerzo](registro-de-esfuerzo.md). La ventana mide esta sesión asistida, no horas humanas. No se ejecutaron Docker, CI remota, carga ni comparación Laravel/Filament; estas dos últimas siguen en etapa 8. La revisión de navegador es un smoke manual, no sustituye la futura suite E2E operativa. Siguiente etapa: cerrar D-07/D-08 e implementar dashboard y SSE; no se inicia aquí.

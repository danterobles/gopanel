# Validación de etapa 6 — Productos y extracción

Fecha: 2026-10-22. Go/Node/MySQL nativos; secretos en `.env` privado. No Docker ni cambios de esquema. Las bases de integración son desechables `gopanel_test_*`; no se altera el negocio de `goconcept` para probar CRUD.

## Incremento 1: productos validado antes de extraer

ADR-0007 cierra D-06. Implementados alta, detalle, edición sin cambios válida, baja lógica confirmada, búsqueda literal SKU/nombre, estado, seis órdenes y paginación 10/25/50. HTML completo/POST 303 y HTMX con fragmentos/avisos/HX-Push-Url. Precio decimal MXN convertido a BIGINT centavos sin float; SKU Unicode trim conservando caso/acentos con unicidad definitiva del índice MySQL. Sin migraciones ni nuevas dependencias.

Archivos: módulo products (input, servicio, handler, repositorio), queries/generated/products, vistas products, registro en main y navegación, protección CSRF compartida y fixture HTTP. PRD/DATA/ARQUITECTURA y decisiones actualizados.

Verificación del incremento 1: `make generate`, `make fmt`, `make lint test build` y `make test-integration` correctos. Tres pruebas unitarias nuevas y cuatro de integración HTTP/servicio cubren importe máximo/fracciones/overflow, campos, collation y SKU concurrente, mismo SKU entre tenants y reutilización, permisos Owner/Admin/Viewer/operador, IDOR/UUID, CSRF/origen/cuerpo, filtros/órdenes/páginas, auditoría/rollback y degradación concurrente de rol. La regresión de clientes también pasó.

Esfuerzo registrado por separado en [registro de esfuerzo](registro-de-esfuerzo.md). La ventana mide esta sesión asistida y no equivale a horas humanas ni a benchmark.

## Incremento 2

Extracción compartida pendiente en este commit. La etapa 6 completa requiere comprobar ambos CRUD después de extraer, generación reproducible, detector de carreras, navegador/nativo y documentación final. Dashboard/SSE no se inicia en esta entrega.

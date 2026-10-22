# ADR-0010 — Consulta estática para el listado reciente

Fecha: 2026-10-22. Estado: aceptado por medición en etapa 8. No cambia contratos de filtros/orden/paginación, permisos, auditoría o esquema.

La primera medición aislada con 50 usuarios y 10 000 registros por recurso/tenant mostró p95 ~614 ms clientes y ~623 ms productos, por encima de la meta 300 ms. EXPLAIN ANALYZE de las consultas reales mostraba lectura de 10 000 filas y Sort por expresiones CASE para obtener 25; tiempo individual ~14–16 ms sin la competencia de 50 trabajadores. La consulta de productos elegía un índice de SKU y también ordenaba. Se conservan baseline y planes locales, separados de un ensayo exploratorio que coincidió con otras pruebas.

Se añade una consulta sqlc explícita Recent para created_desc, con los mismos filtros parametrizados y ORDER BY created_at DESC, id DESC. El repositorio selecciona entre Recent y la consulta CASE existente mediante el sort allowlisted ya validado. Esto permite recorrer hacia atrás el índice existente (tenant_id, deleted_at, created_at, id), sin migración ni SQL interpolado, caché o aumento de pool. Otros órdenes conservan la implementación y sus pruebas; no se afirma que el rendimiento medido los cubra.

Verificar paridad de IDs/conteos con la consulta anterior para tenants, filtros literales/estado, empates y páginas, más regresión CRUD/HTMX completa. Repetir protocolo idéntico y capturar nuevo plan antes de afirmar mejora. No extrapolar una muestra local a producción o Laravel/Filament.

## Resultado verificado

Mismo protocolo aislado después del cambio: clientes 6632 muestras, p50 138.02 ms, p95 286.38 ms y 329.4 RPS; productos 6491 muestras, p50 141.40 ms, p95 287.91 ms y 322.3 RPS. Cero errores. Ambas muestras cumplen la meta, con margen pequeño; no constituyen garantía ni comparación con otro stack. Los planes recientes recorren el índice en reversa y leen 25 filas sin Sort (~0.06–0.08 ms en esas consultas aisladas). Paridad de filtros/estado/empates/páginas entre las consultas vieja y nueva verificada en ambos tenants.

# ADR-0007 — Productos, SKU, precios y extracción

Fecha: 2026-10-22. Estado: aceptada para etapa 6. Cierra D-06 de productos.

## Contrato de producto

- SKU obligatorio: trim, 1–80 caracteres Unicode, UTF-8 válido y sin controles; conservar mayúsculas, acentos y espacios internos. No transliterar ni aplicar restricciones ASCII nuevas. La unicidad definitiva es `(tenant_id, active_sku)` en MySQL con utf8mb4_0900_ai_ci: equivalencias de mayúsculas/acentos/composición Unicode compiten dentro de la empresa. Nunca decidir unicidad solo con una consulta previa. Capturar error 1062 como error de campo SKU 422 sin exponer SQL. Otra empresa puede usar el mismo SKU; baja lógica permite reutilizarlo.
- Nombre obligatorio, trim, 1–180 caracteres Unicode, sin controles. Descripción opcional: CRLF/CR a LF, trim, máximo 5000 caracteres Unicode; permite LF/tab, rechaza otros controles; vacía a NULL. Estado obligatorio active/inactive.
- Precio de formulario `price`: MXN decimal sin separadores de miles, signo ni exponente; enteros o uno/dos decimales, punto decimal. Trim y normalizar a dos decimales. Convertir usando enteros, sin float ni redondeo silencioso: rango 0 a 92233720368547758.07 MXN, correspondiente a BIGINT con signo no negativo. Moneda de formulario debe ser MXN y se valida en servidor; sin selector multimoneda. Persistir price_cents BIGINT y currency MXN.
- Listado por SKU/nombre con búsqueda literal (trim, máximo 180 caracteres), filtro de estado, sort allowlisted created_desc/created_asc/name_asc/name_desc/price_asc/price_desc y desempate por UUID en misma dirección. Páginas 10/25/50, offset máximo 100000; lista y conteo en snapshot consistente, sin caché. CASE parametrizado con =/OR compatible con sqlc fijado.
- Rutas de arquitectura para productos, más GET `/{id}/delete` para confirmación sin JavaScript. Mismos contratos de clientes: HTML normal completo/POST 303; HTMX fragmento, creación 201 + detalle, edición 200 + detalle, baja 200 + listado y HX-Push-Url. Validación 422 con valores/errores por campo; UUID malformado, ajeno, ausente o eliminado: 404 tras comprobar permiso. CSRF, origen y cuerpo máximo 64 KiB para descripción Unicode.
- Owner/Admin mutan; Viewer solo consulta. Bloquear tenant y revalidar permiso dentro de transacción; mutación y auditoría de éxito atómicas, sin datos de campos en auditoría. Operador global no supera membresías comerciales. Baja lógica requiere confirm=yes; no restauración ni borrado físico desde UI.

## Extracción posterior

Primero implementar y validar productos usando vistas específicas, con regresión de clientes. Registrar tiempo UTC de ese incremento y del siguiente por separado. Solo después extraer a `internal/ui/components/` layout de recursos, navegación, avisos, errores/controles de formulario y estructura de tabla/paginación que realmente se repiten. Mantener campos, validación, permisos, consultas y servicios por dominio: no construir registro declarativo, ORM, generador ni framework genérico. Contratos HTTP/IDs de foco y filtros conservados y comprobados después de extraer.

## Esquema y límites

Sin migraciones ni dependencias nuevas; conservar 000001/000002. La restricción generada existente resuelve SKU concurrente. Pruebas sobre MySQL nativo y bases desechables para CRUD, permisos, IDOR, validación monetaria exacta, collation/unicidad/reutilización, filtros/orden/páginas y rollback. Dashboard/SSE permanecen en etapa 7; benchmark/carga/CI remoto no se declaran verificados por esta etapa.

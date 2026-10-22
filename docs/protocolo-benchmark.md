# Protocolo de medición y comparación

[ADR-0009](adr/0009-validacion-local-y-operacion.md) delimita la medición local. No hay implementación Laravel/Filament disponible y el usuario confirma validar Go; no se construye una contraparte nueva ni se declara comparación completada.

## Medición Go

Ejecutar `make bench` con MySQL nativo y sin otras pruebas, generación o carga en paralelo. El runner crea 2 empresas con 10 000 clientes y 10 000 productos cada una, además de 50 identidades Viewer y sesiones independientes (25 por empresa). Los usuarios de carga no admiten login; sus sesiones se preparan en el fixture. Se mide documento HTML completo, primera página de 25 filas y orden created_desc: clientes y productos por separado, 100 requests de calentamiento y 20 segundos de carga cerrada con 50 trabajadores sin think-time. No incluye login/Argon2id, escrituras, dashboard ni streams.

El binario de servidor no usa -race. Cliente de carga y servidor comparten host, al igual que MySQL: esta competencia y los servicios de fondo limitan las conclusiones. Informar Go/MySQL, CPU/RAM/sistema, número de registros, pool, índices, configuración, fecha UTC, warmup, duración, muestras, errores y percentiles con rango nearest-rank. RPS usa tiempo real hasta terminar las solicitudes iniciadas; reportar también pico de RSS/heap/sys del servidor muestreado cada 500 ms. El muestreo de métricas autenticadas y ps agrega una carga pequeña. No medir memoria del proceso de carga como si fuera Go servidor.

Informe privado reproducible `.local/benchmark-go.json`; resultados finales resumidos en validación de etapa 8. Un ensayo ejecutado junto con otras pruebas se identifica como exploratorio y no valida el umbral. Si se optimiza SQL, capturar EXPLAIN ANALYZE de la consulta real, guardar baseline aislada y repetir igual protocolo después; no presentar el mejor resultado de escenarios distintos. La meta p95 < 300 ms no es una garantía; errores/aislamiento importan además de latencia.

## Contraparte futura

Mismo alcance (sesiones/RBAC/tenants, validación/auditoría, CRUD/HTML, SSE), MySQL/dataset/hardware/calentamiento, 50 sesiones, páginas y duración. Documentar versiones Laravel/PHP/Filament/servidor y configuración de workers/OPcache, sin comparar un entorno dev con otro optimizado. Repetir ambos escenarios al menos tres veces, reportando dispersión, memoria/RPS/errores y aislamiento. Medir login/escrituras/SSE por separado si se añaden escenarios.

El esfuerzo del segundo CRUD y extracción está en registro-de-esfuerzo.md; corresponde a una sesión asistida y no equivale a horas humanas. Separar LOC Go/Templ/SQL escritos, generados, pruebas y configuración; conteos no demuestran productividad por sí solos. Sin contraparte y protocolo ejecutado en ambos, D-10 conserva pendiente la comparación.

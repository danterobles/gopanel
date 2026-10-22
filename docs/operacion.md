# Operación del piloto

Contrato: [ADR-0009](adr/0009-validacion-local-y-operacion.md). Desarrollo local nativo; no hay todavía destino público aprobado. No ejecutar varias instancias: los eventos SSE se guardan solo en memoria del proceso.

## Inicio y diagnóstico

Configurar `.env` privado según README. `make generate`, `make migrate-up`, `make build` y `make dev`. Seeds únicamente para desarrollo, no producción. Nunca usar root ni cuentas seed para un servicio público. Detener Go con Ctrl+C/SIGTERM; esperar el drenaje antes de reiniciar en el mismo puerto. MySQL continúa activo.

`/health/live` confirma proceso; `/health/ready` devuelve 503 si MySQL no responde, sin exponer detalles. Logs JSON a stdout: request_id aleatorio, método, patrón de ruta, status y duration_ms. Nunca registrar cuerpos, cookies, query strings, DSN o credenciales. Correlacionar el encabezado X-Request-ID con el log del incidente. Para SSE, duration_ms es vida del stream.

Platform Admin autenticado consulta `/platform/metrics`: solicitudes activas, contadores por método/ruta/status, suma de duraciones e histograma acumulativo con límites en milisegundos, memoria Go, goroutines, pool SQL y contadores globales SSE. El request actual figura activo y termina de contabilizarse al responder. Los contadores se reinician al reiniciar Go; no son una base de monitoreo persistente. Separar /app/events de la latencia de documentos. No usar identificadores de clientes/empresas como etiquetas.

Ante 503 revisar readiness, errores correlacionados, conexiones SQL en uso/WaitCount/WaitDuration y memoria. No aumentar el pool ni agregar caché sin medir. Ante SSE desconectado, refrescar manualmente; reconexión envía ready y recupera datos. Buffers/límites se fijan en ADR-0008. Un reinicio no pierde sesiones MySQL ni negocio, pero sí señales efímeras.

## Backup y ensayo de restauración

`make backup` usa mysqldump local y `.env`: snapshot consistente InnoDB, `--single-transaction`, sin tablespaces ni GTID, salida nueva 0600 en `.local/backups/`. El archivo contiene datos privados y hashes: excluir de Git y de publicaciones. No sobrescribe archivos existentes. También puede elegirse ruta explícita:

```sh
sh scripts/with-env.sh go run ./cmd/backup -output .local/backups/antes-de-migracion.sql
```

No ejecutar DDL durante el dump. El esquema actual no incluye rutinas/eventos programados; revisar opciones de exportación si se añaden. Credenciales MySQL van en archivo temporal privado eliminado al terminar, no en argv. El error del cliente se informa sin volcar SQL ni diagnósticos sensibles. Un error elimina la salida parcial creada por esa ejecución.

`make test-e2e` ensaya Export/RestoreTemporary con mysqldump/mysql reales. Crea origen/destino propios `gopanel_test_<32 hex>`, restaura en destino vacío, compara cada tabla por conteo/digest y prueba login sobre la copia. Rechaza goconcept y destinos no vacíos. RestoreTemporary es una herramienta interna para dumps confiables producidos por este ensayo, no un sandbox para SQL de terceros. No hay comando de rollback/restauración sobre la base de trabajo.

Antes de recuperar una instalación real: detener escrituras, verificar backup, restaurar primero en entorno aislado, comprobar versión de migración/constraints/digests/login/health y acordar el cambio de destino con el responsable. Conservar la base anterior hasta validar; no improvisar DROP o down sobre goconcept. Los tiempos medidos en el ensayo local no son RTO/RPO garantizados.

## Despliegue futuro de una instancia

Propuesta pendiente de un host/dominio concreto: proxy TLS → un proceso Go → MySQL privado con TLS verificable. `APP_ENV=production`, `DB_TLS=true`, cookie __Host- HttpOnly/Secure/SameSite=Lax y secretos por entorno. Mantener Host/origen canónicos; no confiar en encabezados de proxy arbitrarios ni publicar MySQL. La aplicación conserva CSRF en todos los POST.

Para /app/events: desactivar buffering/compresión del proxy que retenga frames, mantener timeout de lectura superior a 15 s de heartbeat y permitir conexiones largas. Probar SIGTERM/reconexión con el proxy real antes de ponerlo en servicio. Health solo expone estado; restringirlo a operación cuando el destino lo permita. /platform/metrics siempre requiere Platform Admin; no habilitar un endpoint anónimo de telemetría.

Mantener Compose como alternativa reproducible, sin publicarlo ni ejecutarlo para estas pruebas nativas. Aplicar migraciones explícitas antes del binario nuevo, con snapshot y ensayo de compatibilidad. No reescribir migraciones aplicadas. Para retirar etapas 7/8 puede usarse el binario anterior compatible con esquema 2; eso no revierte datos de negocio.

Antes de producción hay que aprobar destino, certificados, cuenta MySQL con privilegios mínimos, almacén de secretos, política de backup/cifrado externo y retención, RPO/RTO y restauración remota. Retención sugerida por ADR: 7 diarios/4 semanales; aún no hay scheduler ni copias externas operando. No se declara despliegue ni D-09 completamente verificado.

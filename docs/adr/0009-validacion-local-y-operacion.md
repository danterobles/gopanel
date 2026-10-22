# ADR-0009 — Validación local y operación del piloto

Fecha: 2026-10-22. Estado: aceptado para validación local de etapa 8. El usuario confirma que todavía no existe destino de despliegue ni contraparte Laravel/Filament.

## D-09: lo verificable y lo pendiente

Se valida Go/Node/MySQL nativos, con una sola instancia Go. E2E HTTP ejecuta binario real + MySQL + cookies/CSRF/HTML/HTMX/SSE; la comprobación de JavaScript/teclado/responsive se realiza en navegador aparte. No se incorpora un runner de navegador ni dependencia adicional. Cada prueba/carga/restauración crea bases gopanel_test_<UUID> y solo elimina sus propias bases. Nunca usa goconcept como destino de carga/restauración ni detiene MySQL.

Métricas operativas JSON en GET /platform/metrics, solo Platform Admin activo autenticado. No publicar /metrics anónimo. Contadores HTTP en memoria (telemetría, no caché comercial) por método normalizado/ruta Chi/status, histograma de duración acotado y solicitudes activas; no usar URL, query, tenant, usuario, token o request ID como etiqueta. Los logs ya correlacionan request ID aleatorio con ruta, status y duración. Métricas incluyen pool SQL, memoria Go y estado global del broker sin etiquetas de tenant. Contadores se reinician al reiniciar Go; SSE mide vida de conexión, no latencia de una página. No hay backend externo de telemetría.

Backups: mysqldump consistente InnoDB, sin tablespaces/GTID, archivo privado creado exclusivamente con modo 0600; credenciales en archivo temporal 0600, nunca argv ni salida. Restauración automatizada únicamente en destino temporal vacío con nombre gopanel_test_ validado. Ensayo compara tablas completas por conteo/digest y autentica contra la copia restaurada. Es un ensayo de recuperación local, no un backup remoto cifrado ni una política ya operando en producción.

Runbook propuesto para producción: una instancia, proxy TLS con buffering SSE desactivado y tiempos > heartbeat, cookie Secure, DB_TLS=true, credenciales mínimas y secretos privados, health interno y métricas autenticadas. Backup antes de migraciones y periódico con cifrado previo a copia externa; retención sugerida 7 diarios/4 semanales, por aprobar con el responsable del destino. Probar restauración en entorno aislado antes de reemplazar datos. El host/dominio/proxy definitivo, certificados, almacenamiento, RPO/RTO y ejecución remota quedan pendientes; no se despliega ni se declara D-09 completamente cerrado.

## D-10: protocolo y límites de comparación

Medir binario Go sin race, MySQL nativo y datos sintéticos: 2 tenants × 10 000 clientes y 10 000 productos, 50 identidades Viewer con sesiones independientes (25 por tenant), 25 filas/documento completo, orden created_desc/página 1. Excluir login/Argon2id del tiempo; precargar identidades/sesiones en fixture. Calentar antes de cada recurso; medir clientes y productos por separado, carga cerrada sin think-time durante 20 segundos con 50 trabajadores. Registrar p50/p95/RPS/errores/muestras, memoria del proceso Go, plataforma/hardware/versiones, tamaño de datos y configuración. Evaluar p95 < 300 ms como meta del PRD, sin ocultar fallos. No extrapolar a producción ni a escrituras/SSE/login.

No existe contraparte Laravel/Filament: se entrega protocolo reproducible y resultado Go. No se declara superioridad ni comparación cumplida. Una contraparte futura debe usar iguales historias/permisos/consultas/dataset/hardware/calentamiento/carga, indicar versiones y repetir ambas mediciones. Mantener separados esfuerzo asistido, código específico/configuración y código generado; el conteo de LOC no sustituye productividad ni comparación real.

## Cambios de esquema y herramientas

Sin migraciones previstas, sin nuevos servicios/dependencias/generadores. Python y Go estándar para herramientas; mysql/mysqldump nativos existentes. CI remota y Compose se conservan, pero no se ejecutan con Docker durante esta validación local. Los pendientes externos no impiden cerrar los incrementos locales ni autorizan construir un segundo producto Laravel.

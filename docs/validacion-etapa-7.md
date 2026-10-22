# Validación de etapa 7 — Dashboard y SSE

Fecha: 2026-10-22. Go 1.26.10, Node 22.23.2 y MySQL 8.0.40 nativos, macOS ARM64. Sin Docker ni nuevas dependencias. D-07/D-08 cerradas en [ADR-0008](adr/0008-dashboard-utc-y-ciclo-sse.md). Historias US-07/08 implementadas y verificadas localmente.

## Cambios

- `internal/modules/dashboard/{service.go,handler/,repository/}`, `internal/db/queries/dashboard.sql` y SQL generado: autorización, snapshot REPEATABLE READ, conteos y serie de 30 días calendario UTC con ceros. Ventana desde 00:00 del día actual menos 29 días hasta el instante consultado, excluido. Activos sin bajas; recientes de cualquier estado, sin bajas ni futuros.
- `internal/ui/pages/dashboard.templ` y generado: tarjetas, gráfica SVG con tabla desplegable accesible, carga/vacío/error, refresco parcial/manual y navegación/logout. Layout compartido admite scripts locales opcionales; SSE se carga únicamente aquí. El marcador público X-Dashboard-Scope solo causa recarga si el actor cambió, nunca autoriza ni determina el tenant.
- `internal/platform/events/{broker.go,handler/}`: broker efímero por tenant, sin datos comerciales ni cola. 256 conexiones, 32 por tenant, 4 por usuario, buffer 16. Cliente lento se desconecta sin bloquear publicación y conserva su cupo hasta salir del handler. Contexto de apagado libera streams; escrituras acotadas mediante ResponseController.
- Servicios customers/products: señales solo después de commit exitoso de mutación y auditoría. Payload invalidate con type/resource/version UUID opaco; sin tenant, actor, registro o PII. ready en toda conexión fuerza GET de métricas; Last-Event-ID no habilita replay.
- Auth: autenticación pasiva sin touch y consulta SQL parametrizada StreamAuthorization; SSE revalida cada 2 segundos y antes de enviar. Cierre por expiración, inactividad, logout, cambio de empresa, revocación de membresía/tenant/usuario y fallo de autorización. Origen cruzado rechazado; 401/403 inicial sin redirección HTML. Error DB corta sin datos y permite reconectar. Widgets pasivos no mantienen la sesión viva.
- `cmd/server/main.go`, fixture HTTP y `tests/integration/dashboard_sse_test.go`: registro y conexión de módulos, servidor de prueba con los mismos timeouts reales. Documentos PRD/arquitectura/datos/README/plan/aceptación actualizados.

## Comprobaciones

| Verificación | Resultado |
|---|---|
| `make generate`, `make fmt`, `make lint`, `make test`, `make build` | Correctos; assets/sqlc/Templ locales, gofmt y go vet |
| Generación repetida + SHA-256 | 21 archivos generados y assets idénticos |
| `make test-integration` | Suite completa correcta, incluida regresión de identidad/RBAC/clientes/productos |
| `sh scripts/with-env.sh go test -race -tags=integration ./... -count=1` | Suite completa correcta; sin carreras detectadas |
| Repetición focalizada con `-race` tras ajustes de contexto/cupo | Dashboard/SSE y broker correctos |
| `make smoke smoke-auth smoke-rbac` | Health, assets, sesión persistida tras reinicio, CSRF y roles correctos |
| Navegador local | Creación desde segunda pestaña cambió automáticamente 30→31 clientes y productos; reinicio Go conservó sesión y produjo nuevo refresco SSE; cambio a Sur/Viewer renovó ambas cabeceras y conteos; refresco manual y tabla diaria por Enter correctos |
| Visual/responsive | Escritorio 1280 px y tablet real 768×1024; clientWidth y scrollWidth 768, sin desbordamiento horizontal; capturas inspeccionadas |

Cuatro pruebas unitarias nuevas: ventana UTC (incluido bisiesto/cambio de zona) y tres del broker con aislamiento, límites, overflow, cupo retenido, cancelación idempotente, liberación y concurrencia. Seis pruebas de integración nuevas, con siete subcasos de revocación. Cubren fronteras de microsegundos y exclusión de futuros/bajas, snapshot, ceros, tenant forjado, Viewer/operador, documento/fragmento y error DB; envío posterior a auditoría visible, CRUD de ambos recursos, silencio por rollback/CSRF/validación, aislamiento de streams y reconexión; ausencia de touch y todos los cierres de acceso; capacidad, shutdown y heartbeat que supera los 15 segundos del WriteTimeout normal. La repetición con carreras usa los ajustes finales de scope y backpressure.

Evidencia local ignorada por Git: `.local/stage7-{generate,unit,integration,race,final-focused-race,broker-race,smoke}.log`, `.local/stage7-generated-hashes.json`, `.local/stage7-dashboard-{desktop,tablet,viewer}.png`. Datos de las capturas sintéticos. Fixtures de navegador cerrados, sesiones terminadas, viewport restablecido y bases/credenciales temporales eliminadas. No se detuvo MySQL ni se hicieron mutaciones comerciales de prueba en `goconcept`.

## Migraciones, límites y siguiente etapa

Ninguna migración nueva: esquema versión 2 compatible. Para retirar esta etapa basta el binario previo; no aplicar down destructivo a la base de trabajo. Cambiar binario no revierte mutaciones de negocio.

SSE es efímero y limitado a una instancia: caída entre commit y publish puede perder una señal; ready/refresco manual recuperan el estado desde MySQL. No se garantiza reproducción ni orden global. La revocación se detecta en el siguiente chequeo (2 s más consulta/escritura acotadas), no atómicamente con cada commit externo; los GET autorizan de nuevo. No hay publicación para escrituras hechas directamente por SQL o desde otro proceso.

No se ejecutaron Docker, CI remota, carga, auditoría WCAG formal, suite E2E operativa ni comparación Laravel/Filament. La revisión manual en navegador complementa las pruebas HTTP, no sustituye esa suite futura. Siguiente etapa 8: observabilidad/E2E/carga, operación y protocolo de benchmark, cerrando D-09/D-10 antes de desplegar o comparar. No se inicia etapa 8 aquí.

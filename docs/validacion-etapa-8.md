# Validación de etapa 8 — cierre local

Fecha: 2026-10-22. El usuario confirmó validar Go localmente: todavía no hay destino de despliegue ni implementación Laravel/Filament. Se completa el alcance verificable local, sin declarar despliegue, comparación ni certificación WCAG. D-09/D-10 quedan parcialmente resueltas por [ADR-0009](adr/0009-validacion-local-y-operacion.md), conservando los pendientes externos.

## Entrega y archivos

- `internal/platform/observability/{recorder.go,handler/}`, router HTTP, broker SSE y enlace de plataforma: métricas JSON protegidas por Platform Admin, etiquetas acotadas, histogramas, solicitudes activas, memoria/pool/conexiones. Sin usuarios/tenant/query/token como etiquetas. La finalización en defer libera el gauge incluso si se aborta el request; el aborto sin cabeceras se contabiliza como error 500 de instrumentación, sin convertirlo en respuesta al cliente.
- `internal/platform/backup/`, `cmd/backup/`: exportación consistente privada y restauración interna exclusivamente en bases temporales vacías. Opciones/credenciales en archivo temporal 0600, solo ese defaults-file; salida exclusiva 0600 y eliminación de parcial en error. Nunca sobrescribir backup existente, goconcept ni destino restaurado no vacío.
- `tests/e2e/`: binario compilado real, MySQL temporal y recorrido HTTP completo (incluye HTMX/SSE), reinicio/sesión persistida, permisos y revocación tras gestión de miembros; dump/restauración con comparación de las nueve tablas por conteo/digest y login sobre la copia. No es un runner automatizado de navegador.
- `Makefile`, CI y `scripts/count-loc.py`: test-e2e, bench, backup, reporte de LOC y propuesta de ejecutar E2E/recuperación en CI. Artefactos CI explícitos e2e-result/backup-verification, nunca dumps ni credenciales. CI remota no ejecutada.
- Queries/repositorios customers/products y generado: orden reciente estático allowlisted sobre índice existente, tras EXPLAIN ANALYZE. [ADR-0010](adr/0010-orden-reciente-medido.md), prueba de paridad con filtros/estado/empates/páginas/tenants y [resultado medido](benchmark-go.md).
- Guías: [cuentas y pruebas](pruebas-locales.md), [operación](operacion.md), [protocolo comparativo](protocolo-benchmark.md). PRD, arquitectura, datos, pendientes, plan, aceptación y README reflejan este cierre local.

## Verificaciones

| Comprobación | Resultado |
|---|---|
| Generación y repetición SHA-256 | sqlc/Templ/assets reproducibles |
| `make fmt lint test build` | Correcto: formato, vet, unitarias y binario |
| `go test -race -tags=integration ./... -count=1` con entorno privado | Suite completa correcta tras optimización, incluidas métricas, CRUD/RBAC/sesiones/SSE y paridad SQL |
| Repetición final `-race` en HTTP/observabilidad/backup | Correcta; incluye aborto de request y protección de archivos/credenciales |
| `make test-e2e` | Correcto: proceso real y ensayo de restauración autenticable |
| `make smoke smoke-auth smoke-rbac` | Correcto; salud/assets/login/reinicio/CSRF/roles nativos |
| `make bench`, aislado | 50 usuarios; p95 clientes 286.38 ms y productos 287.91 ms, cero errores, tras baseline >600 ms; límites en resultado/protocolo |
| `python3 scripts/count-loc.py` | Ejecutado, distingue fuentes y generado, sin equiparar dominio con DSL |
| Navegador | Enter para login/enlaces/tabla diaria, métricas JSON de operador, Admin Norte/Viewer Sur, validación monetaria con valores preservados y aria-invalid/describedby, cancelar/logout |
| Responsive | 1280 px escritorio y 768×1024 tablet, clientWidth=scrollWidth=768; capturas inspeccionadas |

Datos sintéticos; E2E, carga y restauración crearon/eliminaron únicamente sus bases gopanel_test_<UUID>. No se detuvo MySQL ni se ejecutó down/restauración/carga contra goconcept. Sus ocho cuentas locales se consultaron sin imprimir contraseñas. Los smoke de la base de trabajo solo cambian sesiones/contadores/auditoría de seguridad; no negocio. Fixtures de navegador terminaron sesión, se cerraron y eliminaron credenciales/base; viewport restablecido.

Artefactos locales privados ignorados por Git: `.local/stage8-*.log`, `.local/stage8-generated-hashes.json`, `.local/e2e-result.json`, `.local/backup-verification.json`, `.local/benchmark-*.json`, `.local/loc-report.json`, `.local/stage8-platform.png`, `.local/stage8-tablet.png`. La revisión de accesibilidad es manual en flujos críticos, no auditoría completa WCAG. Los informes de backup solo contienen conteos/digests y duración, no filas/credenciales; el tiempo del ensayo pequeño no es RTO de producción.

## Migraciones y límites de cierre

Sin migraciones nuevas ni cambios a migraciones aplicadas; esquema 2. No se añadieron dependencias de aplicación o servicios. mysql/mysqldump nativos 8.0.40 ya estaban instalados. Para retirar esta versión, usar binario anterior compatible con esquema 2; eso no revierte operaciones de negocio. No ejecutar rollback destructivo de goconcept.

No se ejecutaron Docker ni CI remota. Producción requiere definir/aprobar host/dominio/proxy/TLS, cuenta MySQL mínima, secretos, backup cifrado externo/retención y RPO/RTO; no se han instalado scheduler, certificados o copias externas. Comparación Laravel/Filament pendiente por ausencia de contraparte, con protocolo entregado. No se afirma MVP operativo público ni comparación completa. Próximo paso: probar con las cuentas locales y acordar destino y contraparte cuando existan.

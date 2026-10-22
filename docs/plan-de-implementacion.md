# Plan de implementación por etapas

Etapas 0–3 autorizadas. Fecha: 2026-10-21. La etapa 3 incorpora identidad, CSRF y selección de tenant sobre MySQL nativo. No se inicia etapa 4 en esta entrega. Las fases orientativas del PRD agrupan estas etapas y su estimación se recalibrará con evidencia del bootstrap.

Evidencia de bootstrap: [validación de etapa 1](validacion-etapa-1.md). La ejecución remota de CI queda pendiente; no se confunde con las comprobaciones locales.

## Secuencia y entregables

| Etapa | Archivos/entregables previstos | Migraciones | Verificación para cerrar |
|---|---|---|---|
| 0. Conciliación | PRD, ARQUITECTURA, DATA, AGENTS; `docs/adr/0001-*`, este plan, registro de decisiones y matriz de aceptación | Ninguna | Contradicciones registradas antes de corregirse, enlaces locales válidos, historias US-01 a US-09 trazadas, diff revisado y commit local |
| 1. Fundaciones | `go.mod`, `cmd/server/`, `internal/config/`, `internal/platform/http/`, conexión mínima DB para readiness, Dockerfile, `compose.yaml`, Makefile, CI, README y herramientas fijadas | Ninguna de negocio | D-01 cerrada; configuración válida/inválida, health checks, apagado ordenado, build/CI y arranque Compose reproducibles |
| 2. Persistencia | `migrations/`, `internal/platform/database/`, `internal/db/queries/`, `internal/db/generated/`, configuración sqlc, seeds y pruebas de integración | Inicial reversible: users, tenants, memberships, customers, products, audit_logs y sessions | MySQL 8 real; up/down/up, constraints, transacciones, consultas aisladas y seeds de dos tenants |
| 3. Identidad y empresa activa | `internal/platform/auth/`, `tenancy/`, handlers y páginas Templ mínimas de login/selector; CSRF y rate limit | Solo aditivas si una decisión justifica cambiar el esquema | D-02/D-03 cerradas; US-01/02, caducidad, revocación y persistencia tras reinicio |
| 4. Permisos y miembros | `internal/platform/rbac/`, módulos tenants/members, políticas, transacciones y auditoría | Solo si las operaciones acordadas lo requieren | D-04/D-05 cerradas; matriz DATA, US-03, IDOR y concurrencia del último Owner |
| 5. Clientes | Módulo customers; layout y vistas Templ/HTMX, SQL parametrizado y auditoría | Solo necesidades verificadas; no reescribir migración inicial | D-06 para clientes; US-04/05, CRUD completo, fragmentos, validación y aislamiento |
| 6. Productos y extracción | Módulo products; reutilización en `internal/ui/components/` tras ambos CRUD; registro de esfuerzo | Solo aditivas justificadas | D-06 para productos; US-05/06, SKU concurrente, centavos/MXN y baja lógica |
| 7. Dashboard y SSE | Módulo dashboard, consultas agregadas, widgets, `internal/platform/events/`, endpoint SSE y extensión local | Ninguna cola persistente; nuevas migraciones solo con justificación | D-07/D-08 cerradas; US-07/08, aislamiento, commit/rollback, carreras y reconexión |
| 8. Validación operativa | `tests/e2e/`, carga reproducible, logs/métricas, runbooks, backup/restauración y benchmark | Solo ajustes medidos y reversibles | D-09/D-10 cerradas según alcance; US-09 y regresión US-01–09, accesibilidad, carga y comparación documentada |

## Forma de ejecución

1. Antes de cada tarea no trivial, identificar archivos, cambios de esquema y pruebas concretas. Revisar los ADR y pendientes que la condicionan.
2. Implementar incrementos verticales verificables. Las vistas mínimas de autenticación no requieren construir antes un sistema genérico de componentes.
3. Ejecutar formato, generación, pruebas y análisis aplicables; reportar expresamente herramientas o servicios que no pudieron ejecutarse. No marcar pruebas futuras como aprobadas.
4. Actualizar documentación y cerrar con commit descriptivo local, únicamente de los cambios de la tarea, informando hash y rama. Sin push automático.
5. Registrar evidencia en relación con [la matriz de aceptación](matriz-de-aceptacion.md). Un cambio de alcance requiere una decisión explícita; los pendientes no autorizan funciones nuevas.

## Riesgos y dependencias

- El aislamiento depende de filtros y políticas en servidor: MySQL no aplica RLS para este diseño. Probar cada repositorio y ruta comercial entre tenants.
- SSE solo funciona dentro de una instancia y no es durable; refrescar desde MySQL permite recuperar el estado.
- La ausencia de contraparte Laravel/Filament impide concluir el benchmark comparativo, aunque permite medir Go.
- Valores de política y herramientas todavía abiertos tienen etapas límite en [decisiones pendientes](decisiones-pendientes.md); no bloquean documentación ni trabajo independiente.

La siguiente tarea recomendada es la etapa 4: RBAC por acción/recurso y gestión de miembros. D-02/D-03 quedan cerradas por ADR-0004; D-04/D-05 deben precisarse antes de esas operaciones. Ver [validación de etapa 3](validacion-etapa-3.md).

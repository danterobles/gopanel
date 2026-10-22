# Matriz de aceptación y pruebas

Fuente: PRD, ARQUITECTURA y DATA conciliados mediante [ADR-0001](adr/0001-conciliacion-de-contratos-poc.md). Fecha: 2026-10-21.

Esta matriz define escenarios objetivo. La etapa 0 revisó trazabilidad; etapas 1 y 2 aportan evidencia parcial en [validación de etapa 1](validacion-etapa-1.md) y [validación de etapa 2](validacion-etapa-2.md). Se ejecutaron pruebas de bootstrap y persistencia/aislamiento SQL, constraints, transacciones, sesiones persistidas y concurrencia del último Owner. Login, autorización/CSRF HTTP, CRUD E2E, dashboard y SSE siguen planificados. La aceptación completa de una historia requiere todos sus escenarios y los criterios generales de AGENTS; las pruebas SQL no sustituyen las de HTTP.

## Datos y niveles

- Dos tenants: `empresa-norte` y `empresa-sur`; Owner por empresa, Admin, Viewer, un usuario en ambas con roles distintos y Platform Admin sin membresía comercial.
- Recursos con valores compartidos entre tenants, membresías/usuarios/tenants inactivos, sesiones válidas/vencidas/revocadas y bajas lógicas.
- Seeds de desarrollo: 100 clientes y 100 productos por tenant. Carga: 10 000 registros por tenant, especificando distribución y tipo de recurso en el protocolo. Credenciales generadas fuera del repositorio.
- Unitarias para validadores/políticas; integración contra MySQL 8 real para SQL/constraints/transacciones; HTTP para middleware, CSRF y representaciones; E2E para flujos completos.

## Historias

| Historia / etapa | Caso positivo | Negativos y aislamiento | Evidencia prevista |
|---|---|---|---|
| US-01 — Identidad / 3 | Login crea cookie y sesión persistida; logout revoca; reiniciar conserva una sesión aún válida | Error genérico para cuenta inexistente/contraseña incorrecta; usuario inactivo denegado; CSRF incluso en login/logout; límites absoluto/inactividad; cookie manipulada o token revocado no autentican; no guardar token en claro | Unitarias con reloj controlado, integración sesiones, HTTP cookies/CSRF/rate limit y E2E login/logout |
| US-02 — Selección / 3 | Listar solo membresías activas de tenants activos; cambiar empresa conserva rol propio de cada membresía | Rechazar tenant candidato ajeno/inactivo y campos de actor/rol manipulados; membresía revocada deja de autorizar; datos de Norte nunca aparecen en Sur | Integración de membresías/sesión, HTTP selector y peticiones posteriores, E2E con usuario compartido |
| US-03 — Membresías / 4 | Owner asigna Admin/Viewer bajo la política acordada; Admin consulta membresías según DATA | Admin/Viewer no administran; IDOR de miembro ajeno; CSRF; cambios concurrentes nunca dejan tenant sin Owner activo; actor no obtiene rol del formulario; inactivar usuario Owner también debe respetar la invariante si se habilita esa operación | Unitarias de políticas, integración de transacciones y bloqueo de tenant, HTTP y E2E |
| US-04 — Clientes / 5 | Crear, consultar detalle, editar y dar de baja; HTMX actualiza fragmento; HTML normal navega con respuesta semántica; auditoría con actor y tenant | Validación visible sin escrituras inválidas; Viewer denegado; recurso ajeno/ausente/eliminado devuelve 404 genérico; CSRF; guardar sin cambios no produce falso 404; filtros/orden solo allowlist | Unitarias, repositorio MySQL, HTTP con/sin `HX-Request`, E2E teclado/formulario y auditoría |
| US-05 — Lecturas y Viewer / 5–6 | Buscar, filtrar, ordenar y paginar clientes/productos con tamaños 10/25/50; conservar parámetros en fragmentos | HTTP manual de Viewer no muta; falsificar `HX-Request` no omite autorización; límites y entradas inválidas; `%`, `_` y barra invertida se buscan literalmente; páginas nunca mezclan tenants | Parsers/políticas unitarios, SQL de listados y conteos, HTTP completo/parcial y E2E |
| US-06 — Productos / 6 | CRUD con MXN y precio entero en centavos; mismo SKU en tenants distintos; reutilización tras baja lógica | Duplicado en tenant rechazado incluso con escrituras concurrentes; collation/normalización según ADR; precio negativo, moneda ajena y campos inválidos rechazados; IDOR, CSRF, Viewer y edición sin cambios | Unitarias de validación, constraints MySQL/concurrencia, HTTP y E2E |
| US-07 — Dashboard / 7 | Contar clientes/productos activos, clientes de 30 días y serie diaria del tenant; refresco manual/parcial | No sumar otro tenant ni bajas lógicas; ventana temporal según ADR; días sin clientes y estados vacío/error; acceso sin membresía denegado | Integración de agregados con fechas controladas, HTTP y E2E |
| US-08 — SSE / 7 | Commit publica invalidación al tenant correcto; el navegador refresca; desconectar/reconectar recupera estado de MySQL | Rollback no publica; otro tenant no recibe; payload sin PII; revocación/caducidad/cambio de tenant según D-08; limpiar suscriptores, limitar clientes lentos; reinicio pierde señales pero no datos | Pruebas concurrentes del broker con detector de carreras, HTTP streaming, integración commit/rollback y E2E reconexión |
| US-09 — Operación / 1 y 8 | Liveness del proceso, readiness dependiente de MySQL; request ID y logs correlacionables; métricas básicas | Caída/timeout de MySQL afecta readiness sin filtrar DSN, secretos o PII; errores controlados; endpoints operativos según política de despliegue | HTTP health/logging, integración de fallo DB, arranque Compose y revisión de logs/métricas |

## Permisos de referencia

La matriz normativa de acciones está en [DATA, sección 6](../DATA.md#6-matriz-de-permisos). Debe convertirse en pruebas tabulares para Owner/Admin/Viewer por acción, con membresía activa y tenant correcto. Platform Admin no supera políticas comerciales por su rol global; si tiene membresía, usa los permisos de esa membresía.

En recursos ajenos, el contrato es 404 genérico. Cuando la acción está prohibida sin necesidad de revelar un recurso, usar 403. El orden de comprobaciones no debe permitir inferir existencia entre empresas. Ningún header HTMX otorga permisos.

## Criterios transversales de salida

| Área | Evidencia requerida / etapa |
|---|---|
| Migraciones | Up/down/up sobre base desechable, constraints e índices verificados; documentar rollback y advertir pérdida de datos cuando corresponda / 2 y cada nueva migración |
| Seguridad | Batería multi-tenant y CSRF en cada mutación, incluidos formularios normales y HTMX; consultas parametrizadas; auditoría de resultados sensibles sin secretos / 3–8 |
| Consistencia | Mutación de negocio y auditoría de éxito atómicas; rollback no registra éxito ni emite SSE; probar manejo de denegaciones/errores sin filtrar datos / 4–7 |
| Interfaz | Teclado, foco, etiquetas, validación y estados vacío/error/carga, confirmación de baja y escritorio/tablet; auditoría de flujos críticos respecto al objetivo WCAG 2.2 AA / 3–8 |
| Herramientas | `make generate`, `make test`, `make lint`, `make build`; cubrir `go test ./...`, `go vet ./...`, gofmt y generación reproducible; detector de carreras para concurrencia / desde 1 según componentes existentes |
| Operación | Arranque desde cero con Compose, apagado ordenado, reinicio con sesiones válidas, backup/restauración ensayada y runbook / 1–8 |
| Rendimiento | Dataset 10 000/tenant, 25 filas/página, 50 usuarios concurrentes; p50/p95, RPS, CPU/memoria y entorno documentados; evaluar meta p95 < 300 ms / 8 |
| Comparación | Medir segundo CRUD y extracción por separado; comparación Laravel/Filament equivalente según D-10, sin conclusiones si falta contraparte / 5–8 |

## Registro de ejecución futuro

Cada cierre de tarea debe citar historias/casos cubiertos, comando y resultado, limitaciones, migraciones/rollback y commit. Una prueba planificada, una revisión de código o la compilación por sí sola no equivalen a aceptar una historia.

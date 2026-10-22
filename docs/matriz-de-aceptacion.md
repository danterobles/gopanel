# Matriz de aceptación y pruebas

Fuente: PRD, ARQUITECTURA y DATA conciliados mediante [ADR-0001](adr/0001-conciliacion-de-contratos-poc.md). Fecha: 2026-10-21.

Esta matriz define escenarios objetivo. La etapa 0 revisó trazabilidad; etapas 1 y 2 aportan evidencia parcial en [validación de etapa 1](validacion-etapa-1.md) y [validación de etapa 2](validacion-etapa-2.md). Se ejecutaron pruebas de bootstrap y persistencia/aislamiento SQL, constraints, transacciones, sesiones persistidas y concurrencia del último Owner. Etapa 3 añade [evidencia de login/logout, CSRF y selección de empresa](validacion-etapa-3.md), con HTTP normal/HTMX y reinicio real del proceso. Etapa 4 aporta [RBAC, miembros, configuración y plataforma](validacion-etapa-4.md): matriz tabular, HTTP manual normal/HTMX, concurrencia de Owners, revocación y rollback de auditoría; smoke nativo y revisión de navegador con teclado/escritorio/tablet. Etapa 5 añade [evidencia de clientes](validacion-etapa-5.md): US-04 y parte de clientes de US-05, CRUD HTML/HTMX, aislamiento, filtros, orden, paginación y auditoría; recorrido de navegador con datos desechables. Productos, dashboard, SSE y batería E2E operativa integral siguen planificados. La aceptación completa de una historia requiere todos sus escenarios y los criterios generales de AGENTS; las pruebas SQL no sustituyen las de HTTP.

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

## Evidencia de clientes (etapa 5)

[Validación de etapa 5](validacion-etapa-5.md) registra pruebas unitarias de campos/parsers/UUID y cinco pruebas de integración HTTP/servicio nuevas, además de regresión SQL existente. Incluye Owner/Admin permitidos, Viewer denegado en HTTP manual, operador sin membresía sin acceso comercial, IDs ajenos/inexistentes/malformados/eliminados, CSRF/origen/límite de cuerpo, Unicode/NULL/escape HTML, orden/filtros/paginación y rollback de auditoría. Prueba concurrente verifica que degradar a Viewer mientras espera una edición impida esa edición. US-05 de productos se valida en etapa 6.

## Registro de ejecución

Cada cierre de tarea debe citar historias/casos cubiertos, comando y resultado, limitaciones, migraciones/rollback y commit. Una prueba planificada, una revisión de código o la compilación por sí sola no equivalen a aceptar una historia.

## Evidencia de productos y extracción (etapa 6)

[Validación de etapa 6](validacion-etapa-6.md) registra US-05/06: tres pruebas unitarias y cuatro pruebas de integración nuevas, además de la regresión completa de clientes. Incluye SKU equivalente por collation y concurrente, repetición entre empresas, reutilización tras baja, precio exacto y límite BIGINT, filtros/seis órdenes/páginas, autorización/IDOR/CSRF, rollback de auditoría y degradación concurrente del rol. Ambas baterías pasan después de extraer los componentes visuales compartidos, también con detector de carreras. Navegador verifica creación/edición, validación monetaria, confirmación, orden/paginación, Viewer y navegación entre módulos; el cierre operativo E2E/carga y el benchmark siguen en etapa 8.

## Evidencia de dashboard y SSE (etapa 7)

[Validación de etapa 7](validacion-etapa-7.md) registra US-07/08: fronteras calendario UTC y futuro, estados/bajas, snapshot y ceros diarios, filtros de tenant ignorando campos forjados, HTML/HTMX y estado de error. Broker con límites por usuario/tenant/global, buffer lleno, cancelación idempotente y carreras. HTTP real verifica señales sin PII tras COMMIT, silencio ante rollback/CSRF/validación, aislamiento, ready y Last-Event-ID ignorado, expiración/logout/rotación/revocación, ausencia de touch, capacidad, fallo DB y heartbeat superando WriteTimeout. Navegador comprueba actualización desde otra pestaña, reconexión tras reiniciar Go, Viewer, cabecera consistente al rotar empresa, teclado y responsive. E2E operativa, carga y comparación quedan en etapa 8.

## Evidencia operativa local (etapa 8)

[Reporte](validacion-etapa-8.md): regresión completa con carreras, E2E HTTP del binario/SQL/cookies/CSRF/RBAC/mutaciones/SSE/reinicio, gestión de miembros y revocación, métricas de operador sin PII y ensayo de backup/restauración autenticable. Revisión de navegador/teclado/errores/responsive en flujos críticos. US-09 cuenta con health/logs/métricas; no equivale a operación pública monitoreada. Carga de 50 identidades/10 000 registros por recurso/empresa: p95 286.38/287.91 ms tras optimización medida, cero errores en esa muestra. No se extrapola a otros escenarios. Pendientes D-09/D-10 externos, CI remota/Compose y auditoría WCAG formal identificados; no se declara comparación ni despliegue completos.

# PRD — GoPanel Pilot

**Versión:** 1.6 · **Estado:** CRUD de productos implementado; extracción de etapa 6 en curso · **Producto:** SaaS administrativo multiempresa de referencia

Contratos conciliados en [ADR-0001](docs/adr/0001-conciliacion-de-contratos-poc.md). Seguimiento: [plan por etapas](docs/plan-de-implementacion.md), [matriz de aceptación](docs/matriz-de-aceptacion.md) y [decisiones pendientes](docs/decisiones-pendientes.md).

## 1. Visión
Construir una aplicación real de administración multiempresa para validar el stack Go + Chi + Templ + HTMX + Tailwind + MySQL 8 (con broker SSE en memoria, una sola instancia Go y sin Redis ni caché de aplicación) y extraer, posteriormente, componentes reutilizables de tablas, formularios y dashboards. El piloto **no** pretende reproducir Filament completo.

## 2. Problema y oportunidad
Los sistemas empresariales repiten autenticación, navegación, CRUD, filtros, tablas, autorizaciones y métricas. Se busca comprobar si un enfoque server-rendered reduce complejidad operativa sin sacrificar productividad, seguridad o experiencia de usuario.

## 3. Usuarios y roles
- **Platform Admin:** administra tenants y observa métricas técnicas; no accede automáticamente a datos comerciales de tenants.
- **Tenant Owner:** administra miembros, roles permitidos y recursos de su empresa.
- **Tenant Admin:** administra clientes, productos y consulta dashboard.
- **Tenant Viewer:** consulta clientes, productos y dashboard sin mutaciones.
Un usuario puede pertenecer a varias empresas con distintos roles.

## 4. Alcance MVP
### 4.1 Identidad y tenants
- Inicio/cierre de sesión con correo y contraseña.
- Sesiones seguras, expiración, invalidación en logout.
- Selector de empresa entre membresías activas; recordar tenant activo en sesión.
- Alta inicial de tenant por flujo administrativo o seed; invitaciones por correo quedan fuera de MVP.
- Gestión de membresías por Owner; no permitir eliminar el último Owner activo. Alta por correo exacto de cuenta existente activa, asignación Admin/Viewer y cambio de estado. Owner inicial por seed o alta administrativa de empresa; sin transferencia de propiedad ni promoción a Owner desde la gestión de miembros (ADR-0005).
- Owner configura el nombre de su empresa. Platform Admin crea empresas con Owner existente, cambia nombre/estado y consulta métricas técnicas del pool MySQL, sin membresía comercial automática.

### 4.2 Autorización
- RBAC por membresía: Owner, Admin, Viewer; Platform Admin por rol global independiente.
- Políticas en servidor por ruta, acción y registro.
- Errores 403 y 404 sin filtraciones de datos de otros tenants.

### 4.3 CRUD de clientes
- Campos: nombre, correo opcional, teléfono opcional, estado activo/inactivo, notas opcionales.
- Listado, alta, detalle, edición y baja lógica.
- Búsqueda por nombre/correo; filtro por estado; orden por nombre/fecha; paginación 10/25/50.
- Validación de campos y mensajes de error según [ADR-0006](docs/adr/0006-clientes-validacion-y-flujos-http.md): nombre 1–180, correo opcional hasta 254 bytes, teléfono opcional hasta 40 caracteres y notas opcionales hasta 5000; trim, normalización y opcionales vacíos a NULL. Confirmación de baja disponible sin JavaScript.

### 4.4 CRUD de productos
- Campos: SKU único por tenant, nombre, descripción, precio en centavos, moneda MXN, estado activo/inactivo.
- Listado, alta, detalle, edición y baja lógica.
- Búsqueda por SKU/nombre; filtro por estado; orden por nombre/precio/fecha; paginación 10/25/50.
- [ADR-0007](docs/adr/0007-productos-sku-precios-y-extraccion.md): SKU trim de 1–80 caracteres Unicode conservando caso/acentos; unicidad con collation MySQL. Precio decimal MXN convertido a centavos enteros sin redondeo; descripción opcional hasta 5000 caracteres y confirmación de baja sin JavaScript.

### 4.5 Dashboard
- Tarjetas: total clientes activos, total productos activos, clientes creados en últimos 30 días.
- Gráfica simple: clientes creados por día en últimos 30 días.
- Refresco parcial HTMX; estados vacío/error.
- Actualización SSE al cambiar clientes/productos en el tenant activo (evento de invalidación, no datos sensibles).

### 4.6 Experiencia y operaciones
- Interfaz responsive para escritorio y tablet; navegación con teclado y etiquetas accesibles.
- Estados de carga, vacío, error, confirmación de eliminación y notificaciones.
- Health checks, logs estructurados, métricas básicas y auditoría de mutaciones.

## 5. Fuera de alcance
Pagos, facturación, inventario avanzado, pedidos, API pública, apps móviles, importaciones masivas, reportes PDF, plugins, personalización visual por tenant, MFA, SSO, permisos de campo, editor drag-and-drop y recuperación automática de contraseña (diferida mediante ADR-0004).

## 6. Historias de usuario y aceptación
| ID | Historia | Criterio verificable |
|---|---|---|
| US-01 | Como usuario ingreso a mi cuenta | Credenciales válidas crean sesión; inválidas no revelan existencia de cuenta |
| US-02 | Como miembro cambio de empresa | Solo aparecen membresías activas; el contexto se aplica a todas las consultas |
| US-03 | Como Owner gestiono miembros | Puedo asignar Admin/Viewer; no puedo dejar tenant sin Owner |
| US-04 | Como Admin creo/edito clientes | Validación, persistencia, auditoría y vista actualizada sin recarga completa |
| US-05 | Como Viewer consulto listados | Puedo filtrar y paginar; no puedo crear, editar ni eliminar, aun con HTTP manual |
| US-06 | Como Admin gestiono productos | SKU duplicado dentro del mismo tenant falla; entre tenants puede repetirse |
| US-07 | Como miembro veo dashboard | Solo muestra métricas de mi tenant |
| US-08 | Como miembro veo cambios recientes | Evento SSE dispara refresco parcial, sin exponer información de otro tenant |
| US-09 | Como operador reviso salud | Endpoints de salud no filtran secretos; logs correlacionables |

## 7. Requisitos no funcionales
- **Seguridad:** 0 accesos cruzados entre tenants en batería automatizada; CSRF en mutaciones; sesiones protegidas; SQL parametrizado.
- **Disponibilidad:** reinicio sin pérdida de datos de negocio; recuperación de sesiones persistidas en MySQL.
- **Rendimiento (metas iniciales, no garantías):** p95 < 300 ms para listado HTML con 10 000 registros/tenant y 25 filas por página, bajo prueba controlada de 50 usuarios concurrentes; documentar hardware y dataset.
- **Operación:** despliegue reproducible con Compose; migraciones versionadas; backups MySQL 8 probados.
- **Accesibilidad:** navegación por teclado y semántica HTML; objetivo WCAG 2.2 AA en flujos críticos.
- **Observabilidad:** request ID, logs JSON, latencias y errores por endpoint sin PII en etiquetas.

## 8. Métricas de validación del piloto
- Tiempo de implementar el segundo CRUD y tiempo de extraer componentes compartidos, registrados por separado; la extracción ocurre después de validar ambos CRUD.
- LOC de configuración de recurso vs lógica específica.
- p50/p95, RPS y memoria en escenarios equivalentes.
- Defectos de aislamiento y autorización (objetivo: cero).
- Complejidad del despliegue y tiempo de incorporación de un desarrollador.
Comparar con una implementación Laravel + Filament del mismo alcance, misma base de datos, datos y entorno, evitando conclusiones por benchmarks no equivalentes.

## 9. Fases y entregables
1. **Fundaciones (2 semanas):** entorno, esquema, migraciones, CI y autenticación.
2. **Seguridad y tenants (2 semanas):** membresías, roles, selección de tenant y pruebas de aislamiento.
3. **CRUD reutilizable (3 semanas):** tablas, formularios, clientes y productos.
4. **Dashboard y realtime (2 semanas):** métricas, broker SSE en memoria, SSE y refrescos.
5. **Hardening y comparación (2 semanas):** E2E, carga, accesibilidad, documentación.
**Estimación orientativa:** 11 semanas con equipo pequeño experimentado; recalibrar tras fase 1.

## 10. Criterios de salida MVP
- Todas las historias US-01 a US-09 aprobadas.
- Pruebas cross-tenant, CSRF, RBAC y concurrencia ejecutadas.
- CRUD funcional sin JavaScript de aplicación obligatorio más allá de HTMX.
- SSE con reconexión y aislamiento por tenant.
- Documentación para levantar proyecto desde cero y resultados de benchmark.

## 11. Riesgos y mitigaciones
- **Sobreingeniería:** limitar abstracciones a componentes utilizados en ambos CRUD.
- **Fugas multi-tenant:** tests de repositorio/HTTP y restricciones de datos; revisar cada consulta.
- **Pérdida de señales SSE:** una sola instancia Go; broker en memoria sin durabilidad. Al reconectar, refrescar desde MySQL; mantener refresco manual disponible. Varias réplicas quedan fuera de la POC.
- **Costo de componentes UI:** reutilizar Templ y Tailwind; accesibilidad desde MVP.
- **Comparación injusta:** mismas historias, datos, hardware y metodología.

## 12. Preguntas abiertas
Persistencia, política de sesiones y Argon2id están decididas en [ADR-0004](docs/adr/0004-autenticacion-sesiones-y-csrf.md): máximo 12 h, inactividad 30 min y touch cada 1 min por defecto; recuperación automática diferida. D-04/D-05 se cierran en [ADR-0005](docs/adr/0005-permisos-miembros-y-plataforma.md): gestión de miembros Admin/Viewer, Owner de bootstrap, configuración del nombre de empresa y administración global de empresas/métricas del pool SQL. No se habilita transferencia de Owner. D-06 de clientes se cierra mediante ADR-0006, con flujos HTML/HTMX y validación implementados. D-06 de productos se cierra mediante ADR-0007; despliegue y contratos de dashboard/SSE siguen pendientes de sus etapas. Herramientas y otros contratos operativos se concretan en [decisiones pendientes](docs/decisiones-pendientes.md), mediante ADR antes de la etapa correspondiente. La existencia de una pregunta abierta no incorpora una funcionalidad adicional al MVP.

## 13. Decisiones de alcance de la POC (ratificadas desde v1.1)
- No utilizar Redis, Memcached ni caché de aplicación.
- Persistir sesiones en MySQL 8; invalidarlas al cerrar sesión.
- SSE mediante un broker en memoria dentro del proceso Go, limitado a **una sola instancia**. La pérdida de eventos al reiniciar es aceptable; el navegador puede recargar el dashboard para recuperar el estado.
- Todas las métricas y listados se consultan directamente en MySQL, sin almacenamiento intermedio.
- Una arquitectura distribuida, pub/sub externo y caché quedan expresamente para fases posteriores.

# Decisiones pendientes

Fecha de revisión: 2026-10-21. Este registro distingue contratos ratificados de decisiones todavía abiertas. No añade funcionalidades al MVP. Cada decisión debe cerrarse en un ADR y reflejarse en las especificaciones antes de implementar el contrato afectado. Se puede avanzar en trabajo independiente.

## Contratos ratificados

Ver [ADR-0001](adr/0001-conciliacion-de-contratos-poc.md): sesiones en MySQL, SSE en memoria para una instancia, ausencia de caché, contexto de seguridad verificado, eventos públicos mínimos, rutas de creación/detalle y actualizaciones sin cambios.

## Pendientes y etapas límite

| ID | Decisión por cerrar | Límite | Evidencia necesaria para cerrar |
|---|---|---|---|
| D-01 | Versiones compatibles de Go, Chi, Templ, HTMX/extensión SSE, Tailwind, driver y sqlc; herramienta de migraciones y mecanismo de fijación | Etapa 1; validar generación SQL en etapa 2 | ADR que justifique dependencias y herramientas, versiones fijadas y generación/compilación reproducible. Comparar Goose y golang-migrate antes de elegir. No instalar generadores adicionales por conveniencia. |
| D-02 | Expiración absoluta e inactividad, frecuencia de actualización de `last_seen_at`, limpieza y parámetros Argon2id | Antes de implementar autenticación, etapa 3 | ADR con valores y variables de configuración, validaciones y pruebas de límites temporales; parámetros Argon2id justificados y medidos en el entorno objetivo. No se cambia la decisión de persistir sesiones en MySQL. |
| D-03 | Recuperación de contraseña: incluir un flujo concreto o diferirlo explícitamente | Antes de cerrar identidad, etapa 3 | Decisión de producto documentada; no asumir correo, proveedor externo ni tokens de recuperación. Login/logout no implican automáticamente recuperación. |
| D-04 | Operaciones de Platform Admin y alcance de `tenant.settings` | Antes del módulo de administración, etapa 4 | Enumerar operaciones, campos, rutas, permisos y auditoría. El PRD exige administrar tenants y consultar métricas técnicas; no autoriza impersonación ni acceso comercial automático. Decidir qué resuelve seed y qué requiere interfaz. |
| D-05 | Alta de miembros existentes y política de asignación/transferencia de Owner | Antes de gestión de membresías, etapa 4 | Definir identificación del usuario objetivo sin filtrar cuentas globales, operaciones/rutas y tratamiento del rol Owner. US-03 garantiza Admin/Viewer; no inferir permisos adicionales. Mantener bloqueo transaccional y protección del último Owner. Invitaciones por correo fuera del MVP. |
| D-06 | Validación y normalización de campos de clientes/productos, especialmente SKU | Antes del CRUD correspondiente, etapas 5 y 6 | Reglas para longitudes, espacios, campos opcionales y búsquedas literales. Definir normalización de SKU y probar collation insensible a mayúsculas/acentos, unicidad y reutilización tras baja. Mantener MXN y centavos enteros. |
| D-07 | Límites de los 30 días del dashboard | Antes de consultas del dashboard, etapa 7 | Precisar ventana móvil o días calendario, límites inclusivos/exclusivos, agrupación UTC y exclusión de bajas lógicas; pruebas de frontera y días sin registros. UTC de persistencia ya es obligatorio. |
| D-08 | Ciclo de autorización de conexiones SSE y configuración operativa del broker | Antes de SSE, etapa 7 | Definir cómo cerrar/revalidar streams al expirar o revocar sesión, membresía o tenant y al cambiar empresa; límites de suscriptores/buffers, clientes lentos, heartbeat y reconexión. Probar que una conexión larga no conserva acceso revocado. Sin reproducción duradera. |
| D-09 | Destino de producción, TLS/proxy, acceso a métricas, backups y restauración | Antes de despliegue fuera de desarrollo, como máximo etapa 8 | Runbook y ADR de despliegue de una instancia, secretos por entorno, cookies seguras, protección de endpoints operativos, retención de backups y restauración comprobada. Compose sigue siendo obligatorio. |
| D-10 | Implementación equivalente Laravel/Filament y protocolo de benchmark | Preparar medición en etapas 5/6; cerrar comparación en etapa 8 | Confirmar disponibilidad o alcance de construcción de la contraparte. Mismas historias, MySQL, dataset, hardware, calentamiento y carga. Si falta, reportar mediciones Go y comparación pendiente; no declarar cumplida la comparación. |

No hay valores finales ni aprobación implícita de nuevas funciones en esta tabla. Los bloqueos solo afectan la parte que depende de la decisión; deben comunicarse antes de implementarla.

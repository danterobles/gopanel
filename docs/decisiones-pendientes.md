# Decisiones pendientes

Fecha de revisión: 2026-10-21. Este registro distingue contratos ratificados de decisiones todavía abiertas. No añade funcionalidades al MVP. Cada decisión debe cerrarse en un ADR y reflejarse en las especificaciones antes de implementar el contrato afectado. Se puede avanzar en trabajo independiente.

## Contratos ratificados

Ver [ADR-0001](adr/0001-conciliacion-de-contratos-poc.md): sesiones en MySQL, SSE en memoria para una instancia, ausencia de caché, contexto de seguridad verificado, eventos públicos mínimos, rutas de creación/detalle y actualizaciones sin cambios.

## Decisiones cerradas

- D-01: selección y fijación cerradas en [ADR-0002](adr/0002-herramientas-y-bootstrap.md). Generación SQL y ejecución de migraciones verificadas contra MySQL local en etapa 2; limitaciones del generador documentadas en ADR-0003.

- D-02: cerrada mediante [ADR-0004](adr/0004-autenticacion-sesiones-y-csrf.md); parámetros, variables y medición local en validación de etapa 3.
- D-03: recuperación automática diferida explícitamente mediante ADR-0004; no se añade proveedor de correo ni flujo de reset.

- D-04: cerrada mediante [ADR-0005](adr/0005-permisos-miembros-y-plataforma.md): Owner edita nombre; Platform Admin administra empresas y consulta métricas del pool SQL, sin acceso comercial implícito.
- D-05: cerrada mediante ADR-0005: alta por correo exacto de cuenta existente activa, roles Admin/Viewer, Owner inicial de bootstrap y sin transferencia/promoción desde UI. Invariante, auditoría y revocación transaccionales.

- D-06 (clientes): cerrada mediante [ADR-0006](adr/0006-clientes-validacion-y-flujos-http.md): validación/normalización, opcionales NULL, filtros/orden/paginación, confirmación y representaciones HTML/HTMX. Productos cerrados por ADR-0007 en etapa 6.

- D-06 (productos): cerrada mediante [ADR-0007](adr/0007-productos-sku-precios-y-extraccion.md): SKU Unicode preservado con unicidad de collation, descripción normalizada, MXN exacto sin float y flujos CRUD HTML/HTMX.

## Pendientes y etapas límite

| ID | Decisión por cerrar | Límite | Evidencia necesaria para cerrar |
|---|---|---|---|
| D-07 | Límites de los 30 días del dashboard | Antes de consultas del dashboard, etapa 7 | Precisar ventana móvil o días calendario, límites inclusivos/exclusivos, agrupación UTC y exclusión de bajas lógicas; pruebas de frontera y días sin registros. UTC de persistencia ya es obligatorio. |
| D-08 | Ciclo de autorización de conexiones SSE y configuración operativa del broker | Antes de SSE, etapa 7 | Definir cómo cerrar/revalidar streams al expirar o revocar sesión, membresía o tenant y al cambiar empresa; límites de suscriptores/buffers, clientes lentos, heartbeat y reconexión. Probar que una conexión larga no conserva acceso revocado. Sin reproducción duradera. |
| D-09 | Destino de producción, TLS/proxy, acceso a métricas, backups y restauración | Antes de despliegue fuera de desarrollo, como máximo etapa 8 | Runbook y ADR de despliegue de una instancia, secretos por entorno, cookies seguras, protección de endpoints operativos, retención de backups y restauración comprobada. Compose sigue siendo obligatorio. |
| D-10 | Implementación equivalente Laravel/Filament y protocolo de benchmark | Preparar medición en etapas 5/6; cerrar comparación en etapa 8 | Confirmar disponibilidad o alcance de construcción de la contraparte. Mismas historias, MySQL, dataset, hardware, calentamiento y carga. Si falta, reportar mediciones Go y comparación pendiente; no declarar cumplida la comparación. |

No hay valores finales ni aprobación implícita de nuevas funciones en esta tabla. Los bloqueos solo afectan la parte que depende de la decisión; deben comunicarse antes de implementarla.

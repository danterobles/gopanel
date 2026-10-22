# Decisiones pendientes

Fecha de revisión: 2026-10-22. Este registro distingue contratos ratificados de decisiones todavía abiertas. No añade funcionalidades al MVP. Cada decisión debe cerrarse en un ADR y reflejarse en las especificaciones antes de implementar el contrato afectado. Se puede avanzar en trabajo independiente.

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

- D-07/D-08: cerradas mediante [ADR-0008](adr/0008-dashboard-utc-y-ciclo-sse.md): ventana calendario UTC, snapshots, streams pasivos revalidados, límites y reconexión sin replay.

## Pendientes y etapas límite

| ID | Decisión por cerrar | Límite | Evidencia necesaria para cerrar |
|---|---|---|---|
| D-09 (parcial) | Destino real/TLS/proxy/certificados, backup cifrado externo/retención y RPO/RTO | Antes de despliegue fuera de desarrollo | ADR-0009 y runbook entregados; métricas protegidas y restauración autenticable verificadas localmente. Usuario confirma que aún no hay destino. Falta validar la instalación real y Compose/CI remota. |
| D-10 (parcial) | Contraparte Laravel/Filament y comparación ejecutada | Antes de emitir conclusiones comparativas | Usuario confirma contraparte inexistente; protocolo y medición Go entregados (ADR-0009/0010). Falta implementación equivalente y repetir ambos bajo mismo protocolo. No se declara comparación cumplida. |

No hay valores finales ni aprobación implícita de nuevas funciones en esta tabla. Los bloqueos solo afectan la parte que depende de la decisión; deben comunicarse antes de implementarla.

# ADR-0001 — Conciliación de contratos de la POC

- Estado: aceptado para conciliar las especificaciones existentes.
- Fecha: 2026-10-21.
- Contexto: etapa 0 del plan autorizado; no implica implementación ni aprobación de funcionalidades adicionales.

## Contexto

El PRD contiene texto previo incompatible con sus decisiones de alcance v1.1 y con ARQUITECTURA, DATA y AGENTS. También existen diferencias entre ejemplos HTTP, formatos de eventos y la prohibición de confiar en campos de formularios. Se registra esta decisión antes de corregir los documentos.

## Decisiones

1. **Persistencia y ejecución.** MySQL 8 es la fuente de verdad de negocio y sesiones. No hay Redis, caché de aplicación ni cola de eventos persistida. SSE usa un broker en memoria y una sola instancia Go. Reiniciar puede perder señales, pero no datos ni sesiones válidas. La reconexión provoca una nueva lectura autorizada desde MySQL. Se descartan las menciones antiguas a SSE entre instancias y a sesiones en el broker.
2. **Contexto de seguridad.** El actor, su rol y el tenant de una operación de negocio se obtienen de sesión y membresía verificadas. El selector puede enviar un tenant candidato; el servidor debe comprobar que usuario, tenant y membresía están activos antes de persistirlo. La gestión de membresías puede recibir el usuario o membresía objetivo y el rol solicitado como datos de la operación, siempre bajo autorización del Owner y validación del recurso. Esos campos nunca definen los privilegios del actor. El alcance de asignación de Owner queda pendiente; US-03 solo exige asignar Admin/Viewer y preservar el último Owner activo.
3. **Eventos.** El broker necesita `tenant_id` para enrutar internamente. El navegador recibe únicamente `{type, resource, version}`. Publicar después del commit, sin PII ni entidades completas. `version` es una señal de invalidación, no una garantía de reproducción duradera ni orden global.
4. **Rutas.** La creación de clientes y productos usa GET/POST `/app/{customers|products}/new`, de acuerdo con la tabla de rutas. Los detalles usan GET `/app/{customers|products}/{id}`. Las rutas de tabla y creación deben distinguirse del identificador de detalle. El ejemplo de POST `/app/customers` se corrige para evitar dos contratos de creación.
5. **Actualizaciones sin cambios.** Cero filas modificadas no prueba por sí solo que un recurso no existe. El servicio debe distinguir una edición válida sin cambios de un recurso ausente o ajeno mediante una comprobación acotada al tenant, o una configuración del driver verificada para contar filas coincidentes. La elección técnica se probará al implementar el repositorio. Un recurso ajeno o eliminado recibe 404 genérico; una edición autorizada sin cambios no debe producir un falso 404.
6. **Reutilización.** Primero se construyen clientes y productos; después se extraen componentes efectivamente compartidos. Se mide el esfuerzo del segundo CRUD y de la extracción por separado, sin atribuirle reutilización que aún no existía.

## Alternativas descartadas

- Broker distribuido, Redis y sesiones en memoria: contradicen el alcance v1.1.
- Confiar en identificadores o roles del formulario: permite escalamiento de privilegios.
- Agregar endpoints duplicados para conciliar ejemplos: aumenta contratos sin una necesidad del MVP.
- Considerar toda actualización sin filas modificadas como 404: confunde existencia con modificación.

## Consecuencias y validación

No se modifica el esquema SQL de referencia ni se crean migraciones. Se actualizan PRD, ARQUITECTURA, DATA y AGENTS. La matriz de aceptación especifica las pruebas futuras; ninguna se declara ejecutada en esta etapa documental.

Las decisiones que requieren política de producto o selección de herramientas permanecen explícitas en [el registro de pendientes](../decisiones-pendientes.md), con una etapa límite. Este ADR no las resuelve de forma implícita.

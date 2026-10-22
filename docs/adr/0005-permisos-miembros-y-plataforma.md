# ADR-0005 — Permisos, miembros y administración de plataforma

Fecha: 2026-10-21. Estado: aceptada para etapa 4. Cierra D-04/D-05 dentro del MVP.

## Contexto

DATA define permisos de membresía y PRD US-03 garantiza asignar Admin/Viewer. Platform Admin administra empresas y consulta métricas técnicas sin acceso comercial implícito. No se incorpora registro público, correo ni transferencia de propiedad.

## Decisión

- Políticas con denegación por defecto para toda la matriz DATA. Cada servicio resuelve de nuevo la membresía activa y el usuario desde MySQL; ignora el rol incluido en cualquier formulario o contexto desactualizado. Los recursos se buscan con tenant e identificador. Falta de permiso: 403; recurso ajeno o ausente: 404 indistinguible.
- `GET /app/members`: Owner/Admin, listado paginado de 25, máximo offset 100000. `POST /app/members`: Owner añade una cuenta activa existente mediante correo exacto normalizado (igualdad binaria adicional para evitar equivalencias de acentos de la collation) y rol Admin/Viewer. Sin búsqueda global, autocompletado ni identificador de usuario proporcionado por el cliente. Cuenta inexistente, inactiva o ya vinculada devuelve el mismo error. El éxito confirma necesariamente que la cuenta puede vincularse, solo al Owner autorizado.
- `POST /app/members/{id}`: Owner cambia rol a Admin/Viewer y estado activo/inactivo. No se elimina historial. Owner existente puede conservar el rol al cambiar estado, sujeto a la misma invariante. Owner inicial se crea por bootstrap (seed o alta de empresa); no se permite promover a Owner ni transferir propiedad desde estos formularios. Una instalación con varios Owners de bootstrap puede degradarlos siempre que conserve uno activo. El último Owner no puede degradarse ni desactivarse. El bloqueo de la fila del tenant serializa autorización, mutación e invariante.
- Cambios efectivos de membresía y altas revocan todas las sesiones del usuario objetivo, también en otras empresas, dentro de la misma transacción. Se exige nuevo login y token antes de usar privilegios cambiados. Ediciones sin cambios no revocan sesiones. No se prolonga su expiración.
- `GET/POST /app/settings`: solo Owner, edición del nombre de su empresa (1–180 caracteres Unicode, trim). Slug inmutable; no configura roles, facturación ni activa/desactiva empresas.
- `GET /platform`: solo flag global vigente `is_platform_admin`, lista paginada de empresas (25) y métricas técnicas del pool SQL (abiertas, en uso, ociosas). Sin métricas comerciales ni datos de clientes/productos.
- `POST /platform/tenants`: Platform Admin crea empresa con nombre, slug (3–100 ASCII minúsculas, números y guiones, extremos alfanuméricos) y correo exacto de Owner existente activo. Tenant y Owner se crean juntos; sesiones de Owner revocadas. `POST /platform/tenants/{id}` edita nombre y estado; slug inmutable. Desactivar revoca sesiones con esa empresa activa. No hay borrado de empresas, edición de usuarios globales, impersonación ni membresía automática del operador. Puede reactivar empresas.
- Formularios con sesión, límite de cuerpo, CSRF y control de origen compartidos con autenticación. HTML completo en navegación, fragmento en HTMX; éxito con 303 o HX-Redirect. Validación 422, conflicto de último Owner 409, error de infraestructura 503 genérico.
- Éxitos y auditoría se confirman en la misma transacción. Denegaciones y fallos de validación se registran sin formularios, correos, nombres ni tokens. Auditoría global de plataforma con tenant NULL y entidad objetivo; auditoría comercial acotada a empresa del actor. Un fallo de auditoría impide confirmar la operación.

## Consecuencias y pruebas

No cambia el esquema ni se reescriben migraciones. El alta de cuentas sigue siendo bootstrap; una cuenta inexistente no puede invitarse. La transferencia de Owner queda fuera del MVP: no se supone un permiso adicional a US-03. Pruebas cubren matriz completa, roles distintos por empresa, HTTP manual, CSRF, recursos ajenos, revocación, atomicidad de auditoría y cambios concurrentes del último Owner. Métricas operativas avanzadas y SSE permanecen en sus etapas.

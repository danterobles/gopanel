# ADR-0008 — Dashboard UTC y ciclo SSE

Fecha: 2026-10-22. Estado: aceptado para etapa 7. Cierra D-07/D-08 sin ampliar alcance. Fuentes: PRD 4.5, DATA 7, ARQUITECTURA tiempo real.

## D-07: métricas

Los últimos 30 días son días calendario UTC: desde 00:00 del día actual menos 29 días, inclusive, hasta el instante de consulta, exclusivo. El servicio toma un único reloj UTC y una transacción REPEATABLE READ para autorización y agregados. Cuenta clientes activos y productos activos no eliminados; clientes recientes y serie cuentan clientes de cualquier estado no eliminados dentro de esa ventana. La serie tiene siempre 30 posiciones, con ceros, y excluye registros futuros. Las tarjetas de activos no dependen de created_at.

`GET /app` es el dashboard autenticado con navegación y logout. `GET /app/dashboard/widgets` devuelve documento normal o fragmento HTMX; no acepta un tenant desde el cliente para autorizar o consultar datos. Los refrescos envían X-Dashboard-Scope (tenant:rol renderizados, sin secretos); si difiere del actor actual se redirige a /app para renovar cabecera y navegación, por ejemplo al cambiar empresa en otra pestaña. Este marcador solo puede provocar recarga, nunca definir el ámbito o privilegios. Gráfica SVG accesible con tabla HTML con fecha y valor textual, sin nueva biblioteca. Refresco manual y enlace de recarga disponibles, estado vacío/error/carga visible. Todos los agregados se consultan en MySQL sin caché. No se añade índice/migración sin medición que lo justifique.

## D-08: transporte y autorización

Una instancia Go, broker efímero por tenant. Cada Save/Delete de clientes/productos publica únicamente tras COMMIT exitoso. Fallos, validación y rollback no publican. No hay outbox ni garantía ante caída entre commit y publicación; la reconexión/refresco recupera datos desde MySQL. Una edición válida sin cambios puede invalidar.

`GET /app/events`: cookie de sesión, sin token/tenant en URL; 401 sin sesión y 403 sin membresía activa o ante origen cruzado. No CORS. Sesión fijada al tenant inicial. Revalida en MySQL cada 2 segundos y antes de enviar cada señal; consulta limitada a 5 segundos. Comprueba expiración absoluta/inactividad, existencia de sesión, usuario/tenant/membresía activos y rol permitido. Revocación, logout, rotación al cambiar empresa o caducidad cierran el stream. No se promete atomicidad entre una comprobación y un commit concurrente; nunca se envían datos comerciales y cada GET posterior autoriza de nuevo.

Las comprobaciones SSE, conexiones/reconexiones y GET de widgets son pasivas: no renuevan last_seen_at. Ver el dashboard de fondo no mantiene viva la sesión; navegación normal sí sigue la política existente. Sin sesión nueva, EventSource recibe 401/403. En stream establecido se emite `closed` (sin datos sensibles) para cerrar la extensión y disparar un GET autorizado; ante error DB se corta sin datos y se permite reconexión. Heartbeat comentario cada 15 segundos, retry 3 segundos; toda conexión empieza con `ready`, que dispara GET widgets incluso con Last-Event-ID. No hay replay ni se confía en ese encabezado. `invalidate` transporta solo `{type,resource,version}` (version UUID opaco sin orden global), nunca tenant/actor/registro/PII. El aviso visual se obtiene del fragmento consultado.

Límites fijados en código para la POC: 256 conexiones totales, 32 por tenant y 4 por usuario; admisión rechazada 503 + Retry-After. Buffer 16 señales por suscriptor, publicación no bloqueante. Cliente lento con buffer lleno se desconecta (su cupo se retiene hasta salir del handler) y recupera por ready al reconectar. Suscripciones y buckets se retiran al salir; cierre del broker cancela streams durante SIGTERM. Cada escritura/flush tiene deadline de 5 segundos mediante ResponseController, manteniendo timeouts de rutas ordinarias. Límite máximo del ciclo normal de revocación: 2 segundos + hasta 5 segundos de consulta y 5 de escritura de cierre; errores cierran sin más datos. Sin garantía de entrega ni eventos persistidos.

La extensión HTMX SSE 2.2.4 ya está fijada y copiada localmente. Se carga solo en el dashboard; atributos sse-connect/hx-trigger/sse-close, sin JavaScript propio, inline ni CDN. Conexión fuera del fragmento para que el refresco no la reabra. Los recursos permanecen con su navegación actual; SSE actualiza dashboard/aviso, no interrumpe formularios o paginación.

## Verificación

Fronteras UTC, futuros, estados, bajas y tenants; HTML/HTMX y permisos. Broker con carreras, límites, overflow, cancelación y shutdown. SSE con MySQL/HTTP real: aislamiento, ready/reconexión, publicación tras commit, silencio ante rollback, revocación/expiración/rotación y no touch. Sin detener MySQL ni mutar datos comerciales de goconcept; bases temporales. Migraciones: ninguna.

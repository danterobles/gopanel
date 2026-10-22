# ADR-0004 — Autenticación, sesiones y CSRF

- Estado: aceptado para etapa 3.
- Fecha: 2026-10-21.
- Cierra D-02 y D-03; la medición de Argon2id y las pruebas se registran en validación de etapa 3.

## Decisiones

1. Sesión opaca: 32 bytes aleatorios, base64url en cookie; solo SHA-256 de los bytes originales en MySQL. Expiración absoluta 12 h, inactividad 30 min, touch condicionado por SQL como máximo cada 1 min. Variables `SESSION_ABSOLUTE_TTL`, `SESSION_IDLE_TTL`, `SESSION_TOUCH_INTERVAL` con validación de límites/relaciones. La actividad se mide con el último touch persistido; el margen conservador puede adelantar la expiración hasta un intervalo de touch. No se prolonga la expiración absoluta.
2. Login y cambio de empresa rotan token y CSRF. Cambiar empresa conserva fecha de creación/expiración absoluta; invalida el token anterior de forma transaccional. Un formulario de otra pestaña con CSRF anterior se rechaza después de cambiar empresa. Logout borra sesión y cookie. Cada petición autenticada consulta MySQL y cada acceso comercial resuelve usuario, membresía y tenant activos; el rol se toma de la membresía actual. Platform Admin sin membresías ve el selector vacío.
3. Cookie HttpOnly, Path=/, SameSite=Lax y Secure en producción. Producción usa prefijo `__Host-`, sin Domain. No confiar en cabeceras proxy para permisos, IP o seguridad de cookies. El despliegue TLS/proxy sigue en D-09.
4. CSRF en todos los POST, también login/HTMX. Login usa token aleatorio de cookie HttpOnly de una hora y formulario coincidente. Las mutaciones autenticadas usan token derivado con separación de dominio del token de sesión; nunca se envía el token de sesión en HTML. Comparación constante y comprobación adicional de origen con `http.CrossOriginProtection` de Go. Tokens solo en formularios o header `X-CSRF-Token`, nunca URL. Las cookies con prefijo de host previenen inyección desde subdominios en producción. Request body limitado a 16 KiB.
5. Argon2id: 64 MiB, 3 iteraciones, 2 lanes, salt de 16 bytes y clave de 32. Perfil compatible con los seeds existentes; verificador PHC estricto con límites antes de asignar memoria y comparación constante. Cuentas inexistentes/inactivas recorren un hash dummy del mismo perfil; error público idéntico y sin registrar correo/contraseña. Máximo dos verificaciones concurrentes por proceso, rechazo con 429 si están ocupadas. No se añaden dependencias.
6. Límite por IP remota directa: 10 intentos/minuto por defecto (`LOGIN_ATTEMPTS_PER_MINUTE`, 1–100). No usar `X-Forwarded-For`. Contadores por SHA-256 de IP en MySQL, transacción y bloqueo de fila para admisión concurrente. Migración nueva 000002 reversible, sin alterar 000001. El límite cuenta éxitos y fracasos; no distingue cuentas y no hay bloqueo por usuario que permita denegación dirigida.
7. Mantenimiento del proceso cada 5 minutos (`AUTH_CLEANUP_INTERVAL`): borrar sesiones expiradas/ociosas y buckets de intentos de más de 24 h, con contextos limitados. Sin instancia adicional ni caché; la validez de la sesión se comprueba aunque la limpieza no haya corrido.
8. Login, logout y cambio de empresa se auditan sin tokens ni PII. Éxitos y mutaciones de sesión se confirman en la misma transacción. Credenciales incorrectas se auditan globalmente con actor/tenant nulos, sin indicar si existe la cuenta. Denegaciones CSRF/empresa y límites se registran con resultado denied. Fallos de persistencia se presentan como 503 genérico y se registran con request ID, sin SQL ni secretos.
9. HTML completo en navegación normal y formularios funcionales sin JavaScript. HTMX devuelve fragmentos de error con 401/422 y redirección `HX-Redirect` en respuesta 200 cuando corresponde; el flujo normal usa 303. Configuración HTMX local permite mostrar errores semánticos, desactiva eval/scripts e indicadores inline; CSS de indicadores compilado localmente. No hay redirects tomados del cliente.
10. D-03: recuperación de contraseña diferida fuera de esta entrega/MVP de identidad; no se incluye email, tokens de recuperación ni reseteo administrativo sin especificación posterior. Esto explicita la pregunta abierta del PRD y no añade servicios. Las credenciales seed privadas siguen siendo válidas.

## Alcance y consecuencias

Se añaden páginas Templ mínimas de login, selector y empresa activa. `/app` solo confirma el contexto verificado; el dashboard/RBAC por acción y los módulos comerciales se construirán después. Los middlewares exponen un ActorContext confiable para esas etapas. No se cambia el esquema de sesiones ni se construye una abstracción genérica de recursos.

El rate limit en MySQL cuesta escrituras por intento, aceptable para la POC. El origen HTTP no sustituye el token CSRF ni la autorización. SSE de larga duración sigue necesitando revalidación propia (D-08). La limpieza del proceso es idempotente y no es necesaria para invalidar sesiones en una petición.

Fuentes: [Go CrossOriginProtection](https://pkg.go.dev/net/http#CrossOriginProtection), [Argon2id](https://pkg.go.dev/golang.org/x/crypto/argon2), [HTMX response handling](https://htmx.org/docs/#response-handling).
